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
# Change preset tools versions with:
export ARG_BEAUTYSH_VERSION=6.2.1
export ARG_BUILDIFIER_IMAGE=docker.io/whilp/buildifier@sha256:67da91fdddd40e9947153bc9157ab9103c141fcabcdbf646f040ba7a763bc531
export ARG_CLANGFORMAT_IMAGE=docker.io/unibeautify/clang-format@sha256:1b2d3997012ae221c600668802f1b761973d9006d330effa9555516432dea9c1
export ARG_GOFMT_IMAGE=docker.io/library/golang:1@sha256:4918412049183afe42f1ecaf8f5c2a88917c2eab153ce5ecf4bf2d55c1507b74
export ARG_SQLFORMAT_VERSION=0.4.2
export ARG_YAPF_VERSION=0.31.0
fmtd ...
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
