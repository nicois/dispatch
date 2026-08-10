package dispatch

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

// Compress input to output.
func Compress(in io.Reader, out io.Writer) error {
	enc, err := zstd.NewWriter(out)
	if err != nil {
		return err
	}
	_, err = io.Copy(enc, in)
	if err != nil {
		_ = enc.Close()
		return err
	}
	return enc.Close()
}

// Decompress reverses Compress, writing the original bytes to out. It is the
// counterpart needed to read back the job output stored in a Cache.
func Decompress(in io.Reader, out io.Writer) error {
	dec, err := zstd.NewReader(in)
	if err != nil {
		return err
	}
	defer dec.Close()
	_, err = io.Copy(out, dec.IOReadCloser())
	return err
}
