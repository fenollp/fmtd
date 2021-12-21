# fmtd
Universal formatter command

```shell
# Install
DOCKER_BUILDKIT=1 docker build --output=/usr/local/bin/ https://github.com/fenollp/fmtd.git#main

# Usage
fmtd *.json src/**.h

# -n prints files that would be modified and exits
```

TODO: pass `./docker_cli*_test.go` tests and use github.com/docker/docker/client instead of `docker` command.
