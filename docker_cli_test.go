package fmttr_test

import (
	"context"
	"os"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

func TestDockerCLIWithDOCKERHOST(t *testing.T) {
	ctx := context.Background()

	err := os.Setenv("DOCKER_HOST", "ssh://flatbit")
	require.NoError(t, err)

	cli, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)

	// containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	// require.NoError(t, err)
	// for _, container := range containers {
	// 	t.Logf("%s %s", container.ID[:10], container.Image)
	// }

	// images, err := cli.ImageList(ctx, types.ImageListOptions{All: true})
	// require.NoError(t, err)
	// for _, image := range images {
	// 	t.Logf("%v", image)
	// }

	r, err := cli.ImagePull(ctx, "alpine", types.ImagePullOptions{})
	require.Error(t, err, `error during connect: Post "http://flatbit/v1.41/images/create?fromImage=alpine&tag=latest": dial tcp: lookup flatbit on 127.0.0.53:53: server misbehaving`)
	require.Nil(t, r)
}
