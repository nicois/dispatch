package dispatch

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// captureLogs redirects package logging for the duration of a test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	previous := logger
	logger = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { logger = previous })
	return buf
}

func TestGetS3ExpiryTimeUnset(t *testing.T) {
	t.Setenv("AWS_EXPIRY_TIME", "")
	if got := GetS3ExpiryTime(); got != nil {
		t.Errorf("GetS3ExpiryTime = %v, want nil when unset", got)
	}
}

func TestGetS3ExpiryTimeParsesRFC3339(t *testing.T) {
	want := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	t.Setenv("AWS_EXPIRY_TIME", want.Format(time.RFC3339))

	got := GetS3ExpiryTime()
	if got == nil {
		t.Fatal("GetS3ExpiryTime = nil, want a parsed time")
	}
	if !got.Equal(want) {
		t.Errorf("GetS3ExpiryTime = %v, want %v", got, want)
	}
}

// A malformed value must be reported, not silently ignored: the consequence is
// losing the safety margin before the credentials expire.
func TestGetS3ExpiryTimeWarnsOnMalformedValue(t *testing.T) {
	logs := captureLogs(t)
	t.Setenv("AWS_EXPIRY_TIME", "not-a-timestamp")

	if got := GetS3ExpiryTime(); got != nil {
		t.Errorf("GetS3ExpiryTime = %v, want nil for an unparseable value", got)
	}
	if !strings.Contains(logs.String(), "AWS_EXPIRY_TIME") {
		t.Errorf("expected a warning mentioning AWS_EXPIRY_TIME, got: %s", logs.String())
	}
}

// Regression: this function used to print debug text to stdout on every call,
// corrupting the output of anyone piping dispatch's results.
func TestGetS3ExpiryTimeDoesNotPollute(t *testing.T) {
	logs := captureLogs(t)
	t.Setenv("AWS_EXPIRY_TIME", time.Now().Format(time.RFC3339))

	GetS3ExpiryTime()

	if strings.Contains(logs.String(), "msg=e ") || strings.Contains(logs.String(), "msg=e2") {
		t.Errorf("debug scaffolding remains in the output: %s", logs.String())
	}
}

func TestNewS3CacheRejectsNonS3URI(t *testing.T) {
	_, err := NewS3Cache(context.Background(), "https://example.com/bucket")
	if err == nil {
		t.Fatal("expected an error for a non-s3 scheme, got nil")
	}
	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("error = %v, want it to mention the scheme", err)
	}
}
