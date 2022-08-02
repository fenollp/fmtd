# fmtd
Universal formatter command

```shell
# Install
DOCKER_BUILDKIT=1 docker build -o=/usr/local/bin/ --platform=local https://github.com/fenollp/fmtd.git#main

# Usage
fmtd *.json src/**.h

#  -2	show Docker progress
#  -n	dry run: no files will be written
```

```shell
# Change preset tools versions with:
export ARG_BUILDIFIER_IMAGE=docker.io/whilp/buildifier@sha256:67da91fdddd40e9947153bc9157ab9103c141fcabcdbf646f040ba7a763bc531
export ARG_CLANGFORMAT_IMAGE=docker.io/unibeautify/clang-format@sha256:1b2d3997012ae221c600668802f1b761973d9006d330effa9555516432dea9c1
export ARG_GOFMT_IMAGE=docker.io/library/golang:1@sha256:fb249eca1b9172732de4950b0fb0fb5c231b83c2c90952c56d822d8a9de4d64b
export ARG_SHFMT_IMAGE=docker.io/mvdan/shfmt@sha256:4564a08dbbc0c4541c182dd28de8ba5dc4a70045a926b4aca2cf76a8f246f28f
export ARG_SQLFORMAT_VERSION=0.4.2
export ARG_TOMLFMT_IMAGE=docker.io/library/rust:1-slim@sha256:7f959043dd9aac68966ba0d35171073de3e76d917a73c7e237e145cdb86de333
export ARG_YAPF_VERSION=0.32.0
fmtd .
```

```shell
# An alias to reformat Git tracked and cached files:
gfmt() {
    local fs='';
    while read -r f; do
        fs="$fs $f";
    done < <(git status --short --porcelain -- . | \grep '^. ' | \grep -Eo '[^ ]+$');
    if [[ -n "$fs" ]]; then
        fmtd $fs;
    fi
}
```

***

## TODO
* pass `./docker_cli*_test.go` tests and use github.com/docker/docker/client instead of `docker` command.
* more from https://github.com/Unibeautify/docker-beautifiers
