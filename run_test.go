package dispatch

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func testCache(t *testing.T) Cache {
	t.Helper()
	cache, err := NewFileCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	return cache
}

func baseOpts() Opts {
	opts := Opts{}
	opts.Concurrency = 1
	return opts
}

// run drives PrepareAndRun to completion, returning its error.
func run(t *testing.T, opts Opts, stdin string, commandLine ...string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	interrupts := make(chan os.Signal, 1)
	return PrepareAndRun(ctx, strings.NewReader(stdin), opts, commandLine, testCache(t), interrupts)
}

func TestPrepareAndRunRejectsInvalidConcurrency(t *testing.T) {
	// Zero used to spawn no workers and exit "successfully" having run nothing;
	// negative used to panic in makeslice.
	for _, concurrency := range []int{0, -1, -100} {
		t.Run("concurrency", func(t *testing.T) {
			opts := baseOpts()
			opts.Concurrency = concurrency
			err := run(t, opts, "a\n", "echo", "hi")
			if err == nil {
				t.Fatalf("concurrency %d: expected an error, got nil", concurrency)
			}
			if !errors.Is(err, ErrInvalidOptions) {
				t.Errorf("concurrency %d: error = %v, want ErrInvalidOptions", concurrency, err)
			}
		})
	}
}

func TestPrepareAndRunRejectsNegativeRateLimitBucket(t *testing.T) {
	opts := baseOpts()
	limit := Duration(time.Millisecond)
	opts.RateLimit = &limit
	opts.RateLimitBucketSize = -1

	err := run(t, opts, "a\n", "echo", "hi")
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("error = %v, want ErrInvalidOptions", err)
	}
}

func TestPrepareAndRunRejectsSubMillisecondRateLimit(t *testing.T) {
	opts := baseOpts()
	limit := Duration(time.Microsecond)
	opts.RateLimit = &limit

	err := run(t, opts, "a\n", "echo", "hi")
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("error = %v, want ErrInvalidOptions", err)
	}
}

func TestPrepareAndRunRejectsUnparseableCommandTemplate(t *testing.T) {
	err := run(t, baseOpts(), "a\n", "echo", "{{.unclosed")
	if err == nil {
		t.Fatal("expected an error for a malformed template, got nil")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("error = %v, want ErrInvalidOptions", err)
	}
}

func TestPrepareAndRunRejectsUnparseableInputTemplate(t *testing.T) {
	opts := baseOpts()
	bad := "{{.unclosed"
	opts.Input = &bad

	err := run(t, opts, "a\n", "echo", "hi")
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("error = %v, want ErrInvalidOptions", err)
	}
}

func TestPrepareAndRunRejectsEmptyCommandLine(t *testing.T) {
	err := run(t, baseOpts(), "a\n")
	if err == nil {
		t.Fatal("expected an error for an empty command line, got nil")
	}
}

// A successful run reports no error and records each job in the cache.
func TestPrepareAndRunExecutesEveryJob(t *testing.T) {
	cache := testCache(t)
	opts := baseOpts()
	opts.Concurrency = 2
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := PrepareAndRun(ctx, strings.NewReader("1\n2\n3\n"), opts,
		[]string{"true"}, cache, make(chan os.Signal, 1))
	if err != nil {
		t.Fatalf("PrepareAndRun: %v", err)
	}

	marker := Marker(mustCommand(t, "true"))
	if _, err := cache.SuccessModTime(ctx, marker); err != nil {
		t.Errorf("expected a cached success for %s: %v", marker, err)
	}
}

// A failing job must be reported to the caller, so a CLI can exit nonzero.
func TestPrepareAndRunReportsFailedJobs(t *testing.T) {
	err := run(t, baseOpts(), "1\n2\n", "false")
	if !errors.Is(err, ErrJobsFailed) {
		t.Errorf("error = %v, want ErrJobsFailed", err)
	}
}

func TestPrepareAndRunSucceedsWhenAllJobsSucceed(t *testing.T) {
	if err := run(t, baseOpts(), "1\n2\n", "true"); err != nil {
		t.Errorf("PrepareAndRun = %v, want nil", err)
	}
}

