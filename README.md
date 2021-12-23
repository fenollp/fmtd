# fmtd
Universal formatter command

```shell
# Install
DOCKER_BUILDKIT=1 docker build -o=/usr/local/bin/ https://github.com/fenollp/fmtd.git#main

# Usage
fmtd *.json src/**.h

#  -2	show Docker progress
#  -n	dry run: no files will be written
```

```shell
# An alias to reformat Git tracked and cached files:
gfmt() {
    while read -r f; do
        fmtd "$f"
    done < <(git status --short --porcelain -- . | \grep '^. ' | \grep -Eo '[^ ]+$')
}
```


TODO: pass `./docker_cli*_test.go` tests and use github.com/docker/docker/client instead of `docker` command.
TODO: turn the core into a lib that'd allow passing input files to a Docker-ized command and getting output files (e.g. for `protoc`)
