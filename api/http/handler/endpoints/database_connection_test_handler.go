package endpoints

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
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	var payload databaseConnectionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	connection := databaseConnectionFromPayload(endpointID, payload)
	if payload.ID != 0 && connection.Password == "" {
		// 编辑连接测试时，空密码沿用已保存密码，确保“留空保留当前密码”的提示与测试行为一致。
		storedConnection, httpErr := handler.databaseConnectionByID(r, endpointID, payload.ID)
		if httpErr != nil {
			return httpErr
		}

		connection.Password = storedConnection.Password
	}

	if _, err := executeDirectDatabaseQuery(r.Context(), connection, databaseTestQuery(connection.Type), false); err != nil {
		return writeDatabaseError(w, "Unable to test database connection", err)
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
