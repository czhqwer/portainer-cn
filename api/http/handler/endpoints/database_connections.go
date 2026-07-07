package endpoints

import (
	"errors"
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/http/security"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

func (handler *Handler) databaseConnectionList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	securityContext, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return httperror.InternalServerError("Unable to retrieve restricted request context", err)
	}

	connections, err := handler.DataStore.DatabaseConnection().ConnectionsByEnvironment(securityContext.UserID, endpointID)
	if err != nil && !handler.DataStore.IsErrObjectNotFound(err) {
		return httperror.InternalServerError("Unable to retrieve database connections", err)
	}

	return response.JSON(w, newDatabaseConnectionResponses(connections))
}

func (handler *Handler) databaseConnectionCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	var payload databaseConnectionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	password := ""
	if payload.Password != nil {
		password = *payload.Password
	}

	securityContext, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return httperror.InternalServerError("Unable to retrieve restricted request context", err)
	}

	now := time.Now().Unix()

	connection := &portainer.DatabaseConnection{
		EnvironmentID:   endpointID,
		ContainerID:     payload.ContainerID,
		Scope:           databaseConnectionScopeEnvironment,
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

	if err := handler.DataStore.DatabaseConnection().Create(connection); err != nil {
		return httperror.InternalServerError("Unable to create database connection", err)
	}

	return response.JSONWithStatus(w, newDatabaseConnectionResponse(*connection), http.StatusCreated)
}

func (handler *Handler) databaseConnectionUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, endpointID)
	if httpErr != nil {
		return httpErr
	}

	var payload databaseConnectionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	connection.Name = payload.Name
	connection.Type = payload.Type
	connection.Host = payload.Host
	connection.Port = payload.Port
	connection.Database = payload.Database
	connection.Username = payload.Username
	connection.QueryTimeout = payload.QueryTimeout
	connection.ContainerID = payload.ContainerID
	connection.Scope = databaseConnectionScopeEnvironment
	connection.UpdatedAt = time.Now().Unix()
	// 编辑连接时空密码表示保留旧密码，避免前端占位输入框把已保存密码误清空。
	if payload.Password != nil && *payload.Password != "" {
		connection.Password = *payload.Password
	}

	if err := handler.DataStore.DatabaseConnection().Update(connection.ID, connection); err != nil {
		return httperror.InternalServerError("Unable to update database connection", err)
	}

	return response.JSON(w, newDatabaseConnectionResponse(*connection))
}

func (handler *Handler) databaseConnectionDelete(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, endpointID)
	if httpErr != nil {
		return httpErr
	}

	if err := handler.DataStore.DatabaseConnection().Delete(connection.ID); err != nil {
		return httperror.InternalServerError("Unable to delete database connection", err)
	}

	return response.Empty(w)
}

func (handler *Handler) databaseEndpointIDFromRequest(r *http.Request) (portainer.EndpointID, *httperror.HandlerError) {
	endpointID, err := request.RetrieveNumericRouteVariableValue(r, "id")
	if err != nil {
		return 0, httperror.BadRequest("Invalid environment identifier route variable", err)
	}

	if _, err := handler.DataStore.Endpoint().Endpoint(portainer.EndpointID(endpointID)); err != nil {
		if handler.DataStore.IsErrObjectNotFound(err) {
			return 0, httperror.NotFound("Unable to find an environment with the specified identifier inside the database", err)
		}

		return 0, httperror.InternalServerError("Unable to retrieve environment from the database", err)
	}

	return portainer.EndpointID(endpointID), nil
}

func (handler *Handler) databaseConnectionFromRequest(r *http.Request, endpointID portainer.EndpointID) (*portainer.DatabaseConnection, *httperror.HandlerError) {
	connectionID, err := request.RetrieveNumericRouteVariableValue(r, "connectionId")
	if err != nil {
		return nil, httperror.BadRequest("Invalid database connection identifier route variable", err)
	}

	return handler.databaseConnectionByID(r, endpointID, portainer.DatabaseConnectionID(connectionID))
}

func (handler *Handler) databaseConnectionByID(r *http.Request, endpointID portainer.EndpointID, connectionID portainer.DatabaseConnectionID) (*portainer.DatabaseConnection, *httperror.HandlerError) {
	// 通过连接 ID 读取时统一校验环境和用户归属，避免测试连接等非 REST 路由绕过隔离。
	securityContext, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return nil, httperror.InternalServerError("Unable to retrieve restricted request context", err)
	}

	connection, err := handler.DataStore.DatabaseConnection().Read(connectionID)
	if err != nil {
		if handler.DataStore.IsErrObjectNotFound(err) {
			return nil, httperror.NotFound("Unable to find database connection", err)
		}

		return nil, httperror.InternalServerError("Unable to retrieve database connection", err)
	}

	if connection.EnvironmentID != endpointID ||
		connection.CreatedByUserID != securityContext.UserID {
		return nil, httperror.NotFound("Unable to find database connection", errors.New("database connection is not owned by user or environment"))
	}

	return connection, nil
}
