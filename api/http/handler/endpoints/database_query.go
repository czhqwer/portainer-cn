package endpoints

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
	"github.com/redis/go-redis/v9"
)

type databaseQueryPayload struct {
	Query    string `json:"Query"`
	Database string `json:"Database"`
	Preview  bool   `json:"Preview"`
}

func (payload *databaseQueryPayload) Validate(r *http.Request) error {
	payload.Query = strings.TrimSpace(payload.Query)
	payload.Database = strings.TrimSpace(payload.Database)
	if payload.Query == "" {
		return errors.New("query is required")
	}

	return nil
}

type databaseQueryResult struct {
	Columns       []string            `json:"Columns"`
	Rows          []map[string]string `json:"Rows"`
	Message       string              `json:"Message"`
	Duration      float64             `json:"Duration"`
	RowsAffected  int64               `json:"RowsAffected,omitempty"`
	StatementType string              `json:"StatementType,omitempty"`
	Preview       bool                `json:"Preview,omitempty"`
}

func (handler *Handler) databaseConnectionQuery(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, endpointID)
	if httpErr != nil {
		return httpErr
	}
	if connection.ContainerID != "" {
		return httperror.BadRequest("Container database connections must be queried through the Docker endpoint", errors.New("container database connection"))
	}

	var payload databaseQueryPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	queryConnection := *connection
	if payload.Database != "" {
		queryConnection.Database = payload.Database
	}

	result, err := executeDirectDatabaseQuery(r.Context(), queryConnection, payload.Query, payload.Preview)
	if err != nil {
		return httperror.InternalServerError("Unable to execute database query", err)
	}

	return response.JSON(w, result)
}

func executeDirectDatabaseQuery(parentCtx context.Context, connection portainer.DatabaseConnection, query string, preview bool) (*databaseQueryResult, error) {
	timeout := normalizedQueryTimeout(connection.QueryTimeout)
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(timeout)*time.Second)
	defer cancel()

	start := time.Now()

	var (
		result *databaseQueryResult
		err    error
	)

	switch connection.Type {
	case "mysql", "mariadb":
		result, err = executeSQLQuery(ctx, "mysql", mysqlDSN(connection, timeout), query, preview)
	case "postgres":
		result, err = executeSQLQuery(ctx, "postgres", postgresDSN(connection, timeout), query, preview)
	case "redis":
		if preview {
			return nil, errors.New("preview is not supported for Redis commands")
		}
		result, err = executeRedisCommand(ctx, connection, query)
	default:
		return nil, errors.New("unsupported database type")
	}
	if err != nil {
		return nil, err
	}

	result.Duration = time.Since(start).Seconds()
	if result.Message == "" {
		result.Message = fmt.Sprintf("%d row(s)", len(result.Rows))
	}

	return result, nil
}

func executeSQLQuery(ctx context.Context, driverName string, dsn string, query string, preview bool) (*databaseQueryResult, error) {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	statementType := databaseStatementType(query)
	if preview {
		if statementType != "update" && statementType != "delete" {
			return nil, errors.New("preview is only supported for update and delete statements")
		}

		return previewSQLExecution(ctx, db, query, statementType)
	}

	if statementType != "select" && statementType != "show" && statementType != "with" && statementType != "describe" && statementType != "desc" && statementType != "explain" {
		return executeSQLStatement(ctx, db, query, statementType)
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	resultRows := make([]map[string]string, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePointers := make([]any, len(columns))
		for i := range values {
			valuePointers[i] = &values[i]
		}

		if err := rows.Scan(valuePointers...); err != nil {
			return nil, err
		}

		row := map[string]string{}
		for i, column := range columns {
			row[column] = databaseValueToString(values[i])
		}
		resultRows = append(resultRows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &databaseQueryResult{
		Columns:       columns,
		Rows:          resultRows,
		StatementType: statementType,
	}, nil
}

func executeSQLStatement(ctx context.Context, db *sql.DB, query string, statementType string) (*databaseQueryResult, error) {
	result, err := db.ExecContext(ctx, query)
	if err != nil {
		return nil, err
	}

	rowsAffected, _ := result.RowsAffected()

	return &databaseQueryResult{
		Columns:       []string{},
		Rows:          []map[string]string{},
		RowsAffected:  rowsAffected,
		StatementType: statementType,
		Message:       fmt.Sprintf("%d row(s) affected", rowsAffected),
	}, nil
}

func previewSQLExecution(ctx context.Context, db *sql.DB, query string, statementType string) (*databaseQueryResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	result, execErr := tx.ExecContext(ctx, query)
	rowsAffected := int64(0)
	if execErr == nil {
		rowsAffected, _ = result.RowsAffected()
	}

	rollbackErr := tx.Rollback()
	if execErr != nil {
		return nil, execErr
	}
	if rollbackErr != nil {
		return nil, rollbackErr
	}

	return &databaseQueryResult{
		Columns:       []string{},
		Rows:          []map[string]string{},
		RowsAffected:  rowsAffected,
		StatementType: statementType,
		Preview:       true,
		Message:       fmt.Sprintf("%d row(s) affected", rowsAffected),
	}, nil
}

func executeRedisCommand(ctx context.Context, connection portainer.DatabaseConnection, query string) (*databaseQueryResult, error) {
	args, err := splitRedisCommand(query)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("redis command is required")
	}

	dbIndex := 0
	if connection.Database != "" {
		dbIndex, err = strconv.Atoi(connection.Database)
		if err != nil {
			return nil, errors.New("redis database must be a number")
		}
	}

	client := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(connection.Host, strconv.Itoa(connection.Port)),
		Username: connection.Username,
		Password: connection.Password,
		DB:       dbIndex,
	})
	defer client.Close()

	commandArgs := make([]any, len(args))
	for i, arg := range args {
		commandArgs[i] = arg
	}

	value, err := client.Do(ctx, commandArgs...).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	if errors.Is(err, redis.Nil) {
		value = nil
	}

	rows := redisValueRows(value)
	return &databaseQueryResult{
		Columns: []string{"Index", "Value"},
		Rows:    rows,
		Message: fmt.Sprintf("%d row(s)", len(rows)),
	}, nil
}

