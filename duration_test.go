package dispatch

import (
	"testing"
	"time"
)

func TestDurationUnmarshalFlag(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"5s", 5 * time.Second},
		{"1m30s", 90 * time.Second},
		{"500ms", 500 * time.Millisecond},
		{"2h", 2 * time.Hour},
		{"1d", 24 * time.Hour},
		{"3d", 72 * time.Hour},
		{"10d", 240 * time.Hour},
		{"1d12h", 36 * time.Hour},
		{"2d30m", 48*time.Hour + 30*time.Minute},
		{"0s", 0},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			var d Duration
			if err := d.UnmarshalFlag(tc.input); err != nil {
				t.Fatalf("UnmarshalFlag(%q): %v", tc.input, err)
			}
			if time.Duration(d) != tc.want {
				t.Errorf("UnmarshalFlag(%q) = %v, want %v", tc.input, time.Duration(d), tc.want)
			}
		})
	}
}

func TestDurationUnmarshalFlagRejectsInvalid(t *testing.T) {
	// An empty value used to panic: v[0] indexes an empty string.
	for _, input := range []string{"", "abc", "5x", "d", "-", "1d1x"} {
		t.Run("input="+input, func(t *testing.T) {
			var d Duration
			if err := d.UnmarshalFlag(input); err == nil {
				t.Errorf("UnmarshalFlag(%q) = %v, want an error", input, time.Duration(d))
			}
		})
	}
}

func TestFriendlyDuration(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"sub-second", 500 * time.Millisecond, "500 milliseconds"},
		{"seconds", 30 * time.Second, "30 seconds"},
		{"minutes with decimal", 5 * time.Minute, "5.0 minutes"},
		{"minutes", 30 * time.Minute, "30 minutes"},
		{"hours with decimal", 2 * time.Hour, "2.0 hours"},
		{"hours", 10 * time.Hour, "10 hours"},
		{"days", 5 * 24 * time.Hour, "5 days"},
		// Regression: the years branch divided by 3600/365.25, omitting the
		// hours-per-day factor, so this reported "65.7 years".
		{"years", 1000 * 24 * time.Hour, "2.7 years"},
		{"many years", 5 * 365 * 24 * time.Hour, "5.0 years"},
		// A negative estimate is nonsense to show a user.
		{"negative clamps to zero", -58 * time.Millisecond, "0 milliseconds"},
		{"zero", 0, "0 milliseconds"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FriendlyDuration(tc.in); got != tc.want {
				t.Errorf("FriendlyDuration(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Whatever the magnitude, the output must never contain a minus sign.
func TestFriendlyDurationNeverNegative(t *testing.T) {
	for _, d := range []time.Duration{
		-time.Millisecond, -time.Second, -time.Hour, -1000 * time.Hour,
	} {
		if got := FriendlyDuration(d); got[0] == '-' {
			t.Errorf("FriendlyDuration(%v) = %q, which is negative", d, got)
		}
	}
}
