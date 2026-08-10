package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Cache interface {
	WriteSuccess(ctx context.Context, marker string, data []byte) error
	WriteFailure(ctx context.Context, marker string, data []byte) error
	SuccessModTime(ctx context.Context, marker string) (time.Time, error)
	FailureModTime(ctx context.Context, marker string) (time.Time, error)
	ReadSuccess(ctx context.Context, marker string) ([]byte, error)
	ReadFailure(ctx context.Context, marker string) ([]byte, error)
}

var ErrNotFound = errors.New("not found")

type fileCache struct {
	root string
}

// NewFileCache returns a Cache which records job outcomes as files beneath root.
// The required subdirectories are created if they do not already exist.
func NewFileCache(root string) (Cache, error) {
	if err := os.MkdirAll(filepath.Join(root, "success"), 0o700); err != nil {
		return nil, fmt.Errorf("could not create the success cache directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "failure"), 0o700); err != nil {
		return nil, fmt.Errorf("could not create the failure cache directory: %w", err)
	}
	return &fileCache{root: root}, nil
}

func (f *fileCache) successPath(marker string) string {
	return filepath.Join(f.root, "success", marker)
}

func (f *fileCache) failurePath(marker string) string {
	return filepath.Join(f.root, "failure", marker)
}

func (f *fileCache) WriteSuccess(ctx context.Context, marker string, data []byte) error {
	return os.WriteFile(f.successPath(marker), data, 0644)
}

func (f *fileCache) WriteFailure(ctx context.Context, marker string, data []byte) error {
	return os.WriteFile(f.failurePath(marker), data, 0644)
}

func (f *fileCache) SuccessModTime(ctx context.Context, marker string) (time.Time, error) {
	return modTime(f.successPath(marker))
}

func (f *fileCache) FailureModTime(ctx context.Context, marker string) (time.Time, error) {
	return modTime(f.failurePath(marker))
}

// modTime reports when the given path was last written. A missing path yields
// ErrNotFound; any other failure is returned as-is, so that a genuine I/O
// problem is not mistaken for "this job has never been run".
func modTime(path string) (time.Time, error) {
	stat, err := os.Stat(path)
	if err == nil {
		return stat.ModTime(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return time.Time{}, ErrNotFound
	}
	return time.Time{}, err
}

func (f *fileCache) ReadSuccess(ctx context.Context, marker string) ([]byte, error) {
	return os.ReadFile(f.successPath(marker))
}

func (f *fileCache) ReadFailure(ctx context.Context, marker string) ([]byte, error) {
	return os.ReadFile(f.failurePath(marker))
}
