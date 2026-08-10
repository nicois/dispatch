package dispatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/time/rate"
)

var (
	ErrUserCancelled = errors.New("user-cancelled session")
	ErrNoMoreJobs    = errors.New("no more jobs")
	// ErrJobsFailed reports that the run completed but some jobs failed.
	ErrJobsFailed = errors.New("some jobs failed")
	// ErrInvalidOptions reports a self-inconsistent configuration.
	ErrInvalidOptions = errors.New("invalid options")
)

type PreparationOpts struct {
	CSV                     bool      `long:"csv" description:"interpret STDIN as a CSV"`
	DebounceFailuresPeriod  *Duration `long:"debounce-failures" description:"re-run failed jobs outside the debounce period, even if they would normally be skipped"`
	DebounceSuccessesPeriod *Duration `long:"debounce-successes" description:"re-run successful jobs outside the debounce period, even if they would normally be skipped"`
	DeferDelay              *Duration `long:"defer-delay" description:"when deferring reruns, wait some time before beginning processing"`
	DeferReruns             bool      `long:"defer-reruns" description:"give priority to jobs which have not previously been run"`
	JsonLine                bool      `long:"json-line" description:"interpret STDIN as JSON objects, one per line"`
	Shuffle                 bool      `long:"shuffle" description:"disregard the order in which the jobs were given"`
	SkipFailures            bool      `long:"skip-failures" description:"skip jobs which have already been run unsuccessfully"`
	SkipSuccesses           bool      `long:"skip-successes" description:"skip jobs which have already been run successfully"`
}
type ExecutionOpts struct {
	AbortOnError        bool      `long:"abort-on-error" description:"stop running (as though CTRL-C were pressed) if a job fails"`
	CacheLocation       *string   `long:"cache-location" description:"path (or S3 URI) to record successes and failures"`
	Concurrency         int       `long:"concurrency" description:"run this many jobs in dispatch" default:"1"`
	DryRun              bool      `long:"dry-run" description:"simulate what would be run"`
	Input               *string   `long:"input" description:"send the input string (plus newline) forever as STDIN to each job"`
	RateLimit           *Duration `long:"rate-limit" description:"prevent jobs starting more than this often"`
	RateLimitBucketSize int       `long:"rate-limit-bucket-size" description:"allow a burst of up to this many jobs when enforcing the rate limit"`
	Timeout             *Duration `long:"timeout" description:"cancel each job after this much time"`
}

type OutputOpts struct {
	Debug         bool `long:"debug" description:"show more detailed log messages"`
	HideFailures  bool `long:"hide-failures" description:"do not display a message each time a job fails"`
	HideSuccesses bool `long:"hide-successes" description:"do not display a message each time a job succeeds"`
	ShowStderr    bool `long:"show-stderr" description:"do not suppress each job's STDERR"`
	ShowStdout    bool `long:"show-stdout" description:"do not suppress each job's STDOUT"`
}

type Opts struct {
	PreparationOpts `group:"preparation"`
	ExecutionOpts   `group:"execution"`
	OutputOpts      `group:"output"`
}

func Marker(cmd RenderedCommand) string {
	h := sha256.New()
	for _, arg := range cmd.command {
		h.Write([]byte(arg))
		h.Write([]byte("\t"))
	}
	if cmd.input != "" {
		h.Write([]byte(cmd.input))
	} else if cmd.hasInput {
		// An explicitly empty input is a distinct job from no input at all.
		// Only this case is new, so markers written by earlier versions
		// remain valid.
		h.Write([]byte("\x00empty-input"))
	}
	return fmt.Sprintf("%x.zstd", h.Sum(nil)[:10])
}

type Stats struct {
	Queued     atomic.Int64
	Skipped    atomic.Int64
	InProgress atomic.Int64
	Succeeded  atomic.Int64
	Failed     atomic.Int64
	Aborted    atomic.Int64

	dirty atomic.Bool
	Total atomic.Int64

	// queueEmptyNanos is when the queue last became empty, as Unix nanoseconds,
	// or zero while the queue is occupied. It is written by the workers and read
	// by the estimator, so it is accessed atomically.
	queueEmptyNanos atomic.Int64

	since time.Time
	etc   *etc
}

// queueEmptyTime reports when the queue last became empty, and whether it is
// currently empty at all.
func (s *Stats) queueEmptyTime() (time.Time, bool) {
	nanos := s.queueEmptyNanos.Load()
	if nanos == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, nanos), true
}

func (s *Stats) ZeroQueued() int64 {
	defer s.SetDirty()
	old := s.Queued.Swap(0)
	if old != 0 {
		s.queueEmptyNanos.Store(time.Now().UnixNano())
	}
	return old
}

func (s *Stats) AddQueued() {
	if s.Queued.Add(1) == 1 {
		s.queueEmptyNanos.Store(0)
	}
	s.SetDirty()
}

