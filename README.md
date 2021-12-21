# fmt
Universal formatter command

## Usage

```shell
fmtd *.json src/**.h

# -n prints files that would be modified and exits
```

## Install

```shell
DOCKER_BUILDKIT=1 docker build --output=/usr/local/bin/ https://github.com/fenollp/fmtd.git#main
```

## To do

TODO: pass `./docker_cli*_test.go` tests and use github.com/docker/docker/client instead of `docker` command.
