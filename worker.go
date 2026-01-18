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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/time/rate"
)

var (
	ErrUserCancelled = errors.New("user-cancelled session")
	ErrNoMoreJobs    = errors.New("no more jobs")
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
	AbortOnError        bool           `long:"abort-on-error" description:"stop running (as though CTRL-C were pressed) if a job fails"`
	CacheLocation       *string        `long:"cache-location" description:"path (or S3 URI) to record successes and failures"`
	Concurrency         int            `long:"concurrency" description:"run this many jobs in dispatch" default:"1"`
	DryRun              bool           `long:"dry-run" description:"simulate what would be run"`
	Input               *string        `long:"input" description:"send the input string (plus newline) forever as STDIN to each job"`
	RateLimit           *time.Duration `long:"rate-limit" description:"prevent jobs starting more than this often"`
	RateLimitBucketSize int            `long:"rate-limit-bucket-size" description:"allow a burst of up to this many jobs when enforcing the rate limit"`
	Timeout             *Duration      `long:"timeout" description:"cancel each job after this much time"`
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

	dirty          atomic.Bool
	Total          atomic.Int64
	queueEmptyTime time.Time

	since time.Time
	etc   *etc
}

func (s *Stats) ZeroQueued() int64 {
	defer s.SetDirty()
	old := s.Queued.Swap(0)
	if old != 0 {
		s.queueEmptyTime = time.Now()
	}
	return old
}

func (s *Stats) AddQueued() {
	if s.Queued.Add(1) == 1 {
		s.queueEmptyTime = time.Time{}
	}
	s.SetDirty()
}

func (s *Stats) SubQueued() {
	if s.Queued.Add(-1) == 0 {
		s.queueEmptyTime = time.Now()
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

func Worker(ctx context.Context, opts Opts, signaller <-chan os.Signal, cancel context.CancelCauseFunc, ch <-chan RenderedCommand, cache Cache, stats *Stats, limiter *rate.Limiter) {
	var ok bool
	var command RenderedCommand
	var cmd *exec.Cmd
	go func() {
		for sig := range signaller {
			if cmd != nil {
				if process := cmd.Process; process != nil {
					var err error
					if sig == syscall.SIGKILL {
						logger.Debug("sent kill signal", slog.Any("signal", sig), slog.Any("process", command), slog.Any("error", err))
						_ = process.Kill()
					} else if sig == syscall.SIGQUIT {
						logger.Debug("sent kill signal to all subprocesses too", slog.Any("signal", sig), slog.Any("process", command), slog.Any("error", err))
						_ = killProcess(-process.Pid)
					} else {
						err = process.Signal(sig)
						logger.Debug("sent signal", slog.Any("signal", sig), slog.Any("process", command), slog.Any("error", err))
					}
				}
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
		cmd = exec.CommandContext(subCtx, command.command[0], command.command[1:]...)

		// launch as new process group so that signals (ex: SIGINT) are not sent also the the child process
		createNewProcessGroup(cmd)

		if command.input != "" {
			cmd.Stdin = Yes{Line: []byte(fmt.Sprintf("%v\n", command.input))}
		}
		marker := Marker(command)

		var buffer bytes.Buffer
		enc := Must(zstd.NewWriter(&buffer))
		stdoutWriters := make([]io.Writer, 0, 2)
		stderrWriters := make([]io.Writer, 0, 2)
		stdoutWriters = append(stdoutWriters, enc)
		stderrWriters = append(stderrWriters, enc)
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
		var err error
		if opts.DryRun {
			err = Sleep(ctx, time.Second)
			buffer.Write([]byte("(dry run)"))
		} else {
			err = cmd.Run()
			Must0(enc.Close())
		}
		cmd = nil
		elapsed := time.Since(timer)
		output := buffer.String()
		if err == nil {
			stats.AddSucceeded(elapsed)
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
