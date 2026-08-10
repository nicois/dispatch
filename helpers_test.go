package dispatch

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

func goroutineCount() int {
	return runtime.NumGoroutine()
}

// waitForGoroutines waits for the goroutine count to fall back to baseline,
// which it may not do instantly even once everything has been signalled to stop.
func waitForGoroutines(baseline int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var current int
	for time.Now().Before(deadline) {
		current = runtime.NumGoroutine()
		if current <= baseline {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("goroutine count is %d, want at most %d; still running:\n%s",
		current, baseline, dispatchGoroutines())
}

// dispatchGoroutines reports the stacks belonging to this package, to make a
// leak report actionable.
func dispatchGoroutines() string {
	buf := make([]byte, 1<<18)
	n := runtime.Stack(buf, true)
	var relevant []string
	for stack := range strings.SplitSeq(string(buf[:n]), "\n\n") {
		if strings.Contains(stack, "nicois/dispatch") {
			relevant = append(relevant, stack)
		}
	}
	return strings.Join(relevant, "\n\n")
}
