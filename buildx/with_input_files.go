package buildx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// InputFilesOption represents the various arguments WithInputFiles takes.
type InputFilesOption func(*inputfilesoptions)

// WithFilenames sets which filenames to use. Each call resets the previous setting.
func WithFilenames(paths []string) InputFilesOption {
	return func(oo *inputfilesoptions) { oo.filenames = paths }
}

// WithUseCurrentDirWhenNoPathsGiven will use $PWD if WithFilenames(l) l == 0.
// Automatically activates WithTraverseDirectories(true).
func WithUseCurrentDirWhenNoPathsGiven() InputFilesOption {
	return func(oo *inputfilesoptions) {
		oo.emptyusePWD = true
		oo.traversedirs = true
	}
}

// WithTraverseDirectories activates directory traversal if given true.
// Does nothing is WithUseCurrentDirWhenNoPathsGiven() was called.
func WithTraverseDirectories(dotraverse bool) InputFilesOption {
	return func(oo *inputfilesoptions) {
		if !oo.emptyusePWD {
			oo.traversedirs = dotraverse
		}
	}
}

// WithEnsureUnderPWD makes sure selected paths all are under $PWD
func WithEnsureUnderPWD(doensure bool) InputFilesOption {
	return func(oo *inputfilesoptions) { oo.under = doensure }
}

// WithSelectionFailureBuilder can be used to wrap errors that correspond to filename selection failures.
func WithSelectionFailureBuilder(f func(fn string, err error) error) InputFilesOption {
	return func(oo *inputfilesoptions) { oo.errer = f }
}

// WithPWD sets the current working directory.
func WithPWD(pwd string) InputFilesOption {
	return func(oo *inputfilesoptions) { oo.pwd = pwd }
}

// WithEnsureWritable makes sure selected files are all writable
func WithEnsureWritable(doensure bool) InputFilesOption {
	return func(oo *inputfilesoptions) { oo.writable = doensure }
}

type inputfilesoptions struct {
	filenames                                  []string
	emptyusePWD, traversedirs, under, writable bool
	errer                                      func(fn string, err error) error
	pwd                                        string
}

// ErrEmptyPWDForInputFiles is returned when calling WithInputFiles missing WithPWD(pwd) and pwd != "".
var ErrEmptyPWDForInputFiles = errors.New("given empty $PWD")

// WithInputFiles have build run with given input files copied in.
func WithInputFiles(opts ...InputFilesOption) Option {
	oo := &inputfilesoptions{
		filenames:    nil,
		emptyusePWD:  false,
		traversedirs: false,
		under:        false,
		errer:        func(fn string, err error) error { return err },
	}
	for _, opt := range opts {
		opt(oo)
	}
	return func(o *options) error {
		if oo.pwd == "" {
			return ErrEmptyPWDForInputFiles
		}

		filenames := oo.filenames
		if oo.emptyusePWD && len(filenames) == 0 {
			filenames = append(filenames, oo.pwd)
		}

		fns := make([]string, 0, len(filenames))
		var moreFns []string
		for _, filename := range filenames {
			additional, err := oo.ensureRegular(filename)
			if err != nil {
				return err
			}
			if len(additional) != 0 {
				moreFns = append(moreFns, additional...)
			} else {
				fns = append(fns, filename)
				if oo.under {
					if err := oo.ensureUnder(filename); err != nil {
						return err
					}
				}
				if oo.writable {
					if err := oo.ensureWritable(filename); err != nil {
						return err
					}
				}
			}
		}
		o.foundFilenamesByTraversingDirs = len(moreFns) != 0
		filenames = uniqueSorted(append(fns, moreFns...))

		for _, filename := range filenames {
			data, err := os.ReadFile(filename)
			if err != nil {
				return oo.errer(filename, err)
			}
			if err := WithInputFile(filename, data)(o); err != nil {
				return err
			}
		}

		return nil
	}
}

func (oo *inputfilesoptions) ensureUnder(fn string) (err error) {
	if filepath.VolumeName(fn) != filepath.VolumeName(oo.pwd) {
		return oo.errer(fn, errors.New("not on $PWD's volume"))
	}
	if fn[0] == '.' || filepath.IsAbs(fn) {
		var fnabs string
		if fnabs, err = filepath.Abs(fn); err != nil {
			return oo.errer(fn, err)
		}
		if !filepath.HasPrefix(fnabs, oo.pwd) {
			return oo.errer(fn, errors.New("not under $PWD"))
		}
	}
	return
}

func (oo *inputfilesoptions) ensureWritable(fn string) error {
	f, err := os.OpenFile(fn, os.O_RDWR, 0200)
	if err != nil {
		return oo.errer(fn, err.(*fs.PathError).Unwrap())
	}
	if err := f.Close(); err != nil {
		return oo.errer(fn, err)
	}
	return nil
}

func (oo *inputfilesoptions) ensureRegular(fn string) ([]string, error) {
	if fi, err := os.Lstat(fn); err != nil {
		return nil, oo.errer(fn, err.(*fs.PathError).Unwrap())
	} else if fi.Mode().IsRegular() {
		return nil, nil
	} else if oo.traversedirs && fi.IsDir() {
		var filenames []string
		if err := filepath.WalkDir(fn, func(path string, d fs.DirEntry, err error) error {
			if name := d.Name(); name != "" && name[0] == '.' { // skip hidden files
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			if oo.writable {
				if err := oo.ensureWritable(path); err != nil {
					return err
				}
			}
			filenames = append(filenames, path)
			return nil
		}); err != nil {
			return nil, err
		}
		return filenames, nil
	}
	return nil, oo.errer(fn, errors.New("not a regular file"))
}

func uniqueSorted(xs []string) []string {
	uniq := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		uniq[x] = struct{}{}
	}
	xs = make([]string, 0, len(uniq))
	for x := range uniq {
		xs = append(xs, x)
	}
	sort.Strings(xs)
	return xs
}
