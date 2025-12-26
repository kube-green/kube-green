package utils

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// DockerClient is a wrapper interface for the Docker library to support unit testing.
type DockerClient interface {
	NegotiateAPIVersion(context.Context)
	VolumeCreate(context.Context, volume.CreateOptions) (volume.Volume, error)
	ImageSave(context.Context, []string, ...client.ImageSaveOption) (io.ReadCloser, error)
}
