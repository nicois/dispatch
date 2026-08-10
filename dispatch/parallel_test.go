package main

import (
	"io"
	"strings"
	"testing"
)

// cliRun invokes the CLI entrypoint against a temporary cache.
func cliRun(t *testing.T, stdin string, args ...string) (int, string) {
	t.Helper()
	var out strings.Builder
	full := append([]string{"--cache-location", t.TempDir()}, args...)
	code := run(full, strings.NewReader(stdin), &out, io.Discard)
	return code, out.String()
}

func TestExitsZeroWhenAllJobsSucceed(t *testing.T) {
	code, out := cliRun(t, "1\n2\n", "--", "true")
	if code != exitSuccess {
		t.Errorf("exit code = %d, want %d; output:\n%s", code, exitSuccess, out)
	}
}

// Regression: a failing job used to be invisible to the caller, which had to
// parse the logs to notice.
func TestExitsNonzeroWhenJobsFail(t *testing.T) {
	code, out := cliRun(t, "1\n2\n", "--", "false")
	if code != exitJobsFailed {
		t.Errorf("exit code = %d, want %d; output:\n%s", code, exitJobsFailed, out)
	}
}

func TestExitsNonzeroForInvalidConcurrency(t *testing.T) {
	// Zero used to run nothing and report success; negative used to panic.
	for _, value := range []string{"0", "-1"} {
		code, out := cliRun(t, "1\n", "--concurrency", value, "--", "true")
		if code != exitFailure {
			t.Errorf("--concurrency %s: exit code = %d, want %d; output:\n%s",
				value, code, exitFailure, out)
		}
	}
}

func TestExitsNonzeroForUnknownFlag(t *testing.T) {
	var out strings.Builder
	if code := run([]string{"--no-such-flag"}, strings.NewReader(""), &out, io.Discard); code != exitFailure {
		t.Errorf("exit code = %d, want %d", code, exitFailure)
	}
}

func TestPartialFailureStillReportsNonzero(t *testing.T) {
	// The final job fails, so the overall run must not report success.
	code, out := cliRun(t, "0\n0\n1\n", "--", "sh", "-c", "exit {{.value}}")
	if code != exitJobsFailed {
		t.Errorf("exit code = %d, want %d; output:\n%s", code, exitJobsFailed, out)
	}
}

func TestDryRunExitsZero(t *testing.T) {
	code, out := cliRun(t, "1\n", "--dry-run", "--", "false")
	if code != exitSuccess {
		t.Errorf("exit code = %d, want %d; output:\n%s", code, exitSuccess, out)
	}
}

// The default no-command behaviour echoes its input, and must still succeed.
func TestNoCommandEchoesInput(t *testing.T) {
	code, out := cliRun(t, "1\n")
	if code != exitSuccess {
		t.Errorf("exit code = %d, want %d; output:\n%s", code, exitSuccess, out)
	}
	if !strings.Contains(out, "no command was provided") {
		t.Errorf("expected a notice about the default command; output:\n%s", out)
	}
}

// Regression: --rate-limit rejected the 'd' suffix accepted by every other
// duration flag.
func TestRateLimitAcceptsDaySuffix(t *testing.T) {
	var out strings.Builder
	code := run([]string{"--cache-location", t.TempDir(), "--rate-limit", "1d", "--help"},
		strings.NewReader(""), &out, io.Discard)
	if code != exitSuccess {
		t.Errorf("--rate-limit 1d was rejected: exit code = %d; output:\n%s", code, out.String())
	}
}

func TestRateLimitRejectsSubMillisecond(t *testing.T) {
	code, out := cliRun(t, "1\n", "--rate-limit", "1us", "--", "true")
	if code != exitFailure {
		t.Errorf("exit code = %d, want %d; output:\n%s", code, exitFailure, out)
	}
}

// A malformed template is a configuration error, not a job failure.
func TestExitsNonzeroForMalformedTemplate(t *testing.T) {
	code, out := cliRun(t, "1\n", "--", "echo", "{{.unclosed")
	if code != exitFailure {
		t.Errorf("exit code = %d, want %d; output:\n%s", code, exitFailure, out)
	}
}
