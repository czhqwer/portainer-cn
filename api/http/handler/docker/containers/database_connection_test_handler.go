package containers

import (
	"net/http"

	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type databaseConnectionTestResponse struct {
	Message string `json:"Message"`
}

func (handler *Handler) databaseConnectionTest(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	containerID, err := request.RetrieveRouteVariableValue(r, "containerId")
	if err != nil {
		return httperror.BadRequest("Invalid container identifier route variable", err)
	}

	var payload databaseConnectionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	client, endpoint, httpErr := handler.prepareDatabaseContainerAccess(r, containerID)
	if httpErr != nil {
		return httpErr
	}
	defer client.Close()

	connection := databaseConnectionFromPayload(endpoint.ID, containerID, payload)

	if _, err := executeDatabaseQuery(r.Context(), client, containerID, connection, databaseTestQuery(connection.Type), false); err != nil {
		return httperror.InternalServerError("Unable to test database connection", err)
	}

	return response.JSON(w, databaseConnectionTestResponse{Message: "Connection successful"})
}

func databaseConnectionFromPayload(endpointID portainer.EndpointID, containerID string, payload databaseConnectionPayload) portainer.DatabaseConnection {
	password := ""
	if payload.Password != nil {
		password = *payload.Password
	}

	return portainer.DatabaseConnection{
		EnvironmentID: endpointID,
		ContainerID:   containerID,
		Type:          payload.Type,
		Host:          payload.Host,
		Port:          payload.Port,
		Database:      payload.Database,
		Username:      payload.Username,
		Password:      password,
		QueryTimeout:  payload.QueryTimeout,
	}
}

func databaseTestQuery(connectionType portainer.DatabaseConnectionType) string {
	if connectionType == "redis" {
		return "PING"
	}

	return "SELECT 1"
}
