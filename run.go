package dispatch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/google/btree"
	"golang.org/x/time/rate"
)

type UnsortedCommand struct {
	command   RenderedCommand
	timestamp time.Time
	index     int64
}

// Run will execute the jobs received via the `commands` channel,
// respecting the provided context and the rate limiter.
// Behaviour such as the level of concurrency is controlled via `opts`.
// A pre-configured cache must also be provided, used to record output logs.
// Statistics will also be updated continuously.
func Run(ctx context.Context, stats *Stats, interruptChannel <-chan os.Signal, opts Opts, cache Cache, commands <-chan RenderedCommand, limiter *rate.Limiter) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	if stats == nil {
		logger.Warn("no statistics will be generated")
	} else {
		// Show the current status, every 10ish seconds
		go func() {
			_ = SleepInLockstep(ctx, 10*time.Second)
			ticker := time.NewTicker(10 * time.Second)
			var lastShown time.Time
		loop:
			for {
				select {
				case <-ctx.Done():
					break loop
				default:
				}
				if stats.ClearDirty() || time.Since(lastShown) >= 10*time.Minute-time.Second {
					logger.Info(stats.String())
					lastShown = time.Now()
				}
				select {
				case <-ctx.Done():
					break loop
				case <-ticker.C:
				}
			}
			ticker.Stop()
			_ = SleepInLockstep(context.Background(), time.Second)
			ticker = time.NewTicker(time.Second)
			for {
				if stats.ClearDirty() || time.Since(lastShown) >= 10*time.Minute-time.Second {
					logger.Info(stats.String())
					lastShown = time.Now()
				}
				<-ticker.C
			}
		}()
	}

	signallers := make([]chan os.Signal, 0, opts.Concurrency)
	// spawn the workers
	wg := &sync.WaitGroup{}
	for range opts.Concurrency {
		signaller := make(chan os.Signal, 2)
		signallers = append(signallers, signaller)
		wg.Add(1)
		go func() {
			defer wg.Done()
			Worker(ctx, opts, signaller, cancel, commands, cache, stats, limiter)
		}()
	}

	// Provide user feedback when starting the exit process, but waiting for running jobs
	go func() {
		select {
		case <-interruptChannel:
			if stats != nil {
				stats.Total.Add(-1 * stats.ZeroQueued())
				stats.SetDirty()
				if stats.ClearDirty() {
					logger.Info(stats.String())
				}
			}
			logger.Warn("received cancellation signal. Waiting for current jobs to finish before exiting. Hit CTRL-C again to exit sooner")
			cancel(ErrUserCancelled)
		case <-ctx.Done():
			logger.Info("ctx cancelled, leaving without cancelling")
			return
		}

		<-interruptChannel
		for _, signaller := range signallers {
			select {
			case signaller <- syscall.SIGTERM:
			default:
			}
		}
		logger.Warn("second CTRL-C received. Sending SIGTERM to running jobs. Hit CTRL-C again to use SIGKILL instead")

		<-interruptChannel
		for _, signaller := range signallers {
			select {
			case signaller <- syscall.SIGKILL:
			default:
			}
		}
		logger.Warn("third CTRL-C received. Sending SIGKILL to running jobs. Hit CTRL-C again to kill all subprocesses too")

		<-interruptChannel
		for _, signaller := range signallers {
			select {
			case signaller <- syscall.SIGQUIT:
			default:
			}
			close(signaller)
		}
		logger.Warn("fourth CTRL-C received. Sending SIGKILL to running jobs and their subprocesses")
	}()

	wg.Wait()
	return context.Cause(ctx)
}

