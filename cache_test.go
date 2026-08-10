package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// NewFileCache must report an unusable root as an error rather than panicking.
func TestNewFileCacheReturnsErrorForUnusableRoot(t *testing.T) {
	// A regular file cannot contain the success/failure subdirectories.
	blocker := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setting up: %v", err)
	}

	_, err := NewFileCache(blocker)
	if err == nil {
		t.Fatal("expected an error for a root which is a regular file, got nil")
	}
}

func TestFileCacheRoundTripsSuccessAndFailure(t *testing.T) {
	cache, err := NewFileCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	ctx := context.Background()

	if err := cache.WriteSuccess(ctx, "marker-a", []byte("payload-a")); err != nil {
		t.Fatalf("WriteSuccess: %v", err)
	}
	got, err := cache.ReadSuccess(ctx, "marker-a")
	if err != nil {
		t.Fatalf("ReadSuccess: %v", err)
	}
	if string(got) != "payload-a" {
		t.Errorf("ReadSuccess = %q, want %q", got, "payload-a")
	}

	if _, err := cache.SuccessModTime(ctx, "marker-a"); err != nil {
		t.Errorf("SuccessModTime on an existing marker: %v", err)
	}
	if _, err := cache.FailureModTime(ctx, "marker-a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FailureModTime on a success-only marker = %v, want ErrNotFound", err)
	}
}

// Absent markers must report ErrNotFound so callers can distinguish
// "never run" from a real I/O failure.
func TestFileCacheModTimeReportsErrNotFound(t *testing.T) {
	cache, err := NewFileCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	ctx := context.Background()

	if _, err := cache.SuccessModTime(ctx, "absent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SuccessModTime = %v, want ErrNotFound", err)
	}
	if _, err := cache.FailureModTime(ctx, "absent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FailureModTime = %v, want ErrNotFound", err)
	}
}

func TestFileCacheModTimeReflectsWriteTime(t *testing.T) {
	cache, err := NewFileCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	ctx := context.Background()

	before := time.Now().Add(-time.Second)
	if err := cache.WriteFailure(ctx, "marker-b", []byte("boom")); err != nil {
		t.Fatalf("WriteFailure: %v", err)
	}
	mtime, err := cache.FailureModTime(ctx, "marker-b")
	if err != nil {
		t.Fatalf("FailureModTime: %v", err)
	}
	if mtime.Before(before) {
		t.Errorf("mtime %v predates the write (%v)", mtime, before)
	}
}
