package dispatch

import (
	"context"
	"strings"
	"testing"
)

// The package-level logger must be usable without a prior SetLogger call,
// otherwise every library entrypoint panics on its first log statement.
func TestLoggerUsableWithoutSetLogger(t *testing.T) {
	for line := range LineReader(strings.NewReader("one\n"), nil) {
		_ = line
	}
	logger.Debug("this must not panic")
	_ = context.Background()
}
