package containers

import (
	"errors"
	"net/http"
	"strings"

	portainer "github.com/portainer/portainer/api"
)

const (
	defaultQueryTimeout = 30
	minQueryTimeout     = 5
	maxQueryTimeout     = 120
)

type databaseConnectionPayload struct {
	Name         string                         `json:"Name"`
	Type         portainer.DatabaseConnectionType `json:"Type"`
	Host         string                         `json:"Host"`
	Port         int                            `json:"Port"`
	Database     string                         `json:"Database"`
	Username     string                         `json:"Username"`
	Password     *string                        `json:"Password"`
	QueryTimeout int                            `json:"QueryTimeout"`
}

func (payload *databaseConnectionPayload) Validate(r *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Host = strings.TrimSpace(payload.Host)
	payload.Database = strings.TrimSpace(payload.Database)
	payload.Username = strings.TrimSpace(payload.Username)

	if payload.Name == "" {
		return errors.New("name is required")
	}

	if payload.Host == "" {
		return errors.New("host is required")
	}

	switch payload.Type {
	case "mysql", "mariadb", "postgres", "redis":
	default:
		return errors.New("unsupported database type")
	}

	if payload.Port == 0 {
		payload.Port = defaultDatabasePort(payload.Type)
	}

	if payload.Port <= 0 || payload.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}

	if payload.QueryTimeout == 0 {
		payload.QueryTimeout = defaultQueryTimeout
	}

	if payload.QueryTimeout < minQueryTimeout || payload.QueryTimeout > maxQueryTimeout {
		return errors.New("query timeout must be between 5 and 120 seconds")
	}

	return nil
}

type databaseConnectionResponse struct {
	ID              portainer.DatabaseConnectionID   `json:"Id"`
	EnvironmentID   portainer.EndpointID             `json:"EnvironmentId"`
	ContainerID     string                           `json:"ContainerId"`
	CreatedByUserID portainer.UserID                 `json:"CreatedByUserId"`
	Name            string                           `json:"Name"`
	Type            portainer.DatabaseConnectionType `json:"Type"`
	Host            string                           `json:"Host"`
	Port            int                              `json:"Port"`
	Database        string                           `json:"Database,omitempty"`
	Username        string                           `json:"Username,omitempty"`
	HasPassword     bool                             `json:"HasPassword"`
	QueryTimeout    int                              `json:"QueryTimeout"`
	CreatedAt       int64                            `json:"CreatedAt"`
	UpdatedAt       int64                            `json:"UpdatedAt"`
}

func newDatabaseConnectionResponse(connection portainer.DatabaseConnection) databaseConnectionResponse {
	return databaseConnectionResponse{
		ID:              connection.ID,
		EnvironmentID:   connection.EnvironmentID,
		ContainerID:     connection.ContainerID,
		CreatedByUserID: connection.CreatedByUserID,
		Name:            connection.Name,
		Type:            connection.Type,
		Host:            connection.Host,
		Port:            connection.Port,
		Database:        connection.Database,
		Username:        connection.Username,
		HasPassword:     connection.Password != "",
		QueryTimeout:    connection.QueryTimeout,
		CreatedAt:       connection.CreatedAt,
		UpdatedAt:       connection.UpdatedAt,
	}
}

func newDatabaseConnectionResponses(connections []portainer.DatabaseConnection) []databaseConnectionResponse {
	responses := make([]databaseConnectionResponse, 0, len(connections))
	for _, connection := range connections {
		responses = append(responses, newDatabaseConnectionResponse(connection))
	}

	return responses
}

func defaultDatabasePort(connectionType portainer.DatabaseConnectionType) int {
	switch connectionType {
	case "mysql", "mariadb":
		return 3306
	case "postgres":
		return 5432
	case "redis":
		return 6379
	}

	return 0
}
