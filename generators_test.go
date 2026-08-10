package dispatch

import (
	"context"
	"strings"
	"testing"
)

func collectArgs(t *testing.T, g Generator, input string) ([]RenderArgs, error) {
	t.Helper()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	var cause error
	capture := func(err error) {
		cause = err
		cancel(err)
	}
	result := []RenderArgs{}
	for args := range g(ctx, capture, strings.NewReader(input)) {
		result = append(result, args)
	}
	return result, cause
}

func TestSimpleLineGenerator(t *testing.T) {
	got, err := collectArgs(t, SimpleLineGenerator, "one\ntwo")
	if err != nil {
		t.Fatalf("unexpected cancellation: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2: %v", len(got), got)
	}
	if got[0]["value"] != "one" || got[1]["value"] != "two" {
		t.Errorf("got %v, want value=one then value=two", got)
	}
}

func TestCsvGenerator(t *testing.T) {
	got, err := collectArgs(t, CsvGenerator, "animal,name\ncat,Scarface Claw\ndog,Bitzer Maloney\n")
	if err != nil {
		t.Fatalf("unexpected cancellation: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2: %v", len(got), got)
	}
	if got[0]["animal"] != "cat" || got[0]["name"] != "Scarface Claw" {
		t.Errorf("record 0 = %v", got[0])
	}
	if got[1]["animal"] != "dog" || got[1]["name"] != "Bitzer Maloney" {
		t.Errorf("record 1 = %v", got[1])
	}
}

// A row with fewer fields than the header must still produce a job, with the
// missing columns rendering as empty, rather than being dropped entirely.
func TestCsvGeneratorAcceptsShortRows(t *testing.T) {
	got, err := collectArgs(t, CsvGenerator, "a,b,c\n1,2\n")
	if err != nil {
		t.Fatalf("unexpected cancellation: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1: %v", len(got), got)
	}
	if got[0]["a"] != "1" || got[0]["b"] != "2" {
		t.Errorf("got %v, want a=1 b=2", got[0])
	}
	if v, ok := got[0]["c"]; !ok || v != "" {
		t.Errorf("missing column c = %q (present=%v), want an empty string", v, ok)
	}
}

// Extra fields beyond the header must not panic or drop the row.
func TestCsvGeneratorAcceptsLongRows(t *testing.T) {
	got, err := collectArgs(t, CsvGenerator, "a,b\n1,2,3\n")
	if err != nil {
		t.Fatalf("unexpected cancellation: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1: %v", len(got), got)
	}
	if got[0]["a"] != "1" || got[0]["b"] != "2" {
		t.Errorf("got %v, want a=1 b=2", got[0])
	}
}

func TestCsvGeneratorCancelsOnMissingHeader(t *testing.T) {
	_, err := collectArgs(t, CsvGenerator, "")
	if err == nil {
		t.Error("expected a cancellation cause for an empty CSV, got nil")
	}
}

func TestCsvGeneratorTrimsWhitespace(t *testing.T) {
	got, err := collectArgs(t, CsvGenerator, " animal , name \n cat , Claw \n")
	if err != nil {
		t.Fatalf("unexpected cancellation: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	if got[0]["animal"] != "cat" || got[0]["name"] != "Claw" {
		t.Errorf("got %v, want trimmed keys and values", got[0])
	}
}

func TestJsonLineGenerator(t *testing.T) {
	got, err := collectArgs(t, JsonLineGenerator, `{"animal":"cat"}`+"\n"+`{"animal":"dog"}`)
	if err != nil {
		t.Fatalf("unexpected cancellation: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2: %v", len(got), got)
	}
	if got[0]["animal"] != "cat" || got[1]["animal"] != "dog" {
		t.Errorf("got %v", got)
	}
}

// One malformed JSON line must not abandon the remaining input.
func TestJsonLineGeneratorSkipsMalformedLines(t *testing.T) {
	got, err := collectArgs(t, JsonLineGenerator, `{"animal":"cat"}`+"\nnot json\n"+`{"animal":"dog"}`+"\n")
	if err != nil {
		t.Fatalf("a single bad line must not cancel the run, got: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2 (bad line skipped): %v", len(got), got)
	}
	if got[0]["animal"] != "cat" || got[1]["animal"] != "dog" {
		t.Errorf("got %v, want the two valid records", got)
	}
}
