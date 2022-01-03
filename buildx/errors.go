package buildx

import (
	"errors"
)

// ErrNoDocker is returned when no usable Docker client can be found
var ErrNoDocker = errors.New("No docker client found: curl -fsSL https://get.docker.com -o get-docker.sh && sudo sh get-docker.sh")

// ErrDockerBuildFailure is returned when docker build failed
var ErrDockerBuildFailure = errors.New("docker build failed with status 1")
