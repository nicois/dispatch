package dispatch

import "io"

// Yes is an io.Reader which repeats Line forever, in the manner of the `yes`
// command. It is used to feed a fixed value to each job's STDIN.
//
// Yes is stateful: it must be used via a pointer, and a distinct instance is
// needed per job.
type Yes struct {
	Line []byte

	// offset records how far into Line the previous read finished, so that a
	// buffer too small to hold a whole Line does not truncate it.
	offset int
}

func (y *Yes) Read(p []byte) (int, error) {
	if len(y.Line) == 0 {
		// Without this, a caller would spin forever on (0, nil).
		return 0, io.EOF
	}
	var written int
	for written < len(p) {
		n := copy(p[written:], y.Line[y.offset:])
		written += n
		y.offset += n
		if y.offset >= len(y.Line) {
			y.offset = 0
		}
	}
	return written, nil
}