func (s *Stats) SubQueued() {
	if s.Queued.Add(-1) == 0 {
		s.queueEmptyNanos.Store(time.Now().UnixNano())
	}
	s.SetDirty()
}

func (s *Stats) AddSucceeded(d time.Duration) {
	s.Succeeded.Add(1)
	s.InProgress.Add(-1)
	s.etc.AddSuccess(d)
	s.SetDirty()
}

func (s *Stats) AddAborted(d time.Duration) {
	s.Aborted.Add(1)
	s.InProgress.Add(-1)
	s.etc.AddFailure(d)
	s.SetDirty()
}

func (s *Stats) AddFailed(d time.Duration) {
	s.Failed.Add(1)
	s.InProgress.Add(-1)
	s.etc.AddFailure(d)
	s.SetDirty()
}

// AddUnstartedFailure records a job which failed before it could be started,
// such as one whose template could not be rendered. Unlike AddFailed it does
// not adjust the in-progress count, as the job never occupied a worker.
func (s *Stats) AddUnstartedFailure() {
	s.Failed.Add(1)
	s.Total.Add(1)
	s.SetDirty()
}

func NewStats(concurrency int, minimumDuration time.Duration) *Stats {
	result := Stats{since: time.Now(), etc: NewEtc(concurrency, minimumDuration)}
	return &result
}

func (s *Stats) IsDirty() bool {
	return s.dirty.Load()
}

func (s *Stats) SetDirty() {
	s.dirty.Store(true)
}

func (s *Stats) ClearDirty() bool {
	return s.dirty.Swap(false)
}

func (s *Stats) String() string {
	etaString := ""
	if d, err := s.etc.Estimate(s); err == nil {
		etaString = FriendlyDuration(d)
	}
	var etaPart string
	var skippedPart string
	if etaString == "" {
		etaPart = fmt.Sprintf("Elapsed time: %v", time.Since(s.since).Round(time.Second))
	} else {
		etaPart = fmt.Sprintf("Estimated time remaining: %v", etaString)
	}
	if skipped := s.Skipped.Load(); skipped > 0 {
		skippedPart = fmt.Sprintf(" (+%v skipped)", skipped)
	}

	return fmt.Sprintf("Queued: %v; In progress: %v; Succeeded: %v; Failed: %v; Aborted: %v; Total: %v%v; %v",
		s.Queued.Load(),
		s.InProgress.Load(),
		s.Succeeded.Load(),
		s.Failed.Load(),
		s.Aborted.Load(),
		s.Total.Load(),
		skippedPart,
		etaPart,
	)
}

// syncWriter serialises writes to an underlying writer.
//
// os/exec copies a job's stdout and stderr in separate goroutines whenever they
// are not the same *os.File, so the writer shared between them must be safe for
// concurrent use. Without this, output is interleaved unpredictably and can be
// lost outright.
type syncWriter struct {
	mutex sync.Mutex
	w     io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.w.Write(p)
}

// currentJob holds the process a worker is presently running, so that the
// signal-handling goroutine and the worker itself can both reach it safely.
type currentJob struct {
	mutex   sync.Mutex
	cmd     *exec.Cmd
	command RenderedCommand
}

func (c *currentJob) set(cmd *exec.Cmd, command RenderedCommand) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cmd = cmd
	c.command = command
}

func (c *currentJob) clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cmd = nil
	c.command = RenderedCommand{}
}

// signal delivers sig to the running process, if there is one. The lock is held
// throughout so the process cannot be replaced midway.
func (c *currentJob) signal(sig os.Signal) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.cmd == nil {
		return
	}
	process := c.cmd.Process
	if process == nil {
		// The job has been created but not yet started.
		return
	}
	switch sig {
	case syscall.SIGKILL:
		err := process.Kill()
		logger.Debug("sent kill signal", slog.Any("signal", sig), slog.Any("process", c.command), slog.Any("error", err))
	case syscall.SIGQUIT:
		// Signal the whole process group, catching any subprocesses.
		err := killProcess(-process.Pid)
		logger.Debug("sent kill signal to all subprocesses too", slog.Any("signal", sig), slog.Any("process", c.command), slog.Any("error", err))
	default:
		err := process.Signal(sig)
		logger.Debug("sent signal", slog.Any("signal", sig), slog.Any("process", c.command), slog.Any("error", err))
	}
}