func PrepareAndRun(ctx context.Context, reader io.Reader, opts Opts, commandLine []string, cache Cache, interruptChannel <-chan os.Signal) error {
	ctx, cancelCause := context.WithCancelCause(ctx)
	defer cancelCause(nil)
	var generator Generator

	if opts.JsonLine {
		generator = JsonLineGenerator
	} else if opts.CSV {
		generator = CsvGenerator
	} else {
		generator = SimpleLineGenerator
	}
	templ, err := ParseCommandline(commandLine)
	var input *template.Template
	if inputString := opts.Input; inputString != nil {
		if t, err := template.New("Input").Parse(*inputString); err == nil {
			input = t
		} else {
			logger.Error("cannot parse the input template", slog.Any("error", err))
			os.Exit(1)
		}
	}
	if err != nil {
		logger.Error("Fatal error while parsing the commandline", slog.Any("error", err))
		os.Exit(1)
	}

	var limiter *rate.Limiter
	var minimumDuration time.Duration
	if opts.RateLimit != nil {
		minimumDuration = *opts.RateLimit
		if opts.RateLimitBucketSize < 1 {
			opts.RateLimitBucketSize = 1
		}
		if *opts.RateLimit < time.Millisecond {
			return errors.New("rate limit must be at least a millisecond if defined")
		}
		limiter = rate.NewLimiter(rate.Every(*opts.RateLimit), opts.RateLimitBucketSize)
	}

	// initialise the stats collector
	stats := NewStats(opts.Concurrency, minimumDuration)

	// this channel is where we insert jobs we want to do,
	presortedCommands := make(chan UnsortedCommand, 10)

	// this channel provides the workers with the highest priority next job
	postSortedCommands := make(chan RenderedCommand)

	// ingest STDIN, generating commands and updating stats
	go func() {
		defer close(presortedCommands)
		var index int64
		for args := range generator(ctx, cancelCause, reader) {
			var mostRecentlyLastRun time.Time
			renderedCommand, err := Render(templ, input, args)
			if err != nil {
				logger.Info("could not render", slog.Any("error", err))
				stats.AddFailed(0)
				continue
			}
			marker := Marker(renderedCommand)
			if mtime, err := cache.SuccessModTime(ctx, marker); err == nil {
				if mtime.After(mostRecentlyLastRun) {
					mostRecentlyLastRun = mtime
				}
				if opts.SkipSuccesses {
					// skip jobs previously run successfully, unless outside of the debounce period
					if period := time.Since(mtime); opts.DebounceSuccessesPeriod != nil && period > time.Duration(*opts.DebounceSuccessesPeriod) {
						logger.Debug("already successfully executed, but outside the debounce period", slog.Any("command", renderedCommand))
					} else {
						logger.Debug("already successfully executed", "command", renderedCommand, slog.String("cached combined output file", marker))
						stats.Skipped.Add(1)
						continue
					}
				}
			}
			if mtime, err := cache.FailureModTime(ctx, marker); err == nil {
				if mtime.After(mostRecentlyLastRun) {
					mostRecentlyLastRun = mtime
				}
				if opts.SkipFailures {
					// skip jobs previously run unsuccessfully, unless outside of the debounce period
					if period := time.Since(mtime); opts.DebounceFailuresPeriod != nil && period > time.Duration(*opts.DebounceFailuresPeriod) {
						logger.Debug("already unsuccessfully executed, but outside the debounce period", slog.Any("command", renderedCommand))
					} else {
						logger.Debug("already unsuccessfully executed", "command", renderedCommand, slog.String("cached combined output file", marker))
						stats.Skipped.Add(1)
						continue
					}
				}
			}
			if !opts.DeferReruns {
				// treat all times as Zero, meaning that the original sequence is preserved in the btree, via the index
				mostRecentlyLastRun = time.Time{}
			}
			if opts.Shuffle {
			} else {
				index = rand.Int63()
			}
			select {
			case <-ctx.Done():
				return
			case presortedCommands <- UnsortedCommand{command: renderedCommand, timestamp: mostRecentlyLastRun, index: index}:
				logger.Debug("inserted unsorted command", slog.Any("command", renderedCommand))
			}
			stats.Total.Add(1)
			stats.AddQueued()
		}
	}()

	go sorter(ctx, opts, presortedCommands, postSortedCommands)

	// call the main entrypoint, now everything is in place
	err = Run(ctx, stats, interruptChannel, opts, cache, postSortedCommands, limiter)
	// provide a summary before exiting
	logger.Info(stats.String())
	if errors.Is(err, ErrNoMoreJobs) {
		return nil
	}
	return err
}

func lessUnsortedCommand(a, b UnsortedCommand) bool {
	if a.timestamp.Equal(b.timestamp) {
		return a.index < b.index
	}
	return a.timestamp.Before(b.timestamp)
}

func sorter(ctx context.Context, opts Opts, presortedCommands <-chan UnsortedCommand, postSortedCommands chan<- RenderedCommand) {
	// hold a sorted representation of the commands
	tree := btree.NewG(2, lessUnsortedCommand)

	// ensure reading and writing don't interfere with each other
	mutex := new(sync.RWMutex)

	// notify at least one item is in the btree
	youHaveMail := make(chan struct{})

	// insert new items into the btree
	go func() {
		var minitime time.Time
		defer close(youHaveMail)
		for {
			// if context is cancelled, exit
			select {
			case <-ctx.Done():
				return
			default:
			}
			select {
			case <-ctx.Done():
				return
			case uc, ok := <-presortedCommands:
				if !ok {
					// channel is closed; nothing more is incoming
					return
				}
				if uc.timestamp.IsZero() {
					// similate a very old time, but not identical to other very old times
					minitime = minitime.Add(time.Nanosecond)
					uc.timestamp = minitime
				}
				// insert the command into the btree
				mutex.Lock()

				_, replaced := tree.ReplaceOrInsert(uc)
				logger.Debug("inserted into BTREE", slog.Any("command", uc))
				mutex.Unlock()
				if replaced {
					panic("this should not happen")
				}
				// notify anyone who cares that new data is available
				select {
				case youHaveMail <- struct{}{}:
				default:
				}
			}
		}
	}()

	// postSortedCommands is the channel which yields the jobs
	// which are ready to run as soon as a worker is available
	defer close(postSortedCommands)

	// sleep a teeny tiny amount to allow the BTREE a chance to get populated
	// with higher-priority jobs. Otherwise, the first jobs to be inserted will
	// immediately be retrieved, even if they are lower priority than jobs
	// inserted a ms later
	if opts.DeferReruns {
		delay := 100 * time.Millisecond
		if dd := opts.DeferDelay; dd != nil {
			delay = time.Duration(*dd)
		}
		logger.Debug("delaying to improve effectiveness of deferring reruns", slog.Duration("delay period", delay))
		_ = Sleep(ctx, delay)
	}

	for {
		var finalIteration bool

		// wait for at least one item to be in the btree
		select {
		case <-ctx.Done():
			return
		case _, ok := <-youHaveMail:
			if !ok {
				finalIteration = true
			}
		}
		// keep sending the oldest known item until the tree
		// is empty or the context is cancelled
		for {
			mutex.Lock()
			uc, found := tree.DeleteMin()
			mutex.Unlock()
			if !found {
				if finalIteration {
					return
				}
				break
			}
			select {
			case <-ctx.Done():
				return
			default:
			}

			select {
			case <-ctx.Done():
				return

				// this is a zero-length channel and will
				// mostly be blocked as all workers will be busy.
			case postSortedCommands <- uc.command:
				logger.Debug("inserted into queue", slog.Any("command", uc))
				// This is a bit of a hack. The intention is that, when rate-limiting,
				// the first jobs picked up by workers are also the first ones to
				// complete the limiter.Wait() step
				time.Sleep(10 * time.Microsecond)
			}
		}
	}
}