// --skip-successes must not re-run a job already recorded as successful.
func TestPrepareAndRunSkipsCachedSuccesses(t *testing.T) {
	cache := testCache(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	opts := baseOpts()
	opts.SkipSuccesses = true

	first := PrepareAndRun(ctx, strings.NewReader("1\n"), opts,
		[]string{"echo", "{{.value}}"}, cache, make(chan os.Signal, 1))
	if first != nil {
		t.Fatalf("first run: %v", first)
	}

	// The second run has nothing left to do.
	stats, err := PrepareAndRunWithStats(ctx, strings.NewReader("1\n"), opts,
		[]string{"echo", "{{.value}}"}, cache, make(chan os.Signal, 1))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if got := stats.Skipped.Load(); got != 1 {
		t.Errorf("Skipped = %d, want 1", got)
	}
	if got := stats.Succeeded.Load(); got != 0 {
		t.Errorf("Succeeded = %d, want 0 (the job should have been skipped)", got)
	}
}

// A timeout counts as a real failure, and is recorded as such.
func TestPrepareAndRunTimesOutLongJobs(t *testing.T) {
	opts := baseOpts()
	timeout := Duration(100 * time.Millisecond)
	opts.Timeout = &timeout

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stats, err := PrepareAndRunWithStats(ctx, strings.NewReader("1\n"), opts,
		[]string{"sleep", "30"}, testCache(t), make(chan os.Signal, 1))

	if !errors.Is(err, ErrJobsFailed) {
		t.Errorf("error = %v, want ErrJobsFailed", err)
	}
	if got := stats.Failed.Load(); got != 1 {
		t.Errorf("Failed = %d, want 1", got)
	}
}

// --dry-run must not execute anything, nor write to the cache.
func TestPrepareAndRunDryRunExecutesNothing(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewFileCache(dir)
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	opts := baseOpts()
	opts.DryRun = true
	sentinel := dir + "/should-not-exist"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := PrepareAndRun(ctx, strings.NewReader("1\n"), opts,
		[]string{"touch", sentinel}, cache, make(chan os.Signal, 1)); err != nil {
		t.Fatalf("PrepareAndRun: %v", err)
	}

	if _, err := os.Stat(sentinel); err == nil {
		t.Error("a dry run created the file, so the command really executed")
	}
	marker := Marker(mustCommand(t, "touch", sentinel))
	if _, err := cache.SuccessModTime(ctx, marker); !errors.Is(err, ErrNotFound) {
		t.Errorf("a dry run wrote to the cache: %v", err)
	}
}

// --abort-on-error stops the run rather than working through the whole queue.
func TestPrepareAndRunAbortOnError(t *testing.T) {
	opts := baseOpts()
	opts.AbortOnError = true

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stats, err := PrepareAndRunWithStats(ctx, strings.NewReader("1\n2\n3\n4\n5\n"), opts,
		[]string{"false"}, testCache(t), make(chan os.Signal, 1))
	if err == nil {
		t.Fatal("expected an error when aborting on failure")
	}
	if got := stats.Failed.Load(); got > 2 {
		t.Errorf("Failed = %d; the run should have aborted promptly", got)
	}
}

// Successive interrupts must escalate: the first waits for running jobs, and
// later ones progressively signal them. The first interrupt cancels the run's
// context, so escalation must not itself give up when that happens.
func TestRunEscalatesSuccessiveInterrupts(t *testing.T) {
	logs := captureLogs(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	interrupts := make(chan os.Signal, 4)
	commands := make(chan RenderedCommand)
	opts := baseOpts()
	opts.Concurrency = 2
	opts.HideFailures = true

	// Long-running jobs which ignore SIGTERM, so the run is still active while
	// the interrupts escalate.
	cmd, err := NewRenderedCommand("bash", "-c", "trap '' TERM; sleep 30")
	if err != nil {
		t.Fatalf("NewRenderedCommand: %v", err)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case commands <- cmd:
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = Run(ctx, NewStats(2, 0), interrupts, opts, testCache(t), commands, nil)
	}()

	// Let the jobs start, then interrupt three times.
	time.Sleep(300 * time.Millisecond)
	for range 3 {
		interrupts <- os.Interrupt
		time.Sleep(300 * time.Millisecond)
	}

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after the escalation reached SIGKILL")
	}

	// Only the first three steps are asserted. The fourth signals the jobs'
	// process groups, and is reachable only if a further interrupt arrives
	// before the SIGKILL'd jobs are reaped, which cannot be timed reliably.
	for _, want := range []string{
		"received cancellation signal",
		"second CTRL-C received",
		"third CTRL-C received",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("missing escalation step %q; logs:\n%s", want, logs.String())
		}
	}

	// The escalation must actually have killed the jobs, rather than waiting
	// out their full 30 second sleep.
	if strings.Contains(logs.String(), `elapsed="30 seconds"`) {
		t.Errorf("jobs ran to completion, so the signals had no effect; logs:\n%s", logs.String())
	}
}

// PrepareAndRun must not leave goroutines behind, so that a long-lived host
// process can call it repeatedly.
func TestPrepareAndRunDoesNotLeakGoroutines(t *testing.T) {
	before := goroutineCount()

	if err := run(t, baseOpts(), "1\n2\n", "true"); err != nil {
		t.Fatalf("PrepareAndRun: %v", err)
	}

	if err := waitForGoroutines(before, 5*time.Second); err != nil {
		t.Error(err)
	}
}
