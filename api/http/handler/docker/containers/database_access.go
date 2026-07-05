package containers

import (
	"errors"
	"net/http"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/docker/consts"
	"github.com/portainer/portainer/api/http/middlewares"
	"github.com/portainer/portainer/api/http/security"
	"github.com/portainer/portainer/api/internal/authorization"
	"github.com/portainer/portainer/api/stacks/stackutils"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
)

func (handler *Handler) prepareDatabaseContainerAccess(r *http.Request, containerID string) (*dockerclient.Client, *portainer.Endpoint, *httperror.HandlerError) {
	endpoint, err := middlewares.FetchEndpoint(r)
	if err != nil {
		return nil, nil, httperror.NotFound("Unable to find an environment on request context", err)
	}

	securityContext, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return nil, nil, httperror.InternalServerError("Unable to retrieve restricted request context", err)
	}

	client, err := handler.dockerClientFactory.CreateClient(endpoint, r.Header.Get(portainer.PortainerAgentTargetHeader), nil)
	if err != nil {
		return nil, nil, httperror.InternalServerError("Unable to connect to the Docker daemon", err)
	}

	inspectedContainer, err := client.ContainerInspect(r.Context(), containerID)
	if err != nil {
		_ = client.Close()
		return nil, nil, httperror.NotFound("Unable to find the container", err)
	}

	if !inspectedContainer.State.Running {
		_ = client.Close()
		return nil, nil, httperror.Conflict("Container is not running", errors.New("container is not running"))
	}

	if securityContext.IsAdmin {
		return client, endpoint, nil
	}

	resourceControl, err := handler.findContainerResourceControl(endpoint.ID, containerID, inspectedContainer)
	if err != nil {
		_ = client.Close()
		return nil, nil, httperror.InternalServerError("Unable to retrieve container access control", err)
	}

	if resourceControl == nil {
		_ = client.Close()
		return nil, nil, httperror.Forbidden("Access denied to container", errors.New("missing resource control"))
	}

	teamIDs := authorization.TeamIDs(securityContext.UserMemberships)
	if !authorization.UserCanAccessResource(securityContext.UserID, teamIDs, resourceControl) {
		_ = client.Close()
		return nil, nil, httperror.Forbidden("Access denied to container", errors.New("user cannot access resource"))
	}

	return client, endpoint, nil
}

func (handler *Handler) findContainerResourceControl(endpointID portainer.EndpointID, containerID string, inspectedContainer container.InspectResponse) (*portainer.ResourceControl, error) {
	resourceControls, err := handler.dataStore.ResourceControl().ReadAll()
	if err != nil {
		if handler.dataStore.IsErrObjectNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	resourceControl := authorization.GetResourceControlByResourceIDAndType(containerID, portainer.ContainerResourceControl, resourceControls)
	if resourceControl != nil {
		return resourceControl, nil
	}

	labels := inspectedContainer.Config.Labels
	if serviceID := labels[consts.SwarmServiceIDLabel]; serviceID != "" {
		resourceControl = authorization.GetResourceControlByResourceIDAndType(serviceID, portainer.ServiceResourceControl, resourceControls)
		if resourceControl != nil {
			return resourceControl, nil
		}
	}

	if stackName := labels[consts.SwarmStackNameLabel]; stackName != "" {
		stackResourceID := stackutils.ResourceControlID(endpointID, stackName)
		resourceControl = authorization.GetResourceControlByResourceIDAndType(stackResourceID, portainer.StackResourceControl, resourceControls)
		if resourceControl != nil {
			return resourceControl, nil
		}
	}

	if stackName := labels[consts.ComposeStackNameLabel]; stackName != "" {
		stackResourceID := stackutils.ResourceControlID(endpointID, stackName)
		resourceControl = authorization.GetResourceControlByResourceIDAndType(stackResourceID, portainer.StackResourceControl, resourceControls)
		if resourceControl != nil {
			return resourceControl, nil
		}
	}

	return nil, nil
}