func mysqlDSN(connection portainer.DatabaseConnection, timeout int) string {
	config := mysql.NewConfig()
	config.User = connection.Username
	config.Passwd = connection.Password
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(connection.Host, strconv.Itoa(connection.Port))
	config.DBName = connection.Database
	config.Params = map[string]string{"charset": "utf8mb4"}
	config.Timeout = time.Duration(timeout) * time.Second
	config.ReadTimeout = time.Duration(timeout) * time.Second
	config.WriteTimeout = time.Duration(timeout) * time.Second

	return config.FormatDSN()
}

func postgresDSN(connection portainer.DatabaseConnection, timeout int) string {
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(connection.Host, strconv.Itoa(connection.Port)),
		Path:   connection.Database,
	}
	if connection.Username != "" {
		if connection.Password != "" {
			u.User = url.UserPassword(connection.Username, connection.Password)
		} else {
			u.User = url.User(connection.Username)
		}
	}

	query := u.Query()
	query.Set("sslmode", "disable")
	query.Set("connect_timeout", strconv.Itoa(timeout))
	u.RawQuery = query.Encode()

	return u.String()
}

func databaseValueToString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return fmt.Sprint(v)
	}
}

func redisValueRows(value any) []map[string]string {
	rows := []map[string]string{}

	appendValue := func(v any) {
		rows = append(rows, map[string]string{
			"Index": strconv.Itoa(len(rows) + 1),
			"Value": databaseValueToString(v),
		})
	}

	switch v := value.(type) {
	case nil:
		return rows
	case []any:
		for _, item := range v {
			appendValue(item)
		}
	case []string:
		for _, item := range v {
			appendValue(item)
		}
	default:
		appendValue(v)
	}

	return rows
}

func normalizedQueryTimeout(timeout int) int {
	if timeout < minQueryTimeout {
		return defaultQueryTimeout
	}
	if timeout > maxQueryTimeout {
		return maxQueryTimeout
	}

	return timeout
}

func databaseStatementType(query string) string {
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		return strings.ToLower(fields[0])
	}

	return ""
}

func splitRedisCommand(command string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false
	inToken := false

	for _, r := range command {
		if escaped {
			current.WriteRune(r)
			escaped = false
			inToken = true
			continue
		}

		if r == '\\' && quote != '\'' {
			escaped = true
			inToken = true
			continue
		}

		if quote != 0 {
			if r == quote {
				quote = 0
				inToken = true
				continue
			}

			current.WriteRune(r)
			inToken = true
			continue
		}

		if r == '"' || r == '\'' {
			quote = r
			inToken = true
			continue
		}

		if unicode.IsSpace(r) {
			if inToken {
				args = append(args, current.String())
				current.Reset()
				inToken = false
			}
			continue
		}

		current.WriteRune(r)
		inToken = true
	}

	if escaped {
		current.WriteRune('\\')
	}

	if quote != 0 {
		return nil, errors.New("unterminated quote in Redis command")
	}

	if inToken {
		args = append(args, current.String())
	}

	return args, nil
}
