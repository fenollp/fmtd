package buildx

import (
	"io"
	"os"
)

// OverwriteFileContents replaces the contents of a file with the data
// from the given reader. File attributes are not changed except modtime.
var OverwriteFileContents OutputFileFunc = func(filename string, r io.Reader) error {
	f, err := os.OpenFile(filename, os.O_RDWR, 0) // already exists
	if err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

// ErrOutputFileFuncSet is returned on multiple calls to WithOutputFileFunc(f) where f != nil
var ErrOutputFileFuncSet = errors.New("cannot reset OutputFileFunc")

// OutputFileFunc represents an effectful function set using WithOutputFileFunc.
type OutputFileFunc func(filename string, r io.Reader) error

// WithOutputFileFunc is executed per file outputed by the build.
func WithOutputFileFunc(f OutputFileFunc) Option {
	return func(o *options) error {
		if o.ofilefunc != nil && f != nil {
			return ErrOutputFileFuncSet
		}
		o.ofilefunc = f
		return nil
	}
}
