package endpoints

import (
	"context"
	"errors"
	"net/http"
	"strings"

	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type databaseTableDetailsResponse struct {
	Database string                `json:"Database"`
	Table    string                `json:"Table"`
	Comment  string                `json:"Comment,omitempty"`
	Columns  []databaseTableColumn `json:"Columns"`
	Indexes  []databaseTableIndex  `json:"Indexes"`
}

type databaseTableColumn struct {
	Name       string `json:"Name"`
	Type       string `json:"Type"`
	Nullable   bool   `json:"Nullable"`
	PrimaryKey bool   `json:"PrimaryKey"`
	Default    string `json:"Default,omitempty"`
	Extra      string `json:"Extra,omitempty"`
	Comment    string `json:"Comment,omitempty"`
}

type databaseTableIndex struct {
	Name    string   `json:"Name"`
	Columns []string `json:"Columns"`
	Unique  bool     `json:"Unique"`
	Primary bool     `json:"Primary"`
}

func (handler *Handler) databaseConnectionTableDetails(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, endpointID)
	if httpErr != nil {
		return httpErr
	}

	database := strings.TrimSpace(r.URL.Query().Get("database"))
	table := strings.TrimSpace(r.URL.Query().Get("table"))
	if database == "" {
		database = connection.Database
	}
	if table == "" {
		return httperror.BadRequest("Invalid database table request", errors.New("table is required"))
	}

	result, err := executeDirectTableDetails(r.Context(), *connection, database, table)
	if err != nil {
		return writeDatabaseError(w, "Unable to retrieve table details", err)
	}

	return response.JSON(w, result)
}

func executeDirectTableDetails(ctx context.Context, connection portainer.DatabaseConnection, database string, table string) (*databaseTableDetailsResponse, error) {
	switch connection.Type {
	case "mysql", "mariadb":
		return mysqlTableDetails(ctx, connection, database, table)
	case "postgres":
		return postgresTableDetails(ctx, connection, database, table)
	default:
		return nil, errors.New("table details are only supported for SQL databases")
	}
}

func mysqlTableDetails(ctx context.Context, connection portainer.DatabaseConnection, database string, table string) (*databaseTableDetailsResponse, error) {
	if database == "" {
		return nil, errors.New("database is required")
	}

	columnsQuery := `
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_DEFAULT, COLUMN_COMMENT, EXTRA
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ` + sqlStringLiteral(database) + `
  AND TABLE_NAME = ` + sqlStringLiteral(table) + `
ORDER BY ORDINAL_POSITION`
	columnsResult, err := executeDirectDatabaseQuery(ctx, connection, columnsQuery, false)
	if err != nil {
		return nil, err
	}

	indexesQuery := `
SELECT INDEX_NAME, COLUMN_NAME, NON_UNIQUE, SEQ_IN_INDEX
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = ` + sqlStringLiteral(database) + `
  AND TABLE_NAME = ` + sqlStringLiteral(table) + `
ORDER BY INDEX_NAME, SEQ_IN_INDEX`
	indexesResult, err := executeDirectDatabaseQuery(ctx, connection, indexesQuery, false)
	if err != nil {
		return nil, err
	}

	commentQuery := `
SELECT TABLE_COMMENT
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ` + sqlStringLiteral(database) + `
  AND TABLE_NAME = ` + sqlStringLiteral(table)
	commentResult, err := executeDirectDatabaseQuery(ctx, connection, commentQuery, false)
	if err != nil {
		return nil, err
	}

	details := &databaseTableDetailsResponse{
		Database: database,
		Table:    table,
		Columns:  make([]databaseTableColumn, 0, len(columnsResult.Rows)),
		Indexes:  databaseIndexesFromRows(indexesResult.Rows, "INDEX_NAME", "COLUMN_NAME", "NON_UNIQUE"),
	}
	if len(commentResult.Rows) > 0 {
		details.Comment = commentResult.Rows[0]["TABLE_COMMENT"]
	}

	primaryColumns := primaryColumnSet(details.Indexes)
	for _, row := range columnsResult.Rows {
		name := row["COLUMN_NAME"]
		details.Columns = append(details.Columns, databaseTableColumn{
			Name:       name,
			Type:       row["COLUMN_TYPE"],
			Nullable:   strings.EqualFold(row["IS_NULLABLE"], "YES"),
			PrimaryKey: row["COLUMN_KEY"] == "PRI" || primaryColumns[name],
			Default:    row["COLUMN_DEFAULT"],
			Extra:      row["EXTRA"],
			Comment:    row["COLUMN_COMMENT"],
		})
	}

	return details, nil
}

