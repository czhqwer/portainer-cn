package endpoints

import (
	"errors"
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
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	var payload databaseConnectionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	if payload.ContainerID != "" {
		return httperror.BadRequest("Container database connections must be tested through the Docker endpoint", errors.New("container database connection"))
	}

	connection := databaseConnectionFromPayload(endpointID, payload)

	if _, err := executeDirectDatabaseQuery(r.Context(), connection, databaseTestQuery(connection.Type), false); err != nil {
		return httperror.InternalServerError("Unable to test database connection", err)
	}

	return response.JSON(w, databaseConnectionTestResponse{Message: "Connection successful"})
}

func databaseConnectionFromPayload(endpointID portainer.EndpointID, payload databaseConnectionPayload) portainer.DatabaseConnection {
	password := ""
	if payload.Password != nil {
		password = *payload.Password
	}

	return portainer.DatabaseConnection{
		EnvironmentID: endpointID,
		ContainerID:   payload.ContainerID,
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
