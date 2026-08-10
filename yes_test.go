package dispatch

import (
	"io"
	"strings"
	"testing"
)

// Yes must present an endlessly repeating stream of Line. Reading it in small
// chunks must reconstruct exactly that stream: the previous implementation
// restarted from the beginning of Line on every short read, so long lines were
// truncated and the newline was never delivered.
func TestYesProducesRepeatingStreamAcrossShortReads(t *testing.T) {
	line := "hello-world\n"
	y := &Yes{Line: []byte(line)}

	buf := make([]byte, 5)
	var got strings.Builder
	for got.Len() < len(line)*3 {
		n, err := y.Read(buf)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if n == 0 {
			t.Fatal("Read returned 0 bytes without an error")
		}
		got.Write(buf[:n])
	}

	want := strings.Repeat(line, 4)
	if !strings.HasPrefix(want, got.String()) {
		t.Errorf("stream = %q, want a prefix of %q", got.String(), want)
	}
}

func TestYesFillsLargeBufferWithWholeRepetitions(t *testing.T) {
	y := &Yes{Line: []byte("ab\n")}
	buf := make([]byte, 9)

	n, err := y.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := string(buf[:n]); got != "ab\nab\nab\n" {
		t.Errorf("Read filled %q, want %q", got, "ab\nab\nab\n")
	}
}

// A consumer reading exactly one line at a time must see complete lines.
func TestYesDeliversCompleteLines(t *testing.T) {
	y := &Yes{Line: []byte("value\n")}
	buf := make([]byte, 6)

	for i := range 3 {
		n, err := y.Read(buf)
		if err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}
		if got := string(buf[:n]); got != "value\n" {
			t.Errorf("Read %d = %q, want %q", i, got, "value\n")
		}
	}
}

// An empty Line would otherwise spin forever returning 0 bytes.
func TestYesWithEmptyLineReportsEOF(t *testing.T) {
	y := &Yes{Line: nil}
	buf := make([]byte, 4)

	n, err := y.Read(buf)
	if n != 0 || err != io.EOF {
		t.Errorf("Read = (%d, %v), want (0, EOF)", n, err)
	}
}

// io.Copy exercises the reader the way os/exec does when wiring up stdin.
func TestYesIsCompatibleWithIoCopy(t *testing.T) {
	y := &Yes{Line: []byte("x\n")}
	var sink strings.Builder

	if _, err := io.CopyN(&sink, y, 10); err != nil {
		t.Fatalf("CopyN: %v", err)
	}
	if got := sink.String(); got != strings.Repeat("x\n", 5) {
		t.Errorf("CopyN produced %q", got)
	}
}
