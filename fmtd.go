package fmtd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/fenollp/fmtd/buildx"
)

// ErrDryRunFoundFiles is returned when a run would have modified files if it weren't for dryrun
var ErrDryRunFoundFiles = errors.New("unformatted files found")

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

	foundFiles := false

	options := []buildx.Option{
		buildx.WithContext(ctx),
		buildx.WithInputFiles(
			buildx.WithPWD(pwd),
			buildx.WithFilenames(filenames),
			buildx.WithUseCurrentDirWhenNoPathsGiven(),
			buildx.WithTraverseDirectories(true),
			buildx.WithEnsureUnderPWD(true),
			buildx.WithEnsureWritable(!dryrun),
			buildx.WithSelectionFailureBuilder(func(fn string, err error) error {
				return fmt.Errorf("unusable file %q (%v)", fn, err)
			}),
		),
		buildx.WithStdout(stdout),
		buildx.WithStderr(stderr),
		buildx.WithExecutable(exe),
		buildx.WithDockerfile(func(m map[interface{}]interface{}) []byte {
			foundFilenamesByTraversingDirs := m["foundFilenamesByTraversingDirs"].(bool)
			return dockerfile(!foundFilenamesByTraversingDirs)
		}),
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
      case "$(echo "$f" | tr '[:upper:]' '[:lower:]')" in \
      # C / C++ / Protocol Buffers / Objective-C / Objective-C++
        *.c|*.cc|*.cpp|*.h|*.hh|*.proto|*.m|*.mm) clang-format -style=google -sort-includes "$f" >../b/"$f" ;; \
      # Bazel / Skylark / Starlark
        build|*/build|*.build|*.bzl|*.sky|*.star|workspace|*/workspace) cp "$f" ../b/"$f" && buildifier -lint=fix ../b/"$f" ;; \
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