func postgresTableDetails(ctx context.Context, connection portainer.DatabaseConnection, database string, table string) (*databaseTableDetailsResponse, error) {
	if database == "" {
		database = "public"
	}

	columnsQuery := `
SELECT c.column_name, c.data_type, c.is_nullable, c.column_default, COALESCE(pgd.description, '') AS column_comment
FROM information_schema.columns c
LEFT JOIN pg_catalog.pg_class pc ON pc.relname = c.table_name
LEFT JOIN pg_catalog.pg_namespace pn ON pn.oid = pc.relnamespace AND pn.nspname = c.table_schema
LEFT JOIN pg_catalog.pg_description pgd ON pgd.objoid = pc.oid AND pgd.objsubid = c.ordinal_position
WHERE c.table_schema = ` + sqlStringLiteral(database) + `
  AND c.table_name = ` + sqlStringLiteral(table) + `
ORDER BY c.ordinal_position`
	columnsResult, err := executeDirectDatabaseQuery(ctx, connection, columnsQuery, false)
	if err != nil {
		return nil, err
	}

	indexesQuery := `
SELECT i.relname AS index_name, a.attname AS column_name, ix.indisunique AS is_unique, ix.indisprimary AS is_primary
FROM pg_class t
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_index ix ON t.oid = ix.indrelid
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
WHERE n.nspname = ` + sqlStringLiteral(database) + `
  AND t.relname = ` + sqlStringLiteral(table) + `
ORDER BY i.relname, a.attnum`
	indexesResult, err := executeDirectDatabaseQuery(ctx, connection, indexesQuery, false)
	if err != nil {
		return nil, err
	}

	commentQuery := `SELECT COALESCE(obj_description((` + sqlStringLiteral(database+"."+table) + `)::regclass, 'pg_class'), '') AS table_comment`
	commentResult, err := executeDirectDatabaseQuery(ctx, connection, commentQuery, false)
	if err != nil {
		return nil, err
	}

	indexes := postgresIndexesFromRows(indexesResult.Rows)
	details := &databaseTableDetailsResponse{
		Database: database,
		Table:    table,
		Columns:  make([]databaseTableColumn, 0, len(columnsResult.Rows)),
		Indexes:  indexes,
	}
	if len(commentResult.Rows) > 0 {
		details.Comment = commentResult.Rows[0]["table_comment"]
	}

	primaryColumns := primaryColumnSet(indexes)
	for _, row := range columnsResult.Rows {
		name := row["column_name"]
		details.Columns = append(details.Columns, databaseTableColumn{
			Name:       name,
			Type:       row["data_type"],
			Nullable:   strings.EqualFold(row["is_nullable"], "YES"),
			PrimaryKey: primaryColumns[name],
			Default:    row["column_default"],
			Comment:    row["column_comment"],
		})
	}

	return details, nil
}

// 表结构查询需要把数据库名、表名拼入信息模式 SQL；这里仅生成 SQL 字符串字面量，
// 不用于标识符拼接，避免名称里包含引号时破坏查询语义。
func sqlStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func databaseIndexesFromRows(rows []map[string]string, nameKey string, columnKey string, nonUniqueKey string) []databaseTableIndex {
	indexByName := map[string]int{}
	indexes := []databaseTableIndex{}
	for _, row := range rows {
		name := row[nameKey]
		index, ok := indexByName[name]
		if !ok {
			indexes = append(indexes, databaseTableIndex{
				Name:    name,
				Unique:  row[nonUniqueKey] == "0",
				Primary: strings.EqualFold(name, "PRIMARY"),
			})
			index = len(indexes) - 1
			indexByName[name] = index
		}
		indexes[index].Columns = append(indexes[index].Columns, row[columnKey])
	}

	return indexes
}

func postgresIndexesFromRows(rows []map[string]string) []databaseTableIndex {
	indexByName := map[string]int{}
	indexes := []databaseTableIndex{}
	for _, row := range rows {
		name := row["index_name"]
		index, ok := indexByName[name]
		if !ok {
			indexes = append(indexes, databaseTableIndex{
				Name:    name,
				Unique:  row["is_unique"] == "true",
				Primary: row["is_primary"] == "true",
			})
			index = len(indexes) - 1
			indexByName[name] = index
		}
		indexes[index].Columns = append(indexes[index].Columns, row["column_name"])
	}

	return indexes
}

func primaryColumnSet(indexes []databaseTableIndex) map[string]bool {
	columns := map[string]bool{}
	for _, index := range indexes {
		if !index.Primary {
			continue
		}
		for _, column := range index.Columns {
			columns[column] = true
		}
	}

	return columns
}
