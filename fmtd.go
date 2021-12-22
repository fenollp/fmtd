package fmtd

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// ErrNoDocker is returned when no usable Docker client can be found
var ErrNoDocker = errors.New("No docker client found: curl -fsSL https://get.docker.com -o get-docker.sh && sudo sh get-docker.sh")

// ErrDryRunFoundFiles is returned when a run would have modified files if it weren't for dryrun
var ErrDryRunFoundFiles = errors.New("unformatted files found")

func unusable(fn string) error { return fmt.Errorf("unusable file: %q", fn) }

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
		log.Println(err)
		return ErrNoDocker
	}

	if len(filenames) == 0 {
		return nil
	}
	fns := make(map[string]struct{}, len(filenames))
	for _, filename := range filenames {
		if err := ensureRegular(filename); err != nil {
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
		fns[filename] = struct{}{}
	}
	filenames = make([]string, 0, len(fns))
	for fn := range fns {
		filenames = append(filenames, fn)
	}
	sort.Strings(filenames)

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
	foundFiles := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			log.Println(err)
			return err
		}
		fmt.Println(hdr.Name)
		if !dryrun {
			if _, err := io.Copy(os.Stdout, tr); err != nil {
				log.Println(err)
				return err
			}
			fmt.Println()
		}
	}
	if dryrun && foundFiles {
		err := ErrDryRunFoundFiles
		log.Println(err)
		return err
	}

	return nil
}

func ensureUnder(pwd, fn string) (err error) {
	if filepath.VolumeName(fn) != filepath.VolumeName(pwd) {
		err = unusable(fn)
		log.Println(err)
		return
	}
	if fn[0] == '.' || filepath.IsAbs(fn) {
		var fnabs string
		if fnabs, err = filepath.Abs(fn); err != nil {
			log.Println(err)
			return unusable(fn)
		}
		if !filepath.HasPrefix(fnabs, pwd) {
			err = unusable(fn)
			log.Println(err)
			return
		}
	}
	return
}

func ensureWritable(fn string) error {
	fd, err := os.OpenFile(fn, os.O_RDWR, 0200)
	if err != nil {
		log.Println(err)
		return unusable(fn)
	}
	if err := fd.Close(); err != nil {
		log.Println(err)
		return unusable(fn)
	}
	return nil
}

func ensureRegular(fn string) error {
	if fi, err := os.Lstat(fn); err != nil {
		log.Println(err)
		return unusable(fn)
	} else if !fi.Mode().IsRegular() {
		return unusable(fn)
	}
	return nil
}
