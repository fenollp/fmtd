package fmtd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

func TestDockerCLIWithDOCKERHOSTAgain(t *testing.T) {
	ctx := context.Background()
	if os.Getenv("CI") == "true" {
		t.Skipf("run this in your shell...")
	}

	err := os.Setenv("DOCKER_HOST", "ssh://flatbit")
	require.NoError(t, err)

	cli, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	wTar := tar.NewWriter(buf)
	defer wTar.Close()

	readDockerFile := `
# syntax=docker.io/docker/dockerfile:1@sha256:42399d4635eddd7a9b8a24be879d2f9a930d0ed040a61324cfdf59ef1357b3b2

#FROM --platform=$BUILDPLATFORM docker.io/library/alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300 AS alpine
FROM docker.io/library/alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300 AS alpine

FROM alpine AS hi
RUN set -x && echo hello >/README

FROM scratch
COPY --from=hi /README /
`[1:]

	err = wTar.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Size: int64(len(readDockerFile)),
	})
	require.NoError(t, err)
	_, err = wTar.Write([]byte(readDockerFile))
	require.NoError(t, err)

	rTar := bytes.NewReader(buf.Bytes())

	buildOptions := types.ImageBuildOptions{
		Context:    rTar,
		Dockerfile: "Dockerfile",
		Remove:     true,
		Outputs: []types.ImageBuildOutput{
			// {
			// 	Type: "tar",
			// 	Attrs: map[string]string{
			// 		"dest": "-",
			// 	},
			// },
			{
				Type: "local",
				Attrs: map[string]string{
					"dest": "res",
				},
			},
		},
	}

	imageBuildResponse, err := cli.ImageBuild(ctx, rTar, buildOptions)
	require.Error(t, err, `error during connect: Post "http://flatbit/v1.41/build?buildargs=null&cachefrom=null&cgroupparent=&cpuperiod=0&cpuquota=0&cpusetcpus=&cpusetmems=&cpushares=0&dockerfile=Dockerfile&labels=null&memory=0&memswap=0&networkmode=&outputs=%5B%7B%22Type%22%3A%22local%22%2C%22Attrs%22%3A%7B%22dest%22%3A%22res%22%7D%7D%5D&rm=1&shmsize=0&target=&ulimits=null&version=": dial tcp: lookup flatbit on 127.0.0.53:53: server misbehaving`)
	require.Empty(t, imageBuildResponse)
}
