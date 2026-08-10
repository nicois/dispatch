package dispatch

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func collect(seq func(func(string) bool)) []string {
	result := []string{}
	for s := range seq {
		result = append(result, s)
	}
	return result
}

func TestLineReader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "trailing newline",
			input: "one\ntwo\nthree\n",
			want:  []string{"one", "two", "three"},
		},
		{
			// Regression: the final line used to be discarded, because
			// ReadString returns it together with io.EOF.
			name:  "no trailing newline",
			input: "one\ntwo\nthree",
			want:  []string{"one", "two", "three"},
		},
		{
			name:  "single line without newline",
			input: "only",
			want:  []string{"only"},
		},
		{
			name:  "empty input",
			input: "",
			want:  []string{},
		},
		{
			name:  "blank lines are skipped",
			input: "one\n\n\ntwo\n",
			want:  []string{"one", "two"},
		},
		{
			name:  "trailing blank line is not emitted",
			input: "one\n\n",
			want:  []string{"one"},
		},
		{
			name:  "carriage returns are stripped",
			input: "one\r\ntwo\r\n",
			want:  []string{"one", "two"},
		},
		{
			name:  "final line without newline after a blank line",
			input: "one\n\nlast",
			want:  []string{"one", "last"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := collect(LineReader(strings.NewReader(tc.input), nil))
			if len(got) != len(tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("line %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// A line longer than bufio's default buffer must survive intact.
func TestLineReaderHandlesVeryLongLine(t *testing.T) {
	long := strings.Repeat("x", 128*1024)
	got := collect(LineReader(strings.NewReader(long+"\n"), nil))
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1", len(got))
	}
	if got[0] != long {
		t.Errorf("got a line of length %d, want %d", len(got[0]), len(long))
	}
}

type failingReader struct {
	data string
	err  error
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.data == "" {
		return 0, f.err
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

// A genuine read error (as opposed to EOF) must cancel the context so the
// caller learns the input stream was truncated.
func TestLineReaderCancelsOnReadError(t *testing.T) {
	sentinel := errors.New("disk exploded")
	_, cancel := context.WithCancelCause(context.Background())
	var cause error
	captured := func(err error) {
		cause = err
		cancel(err)
	}

	got := collect(LineReader(&failingReader{data: "one\n", err: sentinel}, captured))

	if len(got) != 1 || got[0] != "one" {
		t.Errorf("got %q, want [one]", got)
	}
	if !errors.Is(cause, sentinel) {
		t.Errorf("cancellation cause = %v, want %v", cause, sentinel)
	}
}

// EOF is a normal end of stream, not an error worth cancelling over.
func TestLineReaderDoesNotCancelOnEOF(t *testing.T) {
	cancelled := false
	captured := func(err error) { cancelled = true }

	got := collect(LineReader(&failingReader{data: "one\ntwo", err: io.EOF}, captured))

	if len(got) != 2 {
		t.Fatalf("got %q, want two lines", got)
	}
	if cancelled {
		t.Error("EOF must not cancel the context")
	}
}
