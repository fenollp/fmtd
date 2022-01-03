package fmtd

import (
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

	"github.com/fenollp/fmtd/buildx"
)

// ErrDryRunFoundFiles is returned when a run would have modified files if it weren't for dryrun
var ErrDryRunFoundFiles = errors.New("unformatted files found")

func unusable(fn string, err error) error {
	return fmt.Errorf("unusable file %q (%v)", fn, err)
}

func dockerfile(complain bool) []byte {
	var complaining string
	if complain {
		complaining = `echo "! $f" >>../stdout`
	}
	return []byte(`
# syntax=docker.io/docker/dockerfile:1@sha256:42399d4635eddd7a9b8a24be879d2f9a930d0ed040a61324cfdf59ef1357b3b2
`[1:] + `

ARG BUILDIFIER_IMAGE=docker.io/whilp/buildifier@sha256:67da91fdddd40e9947153bc9157ab9103c141fcabcdbf646f040ba7a763bc531
ARG CLANGFORMAT_IMAGE=docker.io/unibeautify/clang-format@sha256:1b2d3997012ae221c600668802f1b761973d9006d330effa9555516432dea9c1
ARG GOFMT_IMAGE=docker.io/library/golang:1@sha256:4918412049183afe42f1ecaf8f5c2a88917c2eab153ce5ecf4bf2d55c1507b74
ARG SHFMT_IMAGE=docker.io/mvdan/shfmt@sha256:f0d8d9f0c9dc15eb4e76b06035e7ffc59018d08e300e0af096be481a37a7d1dc

FROM --platform=$BUILDPLATFORM $BUILDIFIER_IMAGE AS buildifier
FROM --platform=$BUILDPLATFORM $CLANGFORMAT_IMAGE AS clang-format
FROM --platform=$BUILDPLATFORM $GOFMT_IMAGE AS golang
FROM --platform=$BUILDPLATFORM $SHFMT_IMAGE AS shfmt
FROM --platform=$BUILDPLATFORM docker.io/library/alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300 AS alpine

# See https://github.com/Unibeautify/docker-beautifiers

FROM alpine AS tool
WORKDIR /app/b
WORKDIR /app/a
ARG YAPF_VERSION=0.31.0
ARG SQLFORMAT_VERSION=0.4.2
RUN \
  --mount=type=cache,target=/var/cache/apk ln -vs /var/cache/apk /etc/apk/cache && \
    set -ux \
 && apk add --no-cache py3-pip clang emacs jq \
 && touch /app/stdout \
 && pip3 install \
      yapf=="$YAPF_VERSION" \
      sqlparse=="$SQLFORMAT_VERSION"
COPY --from=buildifier /buildifier /usr/bin/buildifier
COPY --from=clang-format /usr/bin/clang-format /usr/bin/clang-format
COPY --from=golang /usr/local/go/bin/gofmt /usr/bin/gofmt
COPY --from=shfmt /bin/shfmt /usr/bin/shfmt

FROM tool AS product
COPY a /app/a/
RUN \
    set -ux \
 && while read -r f; do \
      f=${f#./*} \
      && \
      mkdir -p ../b/"$(dirname "$f")" \
      && \
      case "$f" in \
      # C / C++ / Protocol Buffers / Objective-C / Objective-C++
        *.c|*.cc|*.cpp|*.h|*.hh|*.proto|*.m|*.mm) clang-format -style=google -sort-includes "$f" >../b/"$f" ;; \
      # Bazel / Skylark / Starlark
        BUILD|*.BUILD|*.bzl|*.sky|*.star|WORKSPACE) cp "$f" ../b/"$f" && buildifier -lint=fix ../b/"$f" ;; \
      # JSON
        *.json) cat "$f" | jq -S --tab . >../b/"$f" ;; \
      # Python
        *.py) yapf --style=google "$f" >../b/"$f" ;; \
      # Shell
        *.sh) shfmt -s -p -kp "$f" >../b/"$f" ;; \
      # SQL
        *.sql) sqlformat --keywords=upper --reindent --reindent_aligned --use_space_around_operators --comma_first True "$f" >../b/"$f" ;; \
      # Go
        *.go) gofmt -s "$f" >../b/"$f" ;; \
      # YAML TODO: *.yaml|*.yml)
      # Erlang TODO: *.erl)
        *) ` + complaining + ` ;; \
      esac \
      && \
      if [ -f ../b/"$f" ] && diff -q "$f" ../b/"$f" >/dev/null; then rm ../b/"$f"; fi \
      ; \
   done < <(find . -type f)

FROM scratch
COPY --from=product /app/b/ /
COPY --from=product /app/stdout /
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
		return buildx.ErrNoDocker
	}

	if len(filenames) == 0 {
		filenames = append(filenames, pwd)
	}
	fns := make([]string, 0, len(filenames))
	var moreFns []string
	for _, filename := range filenames {
		additional, err := ensureRegular(pwd, filename, dryrun)
		if err != nil {
			return err
		}
		if len(additional) != 0 {
			moreFns = append(moreFns, additional...)
		} else {
			fns = append(fns, filename)
			if err := ensureUnder(pwd, filename); err != nil {
				return err
			}
			if !dryrun {
				if err := ensureWritable(filename); err != nil {
					return err
				}
			}
		}
	}
	filenames = uniqueSorted(append(fns, moreFns...))

	foundFiles := false
	options := []buildx.Option{
		buildx.WithContext(ctx),
		buildx.WithStdout(stdout),
		buildx.WithStderr(stderr),
		buildx.WithExecutable(exe),
		buildx.WithDockerfile(dockerfile(len(moreFns) == 0)),
		buildx.WithOutputFileFunc(func(filename string, r io.Reader) error {
			fmt.Fprintf(stdout, "%s\n", filename)
			foundFiles = true
			if !dryrun {
				if err := buildx.OverwriteFileContents(filename, r); err != nil {
					return err
				}
			}
			return nil
		}),
	}

	for _, filename := range filenames {
		data, err := os.ReadFile(filename)
		if err != nil {
			return unusable(filename, err)
		}
		options = append(options, buildx.WithInputFile(filename, data))
	}

	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "ARG_") {
			options = append(options, buildx.WithBuildArg(strings.TrimPrefix(kv, "ARG_")))
		}
	}

	if err := buildx.New(options...); err != nil {
		return err
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

func ensureRegular(pwd, fn string, dryrun bool) ([]string, error) {
	if fi, err := os.Lstat(fn); err != nil {
		return nil, unusable(fn, err.(*fs.PathError).Unwrap())
	} else if fi.Mode().IsRegular() {
		return nil, nil
	} else if fi.IsDir() {
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
			if !dryrun {
				if err := ensureWritable(path); err != nil {
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
	return nil, unusable(fn, errors.New("not a regular file"))
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
