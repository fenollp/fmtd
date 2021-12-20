package fmttr

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

// Fmt formats (any) files below the current directory
func Fmt(
	ctx context.Context,
	pwd string,
	dryrun bool,
	stderr io.Writer,
	filenames []string,
) error {
	exe, err := exec.LookPath("docker")
	if err != nil {
		log.Fatal("No docker client found: curl -fsSL https://get.docker.com -o get-docker.sh && sudo sh get-docker.sh")
	}

	for _, filename := range filenames {
		fmt.Println(filename)
		if _, err := os.Stat(filename); err != nil {
			log.Println(err)
			return err
		}
		// TODO: check if regular file / symlink
		// TODO: check if writeable (drop when dryrun)
		// TODO: check if under PWD
	}

	var stdin bytes.Buffer
	tw := tar.NewWriter(&stdin)
	var files = []struct {
		Name, Body string
	}{
		{"readme.txt", "This archive contains some text files."},
		{"gopher.txt", "Gopher names:\nGeorge\nGeoffrey\nGonzo"},
		{"todo.txt", "Get animal handling license."},
		{"Dockerfile", `
# syntax=docker.io/docker/dockerfile:1@sha256:42399d4635eddd7a9b8a24be879d2f9a930d0ed040a61324cfdf59ef1357b3b2

FROM --platform=$BUILDPLATFORM docker.io/library/alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300 AS alpine

FROM alpine AS tool
WORKDIR /app/b
WORKDIR /app

FROM tool AS product
COPY . /app/a/
RUN \
    set -ux \
 && cat a/gopher.txt >b/gopher.txt && echo blaaa >>b/gopher.txt \
 && fn=gopher.txt; if diff -q a/"$fn" b/"$fn" >/dev/null; then rm b/"$fn"; fi \
 && echo hello >b/README

FROM scratch
COPY --from=product /app/b/* /
`[1:]},

		// fmt() {
		//     #TODO: loop inside scripts
		//     #TODO: use my erlfmt
		//     until [[ "$1" = '' ]]; do
		//         local file="$1"
		//         case "$file" in
		//             *.c|*.cc|*.cpp|*.h|*.hh|*.proto|*.m|*.mm)
		//                 clang-format -i -style=google -sort-includes "$file" ;;
		//             *.json)  cat "$file" | jq -S --tab . >"$file"~ && mv "$file"~ "$file" ;;
		//             *.yaml|*.yml)  $HOME/.bin/format-yaml.sh "$file" ;;
		//             *.py) $HOME/.local/bin/yapf --in-place --style=google "$file" ;;
		//             *.erl)  emacs --script $HOME/.bin/erlfmt.el "$file" ;;
		//             BUILD|*/BUILD|*.BUILD|*.bzl|*.sky|*.star|WORKSPACE|*/WORKSPACE)
		//                 buildifier -lint=fix "$file" ;;
		//             # *.sh) shfmt -i 2 -ci -w -kp -s "$file" ;; # go get mvdan.cc/sh/cmd/shfmt
		//             *)  return 1 ;;
		//         esac
		//         [[ $? -ne 0 ]] && return 2
		//         shift
		//     done
		// }
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Mode: 0600, // TODO: clone file attributes but: may not translate + writing should not need them as no new files get created
			Size: int64(len(file.Body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			log.Println(err)
			return err
		}
		if _, err := tw.Write([]byte(file.Body)); err != nil {
			log.Println(err)
			return err
		}
	}
	if err := tw.Close(); err != nil {
		log.Println(err)
		return err
	}

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, exe, "build", "--output=-", "-")
	cmd.Env = append(os.Environ(),
		"DOCKER_BUILDKIT=1",
	)
	cmd.Stdin = &stdin
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		log.Println(err)
		return err
	}

	tr := tar.NewReader(&stdout)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			log.Println(err)
			return err
		}
		fmt.Println("Contents of", hdr.Name)
		if !dryrun {
			if _, err := io.Copy(os.Stdout, tr); err != nil {
				log.Println(err)
				return err
			}
			fmt.Println()
		}
	}

	return nil
}
