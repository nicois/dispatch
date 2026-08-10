package dispatch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"text/template"
)

// RenderedCommand is a single job: a fully-substituted argument vector, plus
// the optional text to be repeated on the job's STDIN.
type RenderedCommand struct {
	command []string
	input   string
	// hasInput distinguishes an explicitly empty input from no input at all.
	hasInput bool
}

// NewRenderedCommand builds a job to be executed, for callers using this
// package as a library rather than via the command line. args must be
// non-empty; args[0] is the executable.
func NewRenderedCommand(args ...string) (RenderedCommand, error) {
	if len(args) == 0 {
		return RenderedCommand{}, errors.New("a command requires at least one argument")
	}
	return RenderedCommand{command: slices.Clone(args)}, nil
}

// WithInput returns a copy of the command which feeds input, followed by a
// newline, repeatedly to the job's STDIN. An empty string is honoured, and is
// distinct from leaving the input unset.
func (r RenderedCommand) WithInput(input string) RenderedCommand {
	r.command = slices.Clone(r.command)
	r.input = input
	r.hasInput = true
	return r
}

// Args returns the command and its arguments.
func (r RenderedCommand) Args() []string {
	return slices.Clone(r.command)
}

// Input reports the STDIN text and whether any was configured.
func (r RenderedCommand) Input() (string, bool) {
	return r.input, r.hasInput
}

type (
	RenderArgs map[string]string
)

// Generator will process the incoming data stream, generating rendered commands
// until either it runs out of input or the context is cancelled. If a fatal error
// occurs which prevents continuing to process the data stream, cancel the context and exit.
// Non-fatal errors should return in an empty command being returned (as well as logging the error)
type Generator func(context.Context, context.CancelCauseFunc, io.Reader) iter.Seq[RenderArgs]

func ParseCommandline(command []string) ([]*template.Template, error) {
	result := make([]*template.Template, len(command))
	for i, part := range command {
		if t, err := template.New("ArgParser").Parse(part); err == nil {
			result[i] = t
		} else {
			return nil, err
		}
	}
	return result, nil
}

func Render(command []*template.Template, input *template.Template, args RenderArgs) (RenderedCommand, error) {
	result := RenderedCommand{command: make([]string, 0, len(command))}
	for _, part := range command {
		var sb strings.Builder
		err := part.Execute(&sb, args)
		if err != nil {
			return result, fmt.Errorf("could not render %v with %q: %w", part, args, err)
		}
		result.command = append(result.command, sb.String())
	}
	if input != nil {
		var sb strings.Builder
		err := input.Execute(&sb, args)
		if err != nil {
			return result, fmt.Errorf("could not render %v with %q: %w", input, args, err)
		}
		result.input = sb.String()
		result.hasInput = true
	}
	return result, nil
}
