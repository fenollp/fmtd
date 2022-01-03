package buildx

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type options struct {
	ctx            context.Context
	stdout, stderr io.Writer
	env            []string
	exe            string
	args           []string
	dockerfiler    func(map[interface{}]interface{}) []byte
	stdoutf        string
	dirA, dirB     string
	ifiles         []inputfile
	ofilefunc      OutputFileFunc

	foundFilenamesByTraversingDirs bool
}

// New calls `DOCKER_BUILDKIT=1 docker build ...` with given Dockerfile
func New(opts ...Option) (err error) {
	o := &options{
		ctx:         context.Background(),
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		env:         os.Environ(),
		exe:         "",
		args:        []string{"build", "--output=-"},
		dockerfiler: nil,
		stdoutf:     "stdout",
		dirA:        "a",
		dirB:        "b",
		ifiles:      nil,
		ofilefunc:   nil,
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

	dockerfile := o.dockerfiler(map[interface{}]interface{}{
		"foundFilenamesByTraversingDirs": o.foundFilenamesByTraversingDirs,
	})
	if len(dockerfile) == 0 {
		return ErrNoDockerfile
	}

	var stdin bytes.Buffer
	tw := tar.NewWriter(&stdin)
	{
		hdr := &tar.Header{
			Name: "Dockerfile",
			Mode: 0200,
			Size: int64(len(dockerfile)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(dockerfile); err != nil {
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
