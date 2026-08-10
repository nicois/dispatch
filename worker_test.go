package dispatch

import (
	"context"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Escalating signals while jobs are running must not race on the shared
// *exec.Cmd. Run under -race, this reproduces the read/write race between the
// signal-handling goroutine and the worker starting its next job.
func TestWorkerSignalEscalationIsRaceFree(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	commands := make(chan RenderedCommand)
	signaller := make(chan os.Signal, 4)
	stats := NewStats(1, 0)
	opts := baseOpts()

	var wg sync.WaitGroup
	wg.Go(func() {
		Worker(ctx, opts, signaller, cancel, commands, testCache(t), stats, nil)
	})

	// Keep the worker churning through short jobs while signals arrive, so the
	// signal handler repeatedly observes cmd being replaced.
	feeding := make(chan struct{})
	go func() {
		defer close(feeding)
		cmd, err := NewRenderedCommand("sleep", "0.05")
		if err != nil {
			return
		}
		for range 20 {
			select {
			case <-ctx.Done():
				return
			case commands <- cmd:
			}
		}
	}()

	for range 30 {
		select {
		case signaller <- syscall.SIGTERM:
		default:
		}
		time.Sleep(2 * time.Millisecond)
	}

	<-feeding
	cancel(ErrUserCancelled)
	close(commands)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Worker did not return after cancellation")
	}
}

// Once its command channel is closed and drained, a Worker must return and
// leave no goroutine behind.
func TestWorkerReturnsWithoutLeakingGoroutines(t *testing.T) {
	before := goroutineCount()

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	commands := make(chan RenderedCommand, 1)
	cmd, err := NewRenderedCommand("true")
	if err != nil {
		t.Fatalf("NewRenderedCommand: %v", err)
	}
	commands <- cmd
	close(commands)

	Worker(ctx, baseOpts(), make(chan os.Signal), cancel, commands, testCache(t), NewStats(1, 0), nil)

	if err := waitForGoroutines(before, 5*time.Second); err != nil {
		t.Error(err)
	}
}

// Run documents that stats may be nil; that must not panic when a job succeeds.
func TestWorkerToleratesNilStats(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	commands := make(chan RenderedCommand, 2)
	success, err := NewRenderedCommand("true")
	if err != nil {
		t.Fatalf("NewRenderedCommand: %v", err)
	}
	failure, err := NewRenderedCommand("false")
	if err != nil {
		t.Fatalf("NewRenderedCommand: %v", err)
	}
	commands <- success
	commands <- failure
	close(commands)

	// Panics propagate out of Worker and fail the test.
	Worker(ctx, baseOpts(), make(chan os.Signal), cancel, commands, testCache(t), nil, nil)
}

// A job's combined output must be captured, compressed and readable back from
// the cache, on both the success and the failure path.
func TestWorkerRecordsOutput(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		success bool
	}{
		{
			name:    "successful job",
			args:    []string{"sh", "-c", "echo OUT; echo ERR >&2"},
			want:    "OUT",
			success: true,
		},
		{
			// Regression: the failure path did not close the zstd encoder, so
			// buffered output could be lost.
			name:    "failed job",
			args:    []string{"sh", "-c", "echo BEFORE_FAILING; exit 3"},
			want:    "BEFORE_FAILING",
			success: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			cache := testCache(t)

			cmd, err := NewRenderedCommand(tc.args...)
			if err != nil {
				t.Fatalf("NewRenderedCommand: %v", err)
			}
			commands := make(chan RenderedCommand, 1)
			commands <- cmd
			close(commands)

			Worker(ctx, baseOpts(), make(chan os.Signal), cancel, commands, cache, NewStats(1, 0), nil)

			marker := Marker(cmd)
			var raw []byte
			if tc.success {
				raw, err = cache.ReadSuccess(ctx, marker)
			} else {
				raw, err = cache.ReadFailure(ctx, marker)
			}
			if err != nil {
				t.Fatalf("reading back %s: %v", marker, err)
			}

			var out strings.Builder
			if err := Decompress(strings.NewReader(string(raw)), &out); err != nil {
				t.Fatalf("decompressing %s: %v", marker, err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("cached output = %q, want it to contain %q", out.String(), tc.want)
			}
		})
	}
}

// Both streams are captured, and stderr is included alongside stdout.
func TestWorkerCapturesStderr(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	cache := testCache(t)

	cmd, err := NewRenderedCommand("sh", "-c", "echo TO_STDERR >&2")
	if err != nil {
		t.Fatalf("NewRenderedCommand: %v", err)
	}
	commands := make(chan RenderedCommand, 1)
	commands <- cmd
	close(commands)

	Worker(ctx, baseOpts(), make(chan os.Signal), cancel, commands, cache, NewStats(1, 0), nil)

	raw, err := cache.ReadSuccess(ctx, Marker(cmd))
	if err != nil {
		t.Fatalf("ReadSuccess: %v", err)
	}
	var out strings.Builder
	if err := Decompress(strings.NewReader(string(raw)), &out); err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if !strings.Contains(out.String(), "TO_STDERR") {
		t.Errorf("cached output = %q, want it to contain the stderr text", out.String())
	}
}

// A job which reads STDIN must receive the configured input.
func TestWorkerFeedsInputToJob(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	cache := testCache(t)

	cmd, err := NewRenderedCommand("sh", "-c", "read line; echo GOT=$line")
	if err != nil {
		t.Fatalf("NewRenderedCommand: %v", err)
	}
	cmd = cmd.WithInput("hello-from-stdin")
	commands := make(chan RenderedCommand, 1)
	commands <- cmd
	close(commands)

	Worker(ctx, baseOpts(), make(chan os.Signal), cancel, commands, cache, NewStats(1, 0), nil)

	raw, err := cache.ReadSuccess(ctx, Marker(cmd))
	if err != nil {
		t.Fatalf("ReadSuccess: %v", err)
	}
	var out strings.Builder
	if err := Decompress(strings.NewReader(string(raw)), &out); err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if !strings.Contains(out.String(), "GOT=hello-from-stdin") {
		t.Errorf("cached output = %q, want the input to have been delivered", out.String())
	}
}
