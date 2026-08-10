package dispatch

import (
	"io"
	"log/slog"
)

// logger is used throughout the package. It defaults to discarding everything,
// so that library consumers (and tests) which never call SetLogger do not panic.
var logger = slog.New(slog.NewTextHandler(io.Discard, nil))

// SetLogger replaces the logger used by this package.
// Passing nil restores the default, which discards all output.
func SetLogger(l *slog.Logger) {
	if l == nil {
		l = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	logger = l
}
