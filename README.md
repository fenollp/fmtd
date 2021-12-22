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

TODO: pass `./docker_cli*_test.go` tests and use github.com/docker/docker/client instead of `docker` command.
