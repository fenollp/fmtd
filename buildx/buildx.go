package buildx

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoDocker is returned when no usable Docker client can be found
var ErrNoDocker = errors.New("No docker client found: curl -fsSL https://get.docker.com -o get-docker.sh && sudo sh get-docker.sh")

// ErrDockerBuildFailure is returned when docker build failed
var ErrDockerBuildFailure = errors.New("docker build failed with status 1")

// Option represents the various arguments a build takes
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
		o.args = append(o.args, "--build-arg="+arg)
		return nil
	}
}

// ErrNoDockerfile is returned when WithDockerfile() wasn't called.
var ErrNoDockerfile = errors.New("missing Dockerfile")

// WithDockerfile have build run with given Dockerfile.
func WithDockerfile(dockerfile []byte) Option {
	return func(o *options) error {
		o.dockerfile = dockerfile
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

// WithOutputFileFunc is executed per file outputed by the build.
func WithOutputFileFunc(f func(filename string, r io.Reader) error) Option {
	return func(o *options) error {
		o.ofilefunc = f
		return nil
	}
}

type options struct {
	ctx            context.Context
	stdout, stderr io.Writer
	env            []string
	exe            string
	args           []string
	dockerfile     []byte
	stdoutf        string
	dirA, dirB     string
	ifiles         []inputfile
	ofilefunc      func(string, io.Reader) error
}

// New calls `DOCKER_BUILDKIT=1 docker build ...` with given Dockerfile
func New(opts ...Option) (err error) {
	o := &options{
		ctx:        context.Background(),
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		env:        os.Environ(),
		exe:        "",
		args:       []string{"build", "--output=-"},
		dockerfile: nil,
		stdoutf:    "stdout",
		dirA:       "a",
		dirB:       "b",
		ifiles:     nil,
		ofilefunc:  nil,
	}

	for _, opt := range opts {
		if err = opt(o); err != nil {
			return
		}
	}

	if o.exe == "" {
		exe, err := exec.LookPath("docker")
		if err != nil {
			return ErrNoDocker
		}
		o.exe = exe
	}

	if len(o.dockerfile) == 0 {
		return ErrNoDockerfile
	}

	var stdin bytes.Buffer
	tw := tar.NewWriter(&stdin)
	{
		hdr := &tar.Header{
			Name: "Dockerfile",
			Mode: 0200,
			Size: int64(len(o.dockerfile)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(o.dockerfile); err != nil {
			return err
		}
	}
	for _, ifile := range o.ifiles {
		hdr := &tar.Header{
			Name: filepath.Join(o.dirA, ifile.filename),
			Mode: 0600,
			Size: int64(len(ifile.data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(ifile.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}

	o.args = append(o.args, "-")
	cmd := exec.CommandContext(o.ctx, o.exe, o.args...)
	cmd.Env = append(o.env, "DOCKER_BUILDKIT=1")
	cmd.Stdin = &stdin
	var tarbuf bytes.Buffer
	cmd.Stdout = &tarbuf
	cmd.Stderr = o.stderr
	if err := cmd.Run(); err != nil {
		if err.Error() == "exit status 1" {
			return ErrDockerBuildFailure
		}
		return err
	}

	tr := tar.NewReader(&tarbuf)
	var stdoutf bytes.Buffer
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return err
		}
		if strings.HasSuffix(hdr.Name, "/") {
			continue
		}
		if hdr.Name == o.stdoutf {
			if _, err := io.Copy(&stdoutf, tr); err != nil { // show later
				return err
			}
			continue
		}
		if f := o.ofilefunc; f != nil {
			filename := strings.TrimPrefix(hdr.Name, o.dirB+"/")
			if err := o.ofilefunc(filename, tr); err != nil {
				return err
			}
		}
	}

	if _, err := io.Copy(o.stdout, &stdoutf); err != nil {
		return err
	}

	return nil
}
