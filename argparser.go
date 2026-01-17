package dispatch

import (
	"context"
	"fmt"
	"io"
	"iter"
	"strings"
	"text/template"
)

type RenderedCommand struct {
	command []string
	input   string
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

	}
	return result, nil
}
