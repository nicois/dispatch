package dispatch

import (
	"bufio"
	"context"
	"errors"
	"io"
	"iter"
	"strings"
)

func LineReader(reader io.Reader, cancel context.CancelCauseFunc) iter.Seq[string] {
	return func(yield func(string) bool) {
		r := bufio.NewReader(reader)
		for {
			text, err := r.ReadString('\n')
			// Strip the line ending, tolerating CRLF as well as LF.
			text = strings.TrimRight(text, "\n")
			text = strings.TrimRight(text, "\r")

			// A non-nil err may still be accompanied by a final unterminated
			// line, so always emit what was read before acting on the error.
			if len(text) > 0 {
				if !yield(text) {
					return
				}
			}

			if err != nil {
				// EOF simply means the input is exhausted; anything else means
				// the caller's input was truncated and should be reported.
				if !errors.Is(err, io.EOF) && cancel != nil {
					cancel(err)
				}
				return
			}
		}
	}
}
