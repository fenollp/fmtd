package buildx

import (
	"context"
	"errors"
	"io"
)

// Option represents the various arguments a New takes
type Option func(*options) error

// ErrNilContext is returned when a nil context with given as option
var ErrNilContext = errors.New("context is nil")

// WithContext have build run with cancellable context.
// Defaults to context.Background()
func WithContext(ctx context.Context) Option {
	return func(o *options) error {
		if ctx == nil {
			return ErrNilContext
		}
		o.ctx = ctx
		return nil
	}
}

// WithStdout have build run with STDOUT written to given io.Writer.
// Defaults to os.Stdout
func WithStdout(stdout io.Writer) Option {
	return func(o *options) error {
		o.stdout = stdout
		return nil
	}
}

// WithStderr have build run with STDERR written to given io.Writer.
// Defaults to os.Stderr
func WithStderr(stderr io.Writer) Option {
	return func(o *options) error {
		o.stderr = stderr
		return nil
	}
}

// WithEnviron have build run with environment variables coming from environ.
// Note this is the env outside Docker.
// Defaults to os.Environ()
func WithEnviron(environ []string) Option {
	return func(o *options) error {
		o.env = environ
		return nil
	}
}

// WithExecutable have build run with given Docker executable path.
// Defaults to being resolved with exec.Lookpath("docker").
func WithExecutable(exe string) Option {
	return func(o *options) error {
		o.exe = exe
		return nil
	}
}

// WithBuildArg have build run with given build argument in key=value format.
// Multiple calls append build args.
func WithBuildArg(arg string) Option {
	return func(o *options) error {
		if arg != "" {
			o.args = append(o.args, "--build-arg="+arg)
		}
		return nil
	}
}

// ErrNoDockerfile is returned when WithDockerfile() wasn't called.
var ErrNoDockerfile = errors.New("missing Dockerfile")

// WithDockerfile have build run with given Dockerfile.
func WithDockerfile(dockerfiler func(map[interface{}]interface{}) []byte) Option {
	return func(o *options) error {
		o.dockerfiler = dockerfiler
		return nil
	}
}

// ErrEmptyStdoutFile is returned when WithStdoutFile("") was called.
var ErrEmptyStdoutFile = errors.New("empty stdout file")

// WithStdoutFile have build run with given stdout file.
// Defaults to "stdout".
func WithStdoutFile(stdoutf string) Option {
	return func(o *options) error {
		if stdoutf == "" {
			return ErrEmptyStdoutFile
		}
		o.stdoutf = stdoutf
		return nil
	}
}

// ErrEmptyDirA is returned when WithDirectoryA("") was called.
var ErrEmptyDirA = errors.New("empty dir a")

// WithDirectoryA have build run with given "a" directory.
// Defaults to "a".
func WithDirectoryA(dirA string) Option {
	return func(o *options) error {
		if dirA == "" {
			return ErrEmptyDirA
		}
		o.dirA = dirA
		return nil
	}
}

// ErrEmptyDirB is returned when WithDirectoryB("") was called.
var ErrEmptyDirB = errors.New("empty dir b")

// WithDirectoryB have build run with given "b" directory.
// Defaults to "b".
func WithDirectoryB(dirB string) Option {
	return func(o *options) error {
		if dirB == "" {
			return ErrEmptyDirB
		}
		o.dirB = dirB
		return nil
	}
}

type inputfile struct {
	filename string
	data     []byte
}

// WithInputFile have build run with given input file copied in.
// Multiple calls add input files.
func WithInputFile(relativePath string, data []byte) Option {
	return func(o *options) error {
		o.ifiles = append(o.ifiles, inputfile{
			filename: relativePath,
			data:     data,
		})
		return nil
	}
}
