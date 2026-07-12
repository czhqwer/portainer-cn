package platform

import (
	"context"
	"errors"
	"io"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/errdefs"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	dockerclient "github.com/portainer/portainer/api/docker/client"
	"github.com/portainer/portainer/api/logs"
)

type DockerJavaImageBuilder struct {
	dataStore dataservices.DataStore
	factory   dockerArchiveClientFactory
}

func NewDockerJavaImageBuilder(dataStore dataservices.DataStore, factory *dockerclient.ClientFactory) *DockerJavaImageBuilder {
	return &DockerJavaImageBuilder{dataStore: dataStore, factory: factory}
}
func (b *DockerJavaImageBuilder) Build(ctx context.Context, request JavaImageBuildRequest) (JavaImageBuildResult, error) {
	if b == nil || b.dataStore == nil || b.factory == nil || request.Context == nil {
		return JavaImageBuildResult{}, errors.New("java image builder unavailable")
	}
	endpoint, err := b.dataStore.Endpoint().Endpoint(portainer.EndpointID(request.EndpointID))
	if err != nil {
		return JavaImageBuildResult{}, err
	}
	cli, err := b.factory.CreateClient(endpoint, "", nil)
	if err != nil {
		return JavaImageBuildResult{}, err
	}
	defer logs.CloseAndLogErr(cli)
	response, err := cli.ImageBuild(ctx, request.Context, build.ImageBuildOptions{Dockerfile: "Dockerfile", Tags: []string{request.CandidateRef}, Remove: true, ForceRemove: true})
	if err != nil {
		return JavaImageBuildResult{}, err
	}
	defer logs.CloseAndLogErr(response.Body)
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return JavaImageBuildResult{}, err
	}
	inspect, _, err := cli.ImageInspectWithRaw(ctx, request.CandidateRef)
	if err != nil {
		_ = b.Cleanup(context.Background(), request.EndpointID, request.CandidateRef)
		return JavaImageBuildResult{}, err
	}
	return JavaImageBuildResult{CandidateRef: request.CandidateRef, ImageID: inspect.ID}, nil
}
func (b *DockerJavaImageBuilder) Cleanup(ctx context.Context, endpointID int, candidate string) error {
	if b == nil || b.dataStore == nil || b.factory == nil || candidate == "" {
		return nil
	}
	endpoint, err := b.dataStore.Endpoint().Endpoint(portainer.EndpointID(endpointID))
	if err != nil {
		return err
	}
	cli, err := b.factory.CreateClient(endpoint, "", nil)
	if err != nil {
		return err
	}
	defer logs.CloseAndLogErr(cli)
	_, err = cli.ImageRemove(ctx, candidate, image.RemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}
