package dispatch

import (
	"context"
	"encoding/json"
	"io"
	"iter"
	"log/slog"
)

func JsonLineGenerator(ctx context.Context, cancel context.CancelCauseFunc, in io.Reader) iter.Seq[RenderArgs] {
	return func(yield func(RenderArgs) bool) {
		for text := range LineReader(in, cancel) {
			result := make(map[string]string)
			err := json.Unmarshal([]byte(text), &result)
			if err != nil {
				// Treat a malformed line the same way the CSV generator does:
				// warn and carry on, so one bad record does not discard the
				// rest of what may be a very long input stream.
				logger.Warn("could not parse a line as JSON",
					slog.String("line", text),
					slog.Any("error", err))
				continue
			}
			if !yield(result) {
				return
			}
		}
	}
}
