package endpoints

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type databaseSchemaResponse struct {
	Databases []databaseSchemaDatabase `json:"Databases"`
	Message   string                   `json:"Message,omitempty"`
}

type databaseSchemaDatabase struct {
	Name   string                `json:"Name"`
	Tables []databaseSchemaTable `json:"Tables"`
}

type databaseSchemaTable struct {
	Name string `json:"Name"`
	Type string `json:"Type,omitempty"`
}

func (handler *Handler) databaseConnectionSchema(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, endpointID)
	if httpErr != nil {
		return httpErr
	}

	result, err := executeDirectDatabaseSchema(r.Context(), *connection)
	if err != nil {
		return writeDatabaseError(w, "Unable to retrieve database schema", err)
	}

	return response.JSON(w, result)
}

func executeDirectDatabaseSchema(ctx context.Context, connection portainer.DatabaseConnection) (*databaseSchemaResponse, error) {
	switch connection.Type {
	case "mysql", "mariadb":
		return mysqlDatabaseSchema(ctx, connection)
	case "postgres":
		return postgresDatabaseSchema(ctx, connection)
	case "redis":
		return &databaseSchemaResponse{
			Databases: []databaseSchemaDatabase{},
			Message:   "Redis does not support database tree browsing",
		}, nil
	default:
		return nil, errors.New("unsupported database type")
	}
}

func mysqlDatabaseSchema(ctx context.Context, connection portainer.DatabaseConnection) (*databaseSchemaResponse, error) {
	if connection.Database != "" {
		tables, err := mysqlTables(ctx, connection, connection.Database)
		if err != nil {
			return nil, err
		}

		return &databaseSchemaResponse{
			Databases: []databaseSchemaDatabase{
				{
					Name:   connection.Database,
					Tables: tables,
				},
			},
		}, nil
	}

	result, err := executeDirectDatabaseQuery(ctx, connection, "SHOW DATABASES", false)
	if err != nil {
		return nil, err
	}

	databases := make([]databaseSchemaDatabase, 0, len(result.Rows))
	for _, row := range result.Rows {
		name := firstDatabaseValue(row)
		if name == "" {
			continue
		}

		tables, err := mysqlTables(ctx, connection, name)
		if err != nil {
			return nil, err
		}

		databases = append(databases, databaseSchemaDatabase{
			Name:   name,
			Tables: tables,
		})
	}

	return &databaseSchemaResponse{Databases: databases}, nil
}

func mysqlTables(ctx context.Context, connection portainer.DatabaseConnection, database string) ([]databaseSchemaTable, error) {
	query := fmt.Sprintf("SHOW FULL TABLES FROM `%s`", strings.ReplaceAll(database, "`", "``"))
	result, err := executeDirectDatabaseQuery(ctx, connection, query, false)
	if err != nil {
		return nil, err
	}

	tables := make([]databaseSchemaTable, 0, len(result.Rows))
	for _, row := range result.Rows {
		name := firstDatabaseValue(row)
		if name == "" {
			continue
		}
		tableType := ""
		for key, value := range row {
			if strings.Contains(strings.ToLower(key), "type") {
				tableType = value
			}
		}
		tables = append(tables, databaseSchemaTable{Name: name, Type: tableType})
	}

	return tables, nil
}

func postgresDatabaseSchema(ctx context.Context, connection portainer.DatabaseConnection) (*databaseSchemaResponse, error) {
	query := `
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_schema NOT LIKE 'pg_%'
  AND table_schema <> 'information_schema'
ORDER BY table_schema, table_name`
	result, err := executeDirectDatabaseQuery(ctx, connection, query, false)
	if err != nil {
		return nil, err
	}

	databaseIndexByName := map[string]int{}
	databases := []databaseSchemaDatabase{}
	for _, row := range result.Rows {
		schema := row["table_schema"]
		if schema == "" {
			continue
		}

		index, ok := databaseIndexByName[schema]
		if !ok {
			databases = append(databases, databaseSchemaDatabase{Name: schema})
			index = len(databases) - 1
			databaseIndexByName[schema] = index
		}

		databases[index].Tables = append(databases[index].Tables, databaseSchemaTable{
			Name: row["table_name"],
			Type: row["table_type"],
		})
	}

	return &databaseSchemaResponse{Databases: databases}, nil
}

func firstDatabaseValue(row map[string]string) string {
	for key, value := range row {
		key = strings.ToLower(key)
		if value != "" && strings.Contains(key, "database") {
			return value
		}
		if value != "" && strings.Contains(key, "table") && !strings.Contains(key, "type") {
			return value
		}
	}

	for key, value := range row {
		if value != "" && !strings.Contains(strings.ToLower(key), "type") {
			return value
		}
	}

	return ""
}
