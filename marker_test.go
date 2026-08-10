package dispatch

import (
	"strings"
	"testing"
)

func mustCommand(t *testing.T, args ...string) RenderedCommand {
	t.Helper()
	cmd, err := NewRenderedCommand(args...)
	if err != nil {
		t.Fatalf("NewRenderedCommand: %v", err)
	}
	return cmd
}

// The marker is a cache key, so it must be stable across releases: changing it
// silently orphans every result a user has already cached. These are the
// values produced by the original implementation.
func TestMarkerIsBackwardsCompatible(t *testing.T) {
	tests := []struct {
		name string
		cmd  RenderedCommand
		want string
	}{
		{
			name: "no input",
			cmd:  mustCommand(t, "echo", "value is 1"),
			want: "9bfdb2668ac9919e0db1.zstd",
		},
		{
			name: "with input",
			cmd:  mustCommand(t, "rm", "-f", "foo.1").WithInput("y"),
			want: "ab8b937c790098be3e55.zstd",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Marker(tc.cmd); got != tc.want {
				t.Errorf("Marker = %s, want %s (changing this invalidates existing caches)", got, tc.want)
			}
		})
	}
}

func TestMarkerDistinguishesDifferentCommands(t *testing.T) {
	a := Marker(mustCommand(t, "echo", "one"))
	b := Marker(mustCommand(t, "echo", "two"))
	if a == b {
		t.Errorf("distinct commands share the marker %s", a)
	}
}

// Argument boundaries must be significant, so that ["a","bc"] and ["ab","c"]
// do not collide.
func TestMarkerRespectsArgumentBoundaries(t *testing.T) {
	a := Marker(mustCommand(t, "cmd", "a", "bc"))
	b := Marker(mustCommand(t, "cmd", "ab", "c"))
	if a == b {
		t.Errorf("commands with different argument boundaries share the marker %s", a)
	}
}

func TestMarkerDistinguishesDifferentInput(t *testing.T) {
	a := Marker(mustCommand(t, "cmd").WithInput("yes"))
	b := Marker(mustCommand(t, "cmd").WithInput("no"))
	if a == b {
		t.Errorf("distinct inputs share the marker %s", a)
	}
}

// An explicitly empty input produces a different job from no input at all,
// so the two must not share a cache entry.
func TestMarkerDistinguishesEmptyInputFromNoInput(t *testing.T) {
	none := Marker(mustCommand(t, "cmd"))
	empty := Marker(mustCommand(t, "cmd").WithInput(""))
	if none == empty {
		t.Errorf("empty input and absent input share the marker %s", none)
	}
}

func TestMarkerIsDeterministic(t *testing.T) {
	cmd := mustCommand(t, "echo", "hello").WithInput("x")
	first := Marker(cmd)
	for range 5 {
		if got := Marker(cmd); got != first {
			t.Fatalf("Marker is not deterministic: %s then %s", first, got)
		}
	}
}

func TestMarkerHasExpectedShape(t *testing.T) {
	got := Marker(mustCommand(t, "echo"))
	if !strings.HasSuffix(got, ".zstd") {
		t.Errorf("Marker = %q, want a .zstd suffix", got)
	}
	if len(got) != len("0123456789abcdef0123")+len(".zstd") {
		t.Errorf("Marker = %q, want 20 hex characters plus the suffix", got)
	}
}
