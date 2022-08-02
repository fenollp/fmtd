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
# syntax=docker.io/docker/dockerfile:1@sha256:443aab4ca21183e069e7d8b2dc68006594f40bddf1b15bbd83f5137bd93e80e2
`[1:] + `

ARG ALPINE=docker.io/library/alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300
ARG BUILDIFIER_IMAGE=docker.io/whilp/buildifier@sha256:67da91fdddd40e9947153bc9157ab9103c141fcabcdbf646f040ba7a763bc531
ARG CLANGFORMAT_IMAGE=docker.io/unibeautify/clang-format@sha256:1b2d3997012ae221c600668802f1b761973d9006d330effa9555516432dea9c1
ARG GOFMT_IMAGE=docker.io/library/golang:1@sha256:fb249eca1b9172732de4950b0fb0fb5c231b83c2c90952c56d822d8a9de4d64b
ARG SHFMT_IMAGE=docker.io/mvdan/shfmt@sha256:4564a08dbbc0c4541c182dd28de8ba5dc4a70045a926b4aca2cf76a8f246f28f
ARG TOMLFMT_IMAGE=docker.io/library/rust:1-slim@sha256:7f959043dd9aac68966ba0d35171073de3e76d917a73c7e237e145cdb86de333

FROM --platform=$BUILDPLATFORM $ALPINE AS alpine
FROM --platform=$BUILDPLATFORM $BUILDIFIER_IMAGE AS buildifier
FROM --platform=$BUILDPLATFORM $CLANGFORMAT_IMAGE AS clang-format
FROM --platform=$BUILDPLATFORM $GOFMT_IMAGE AS golang
FROM --platform=$BUILDPLATFORM $SHFMT_IMAGE AS shfmt
FROM --platform=$BUILDPLATFORM $TOMLFMT_IMAGE AS rust

# See https://github.com/Unibeautify/docker-beautifiers

# https://github.com/Unibeautify/docker-beautifiers/issues/63
FROM rust AS tomlfmt
RUN \
  --mount=type=cache,target=/usr/local/cargo/registry/index/ \
  --mount=type=cache,target=/usr/local/cargo/registry/cache/ \
  --mount=type=cache,target=/usr/local/cargo/git/db/ \
    set -ux \
 && rustup target add x86_64-unknown-linux-musl \
#&& cargo install --target x86_64-unknown-linux-musl --git https://github.com/segeljakt/toml-fmt \
# TODO: whence https://github.com/segeljakt/toml-fmt/pull/3
 && cargo install --target x86_64-unknown-linux-musl --git https://github.com/fenollp/toml-fmt --branch upupup \
 && [ '[a]' = "$(echo '[a]' | toml-fmt)" ]

FROM alpine AS tool
WORKDIR /app/b
WORKDIR /app/a
ARG YAPF_VERSION=0.32.0
ARG SQLFORMAT_VERSION=0.4.2
RUN \
  --mount=type=cache,target=/var/cache/apk ln -vs /var/cache/apk /etc/apk/cache && \
    set -ux \
 && apk add --no-cache \
    # For pip3 install
      py3-pip \
    # For clang-format
      clang \
    # JSON formatter
      jq \
 && touch /app/stdout \
 && pip3 install \
      yapf=="$YAPF_VERSION" \
      sqlparse=="$SQLFORMAT_VERSION"
COPY --from=buildifier /buildifier /usr/bin/buildifier
COPY --from=clang-format /usr/bin/clang-format /usr/bin/clang-format
COPY --from=golang /usr/local/go/bin/gofmt /usr/bin/gofmt
COPY --from=shfmt /bin/shfmt /usr/bin/shfmt
COPY --from=tomlfmt /usr/local/cargo/bin/toml-fmt /usr/bin/toml-fmt

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
      # Bazel / Skylark / Starlark
        build|*/build|*.build|*.bzl|*.sky|*.star|workspace|*/workspace) cp "$f" ../b/"$f" && buildifier -lint=fix ../b/"$f" ;; \
      # C / C++ / Protocol Buffers / Objective-C / Objective-C++
        *.c|*.cc|*.cpp|*.h|*.hh|*.proto|*.m|*.mm) clang-format -style=google -sort-includes "$f" >../b/"$f" ;; \
      # Erlang TODO: *.erl)
      # Go
        *.go) gofmt -s "$f" >../b/"$f" ;; \
      # JSON
        *.json) cat "$f" | jq -S --tab . >../b/"$f" ;; \
      # Python
        *.py) yapf --style=google "$f" >../b/"$f" ;; \
      # Shell
        *.sh) shfmt -s -p -kp "$f" >../b/"$f" ;; \
      # SQL
        *.sql) sqlformat --keywords=upper --reindent --reindent_aligned --use_space_around_operators --comma_first True "$f" >../b/"$f" ;; \
      # TOML
        *.toml) cat "$f" | toml-fmt >../b/"$f" ;; \
      # YAML TODO: *.yaml|*.yml)
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