// Worker consumes jobs from ch until the channel is closed or ctx is cancelled.
// Signals received on signaller are forwarded to the job currently running.
// stats may be nil, in which case no statistics are recorded.
func Worker(ctx context.Context, opts Opts, signaller <-chan os.Signal, cancel context.CancelCauseFunc, ch <-chan RenderedCommand, cache Cache, stats *Stats, limiter *rate.Limiter) {
	var ok bool
	var command RenderedCommand
	job := new(currentJob)

	// Forward signals until the worker returns, so this goroutine does not
	// outlive it even if signaller is never closed.
	handlerDone := make(chan struct{})
	defer close(handlerDone)
	go func() {
		for {
			select {
			case <-handlerDone:
				return
			case sig, open := <-signaller:
				if !open {
					return
				}
				job.signal(sig)
			}
		}
	}()

	for {
		// exit immediately if the context is cancelled
		select {
		case <-ctx.Done():
			return
		default:
		}

		select {
		case <-ctx.Done():
			return
		case command, ok = <-ch:
			if !ok {
				return
			}
		}
		if limiter != nil {
			// exit immediately if the context is cancelled while waiting for a slot
			if err := limiter.Wait(ctx); err != nil {
				return
			}
		}
		timer := time.Now()
		logger.Debug("about to execute", slog.Any("command", command))
		var subCancel context.CancelFunc
		subCtx := context.Background()
		if opts.Timeout != nil {
			subCtx, subCancel = context.WithTimeout(subCtx, time.Duration(*opts.Timeout))
		}
		cmd := exec.CommandContext(subCtx, command.command[0], command.command[1:]...)

		// launch as new process group so that signals (ex: SIGINT) are not sent also the the child process
		createNewProcessGroup(cmd)

		// hasInput, rather than a non-empty string, so that an explicitly
		// empty --input still feeds newlines to a job which reads STDIN.
		if command.hasInput {
			cmd.Stdin = &Yes{Line: fmt.Appendf(nil, "%v\n", command.input)}
		}
		marker := Marker(command)

		var buffer bytes.Buffer
		enc, err := zstd.NewWriter(&buffer)
		if err != nil {
			// Nothing can be recorded without an encoder, so stop rather than
			// running jobs whose results are silently discarded.
			cancel(fmt.Errorf("could not create a compressor: %w", err))
			if subCancel != nil {
				subCancel()
			}
			return
		}
		// Both streams feed the one encoder, so guard it against the concurrent
		// copier goroutines os/exec uses.
		combined := &syncWriter{w: enc}
		stdoutWriters := make([]io.Writer, 0, 2)
		stderrWriters := make([]io.Writer, 0, 2)
		stdoutWriters = append(stdoutWriters, combined)
		stderrWriters = append(stderrWriters, combined)
		if opts.ShowStderr {
			stderrWriters = append(stderrWriters, os.Stderr)
		}
		if opts.ShowStdout {
			stdoutWriters = append(stdoutWriters, os.Stdout)
		}
		cmd.Stdout = io.MultiWriter(stdoutWriters...)
		cmd.Stderr = io.MultiWriter(stderrWriters...)
		if stats != nil {
			stats.InProgress.Add(1)
			stats.SubQueued()
		}
		if opts.DryRun {
			err = Sleep(ctx, time.Second)
			_, _ = enc.Write([]byte("(dry run)"))
		} else {
			// Start and Wait are used separately, rather than Run, so that the
			// process is only published to the signal handler once Start has
			// finished populating cmd.Process.
			if err = cmd.Start(); err == nil {
				job.set(cmd, command)
				err = cmd.Wait()
				job.clear()
			}
		}
		// Always close the encoder, so that buffered output is flushed on the
		// failure path too, and its goroutines are released.
		if closeErr := enc.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("could not finish compressing the output: %w", closeErr)
		}
		elapsed := time.Since(timer)
		output := buffer.String()
		if err == nil {
			if stats != nil {
				stats.AddSucceeded(elapsed)
			}
			if !opts.HideSuccesses {
				logger.Info("Success", slog.String("elapsed", FriendlyDuration(elapsed)), slog.Any("command", command), slog.String("output ID", marker))
			}
			if !opts.DryRun {
				if err = cache.WriteSuccess(ctx, marker, []byte(output)); err != nil {
					cancel(fmt.Errorf("could not mark command as successful: %w", err))
				}
			}
		} else {
			// the job has failed - but is it because we chose to cancel before it was done,
			// or because the job actually failed? Remember that a timeout counts as a real failure
			realFailure := subCtx.Err() == nil || errors.Is(subCtx.Err(), context.DeadlineExceeded)
			if realFailure {
				if stats != nil {
					stats.AddFailed(elapsed)
				}
			} else {
				logger.Warn("job was aborted due to context cancellation", slog.Any("command", command))
				if stats != nil {
					stats.AddAborted(elapsed)
				}
			}
			if !opts.HideFailures {
				logger.Warn("Failure", slog.String("elapsed", FriendlyDuration(elapsed)), slog.Any("command", command), slog.String("output ID", marker), slog.Any("error", err))
			}
			// store the fact this failed (unless it was due to context cancellation)
			if !opts.DryRun && realFailure {
				if err = cache.WriteFailure(ctx, marker, []byte(output)); err != nil {
					cancel(fmt.Errorf("could not mark command as failed: %w", err))
				}
			}
			if cancel != nil && opts.AbortOnError {
				cancel(errors.New("nonzero exit code"))
			}
		}
		if subCancel != nil {
			subCancel()
		}
	}
}
