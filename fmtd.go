package fmtd

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoDocker is returned when no usable Docker client can be found
var ErrNoDocker = errors.New("No docker client found: curl -fsSL https://get.docker.com -o get-docker.sh && sudo sh get-docker.sh")

// ErrDryRunFoundFiles is returned when a run would have modified files if it weren't for dryrun
var ErrDryRunFoundFiles = errors.New("unformatted files found")

func unusable(fn string, err error) error {
	return fmt.Errorf("unusable file %q (%v)", fn, err)
}

func dockerfile() []byte {
	patterns := []string{
		// C / C++ / Protocol Buffers / Objective-C / Objective-C++
		"*.c", "*.cc", "*.cpp", "*.h", "*.hh", "*.proto", "*.m", "*.mm",
		// JSON
		"*.json",
		// YAML
		// TODO: "*.yaml", "*.yml",
		// Python
		"*.py",
		// Erlang
		// TODO: "*.erl",
		// Bazel / Skylark / Starlark
		"BUILD", "*.BUILD", "*.bzl", "*.sky", "*.star", "WORKSPACE",
		// Shell
		"*.sh",
		// SQL
		"*.sql",
		// Go
		// TODO: "*.go",
	}

	return []byte(`
# syntax=docker.io/docker/dockerfile:1@sha256:42399d4635eddd7a9b8a24be879d2f9a930d0ed040a61324cfdf59ef1357b3b2
`[1:] + `
FROM --platform=$BUILDPLATFORM docker.io/library/alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300 AS alpine
FROM --platform=$BUILDPLATFORM docker.io/whilp/buildifier@sha256:67da91fdddd40e9947153bc9157ab9103c141fcabcdbf646f040ba7a763bc531 AS buildifier
FROM --platform=$BUILDPLATFORM docker.io/unibeautify/clang-format@sha256:1b2d3997012ae221c600668802f1b761973d9006d330effa9555516432dea9c1 AS clang-format

# See https://github.com/Unibeautify/docker-beautifiers

FROM alpine AS tool
WORKDIR /app/b
WORKDIR /app/a
ARG YAPF_VERSION=0.31.0
ARG BEAUTYSH_VERSION=6.2.1
ARG SQLFORMAT_VERSION=0.4.2
RUN \
  --mount=type=cache,target=/var/cache/apk ln -vs /var/cache/apk /etc/apk/cache && \
    set -ux \
 && apk add --no-cache py3-pip clang emacs jq \
 && pip3 install \
      yapf=="$YAPF_VERSION" \
      beautysh=="$BEAUTYSH_VERSION" \
      sqlparse=="$SQLFORMAT_VERSION"
COPY --from=buildifier /usr/bin/buildifier /usr/bin/buildifier
COPY --from=clang-format /usr/bin/clang-format /usr/bin/clang-format

FROM tool AS product
COPY a /app/a/
RUN \
    set -ux \
 && while read -r f; do \
      mkdir -p ../b/"$(dirname "$f")" \
      && \
      case "$f" in \
        *.c|*.cc|*.cpp|*.h|*.hh|*.proto|*.m|*.mm) clang-format -style=google -sort-includes "$f" >../b/"$f";; \
        BUILD|*.BUILD|*.bzl|*.sky|*.star|WORKSPACE) cp "$f" ../b/"$f" && buildifier -lint=fix ../b/"$f" ;; \
        *.json) cat "$f" | jq -S --tab . >../b/"$f" ;; \
        *.py) yapf --style=google "$f" >../b/"$f" ;; \
        *.sh) beautysh --backup "$f" && mv "$f".bak ../b/"$f" ;; \
        *.sql) sqlformat "$f" >../b/"$f" ;; \
        *) echo "unhandled $f" ;; \
      esac \
      && \
      if diff -q "$f" ../b/"$f" >/dev/null; then rm ../b/"$f"; fi \
      ; \
   done < <(find . -type f \( -iname '` + strings.Join(patterns, "' -or -iname '") + `' \))

FROM scratch
COPY --from=product /app/b /
`)
}

// Fmt formats (any) files below the current directory
func Fmt(
	ctx context.Context,
	pwd string,
	dryrun bool,
	stdout, stderr io.Writer,
	filenames []string,
) error {
	exe, err := exec.LookPath("docker")
	if err != nil {
		return ErrNoDocker
	}

	if len(filenames) == 0 {
		return nil
	}
	files := make(map[string][]byte, len(filenames))
	for _, filename := range filenames {
		if _, ok := files[filename]; ok {
			continue
		}
		if err := ensureRegular(filename); err != nil { // TODO: expand directories
			return err
		}
		if err := ensureUnder(pwd, filename); err != nil {
			return err
		}
		if !dryrun {
			if err := ensureWritable(filename); err != nil {
				return err
			}
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return unusable(filename, err)
		}
		files[filename] = data
	}
	filenames = make([]string, 0, len(files))
	for filename := range files {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)

	var stdin bytes.Buffer
	tw := tar.NewWriter(&stdin)
	{
		data := dockerfile()
		hdr := &tar.Header{
			Name: "Dockerfile",
			Mode: 0200,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	for _, filename := range filenames {
		data := files[filename]
		hdr := &tar.Header{
			Name: filepath.Join("a", filename),
			Mode: 0600,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}

	var tarbuf bytes.Buffer
	cmd := exec.CommandContext(ctx, exe, "build", "--output=-", "-")
	cmd.Env = append(os.Environ(),
		"DOCKER_BUILDKIT=1",
	)
	cmd.Stdin = &stdin
	cmd.Stdout = &tarbuf
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	tr := tar.NewReader(&tarbuf)
	foundFiles := false
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
		fmt.Println(hdr.Name)
		if !dryrun {
			f, err := os.OpenFile(hdr.Name, os.O_RDWR, 0) // already exists
			if err != nil {
				return err
			}
			if err := f.Truncate(0); err != nil {
				return err
			}
			if _, err := f.Seek(0, 0); err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	if dryrun && foundFiles {
		return ErrDryRunFoundFiles
	}

	return nil
}

func ensureUnder(pwd, fn string) (err error) {
	if filepath.VolumeName(fn) != filepath.VolumeName(pwd) {
		return unusable(fn, errors.New("not on $PWD's volume"))
	}
	if fn[0] == '.' || filepath.IsAbs(fn) {
		var fnabs string
		if fnabs, err = filepath.Abs(fn); err != nil {
			return unusable(fn, err)
		}
		if !filepath.HasPrefix(fnabs, pwd) {
			return unusable(fn, errors.New("not under $PWD"))
		}
	}
	return
}

func ensureWritable(fn string) error {
	f, err := os.OpenFile(fn, os.O_RDWR, 0200)
	if err != nil {
		return unusable(fn, err.(*fs.PathError).Unwrap())
	}
	if err := f.Close(); err != nil {
		return unusable(fn, err)
	}
	return nil
}

func ensureRegular(fn string) error {
	if fi, err := os.Lstat(fn); err != nil {
		return unusable(fn, err.(*fs.PathError).Unwrap())
	} else if !fi.Mode().IsRegular() {
		return unusable(fn, errors.New("not a regular file"))
	}
	return nil
}
