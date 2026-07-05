package containers

import (
	"errors"
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/http/middlewares"
	"github.com/portainer/portainer/api/http/security"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

func (handler *Handler) databaseConnectionList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	containerID, err := request.RetrieveRouteVariableValue(r, "containerId")
	if err != nil {
		return httperror.BadRequest("Invalid container identifier route variable", err)
	}

	client, endpoint, httpErr := handler.prepareDatabaseContainerAccess(r, containerID)
	if httpErr != nil {
		return httpErr
	}
	defer client.Close()

	securityContext, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return httperror.InternalServerError("Unable to retrieve restricted request context", err)
	}

	connections, err := handler.dataStore.DatabaseConnection().ConnectionsByContainer(securityContext.UserID, endpoint.ID, containerID)
	if err != nil && !handler.dataStore.IsErrObjectNotFound(err) {
		return httperror.InternalServerError("Unable to retrieve database connections", err)
	}

	return response.JSON(w, newDatabaseConnectionResponses(connections))
}

func (handler *Handler) databaseConnectionCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	containerID, err := request.RetrieveRouteVariableValue(r, "containerId")
	if err != nil {
		return httperror.BadRequest("Invalid container identifier route variable", err)
	}

	var payload databaseConnectionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	if payload.Port == 0 {
		payload.Port = defaultDatabasePort(payload.Type)
	}

	password := ""
	if payload.Password != nil {
		password = *payload.Password
	}

	if password != "" && !handler.dataStore.Connection().IsEncryptedStore() {
		return httperror.Forbidden("Database encryption is required to save passwords", errors.New("datastore encryption is not enabled"))
	}

	client, endpoint, httpErr := handler.prepareDatabaseContainerAccess(r, containerID)
	if httpErr != nil {
		return httpErr
	}
	defer client.Close()

	securityContext, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return httperror.InternalServerError("Unable to retrieve restricted request context", err)
	}

	now := time.Now().Unix()
	connection := &portainer.DatabaseConnection{
		EnvironmentID:   endpoint.ID,
		ContainerID:     containerID,
		CreatedByUserID: securityContext.UserID,
		Name:            payload.Name,
		Type:            payload.Type,
		Host:            payload.Host,
		Port:            payload.Port,
		Database:        payload.Database,
		Username:        payload.Username,
		Password:        password,
		QueryTimeout:    payload.QueryTimeout,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := handler.dataStore.DatabaseConnection().Create(connection); err != nil {
		return httperror.InternalServerError("Unable to create database connection", err)
	}

	return response.JSONWithStatus(w, newDatabaseConnectionResponse(*connection), http.StatusCreated)
}

func (handler *Handler) databaseConnectionUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	containerID, err := request.RetrieveRouteVariableValue(r, "containerId")
	if err != nil {
		return httperror.BadRequest("Invalid container identifier route variable", err)
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, containerID)
	if httpErr != nil {
		return httpErr
	}

	var payload databaseConnectionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	if payload.Port == 0 {
		payload.Port = defaultDatabasePort(payload.Type)
	}

	if payload.Password != nil && *payload.Password != "" && !handler.dataStore.Connection().IsEncryptedStore() {
		return httperror.Forbidden("Database encryption is required to save passwords", errors.New("datastore encryption is not enabled"))
	}

	client, _, httpErr := handler.prepareDatabaseContainerAccess(r, containerID)
	if httpErr != nil {
		return httpErr
	}
	defer client.Close()

	connection.Name = payload.Name
	connection.Type = payload.Type
	connection.Host = payload.Host
	connection.Port = payload.Port
	connection.Database = payload.Database
	connection.Username = payload.Username
	connection.QueryTimeout = payload.QueryTimeout
	connection.UpdatedAt = time.Now().Unix()
	if payload.Password != nil {
		connection.Password = *payload.Password
	}

	if err := handler.dataStore.DatabaseConnection().Update(connection.ID, connection); err != nil {
		return httperror.InternalServerError("Unable to update database connection", err)
	}

	return response.JSON(w, newDatabaseConnectionResponse(*connection))
}

func (handler *Handler) databaseConnectionDelete(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	containerID, err := request.RetrieveRouteVariableValue(r, "containerId")
	if err != nil {
		return httperror.BadRequest("Invalid container identifier route variable", err)
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, containerID)
	if httpErr != nil {
		return httpErr
	}

	client, _, httpErr := handler.prepareDatabaseContainerAccess(r, containerID)
	if httpErr != nil {
		return httpErr
	}
	defer client.Close()

	if err := handler.dataStore.DatabaseConnection().Delete(connection.ID); err != nil {
		return httperror.InternalServerError("Unable to delete database connection", err)
	}

	return response.Empty(w)
}

func (handler *Handler) databaseConnectionFromRequest(r *http.Request, containerID string) (*portainer.DatabaseConnection, *httperror.HandlerError) {
	connectionID, err := request.RetrieveNumericRouteVariableValue(r, "connectionId")
	if err != nil {
		return nil, httperror.BadRequest("Invalid database connection identifier route variable", err)
	}

	securityContext, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return nil, httperror.InternalServerError("Unable to retrieve restricted request context", err)
	}

	endpoint, err := middlewares.FetchEndpoint(r)
	if err != nil {
		return nil, httperror.NotFound("Unable to find an environment on request context", err)
	}

	connection, err := handler.dataStore.DatabaseConnection().Read(portainer.DatabaseConnectionID(connectionID))
	if err != nil {
		if handler.dataStore.IsErrObjectNotFound(err) {
			return nil, httperror.NotFound("Unable to find database connection", err)
		}

		return nil, httperror.InternalServerError("Unable to retrieve database connection", err)
	}

	if connection.EnvironmentID != endpoint.ID || connection.ContainerID != containerID || connection.CreatedByUserID != securityContext.UserID {
		return nil, httperror.NotFound("Unable to find database connection", errors.New("database connection is not owned by user, environment, or container"))
	}

	return connection, nil
}
