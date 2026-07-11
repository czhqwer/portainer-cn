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
	"github.com/lib/pq"
	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
	"github.com/redis/go-redis/v9"
)

type databaseQueryPayload struct {
	Query              string `json:"Query"`
	Database           string `json:"Database"`
	Preview            bool   `json:"Preview"`
	ConfirmUnsafeWrite bool   `json:"ConfirmUnsafeWrite"`
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
	Columns              []string            `json:"Columns"`
	Rows                 []map[string]string `json:"Rows"`
	Message              string              `json:"Message"`
	Duration             float64             `json:"Duration"`
	RowsAffected         int64               `json:"RowsAffected,omitempty"`
	StatementType        string              `json:"StatementType,omitempty"`
	Preview              bool                `json:"Preview,omitempty"`
	RequiresConfirmation bool                `json:"RequiresConfirmation,omitempty"`
	UnsafeWrite          bool                `json:"UnsafeWrite,omitempty"`
	ErrorCode            string              `json:"ErrorCode,omitempty"`
	// Redis 单次查询的 Key/Type 对所有行相同，提到表头展示，避免每行重复。
	RedisKey  string `json:"RedisKey,omitempty"`
	RedisType string `json:"RedisType,omitempty"`
}

type databaseErrorResponse struct {
	Message   string `json:"Message"`
	Details   string `json:"Details,omitempty"`
	ErrorCode string `json:"ErrorCode"`
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

	var payload databaseQueryPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	queryConnection := *connection
	if payload.Database != "" {
		queryConnection.Database = payload.Database
	}

	statementType := databaseStatementType(payload.Query)
	if !payload.Preview && isUnsafeWriteStatement(payload.Query) && !payload.ConfirmUnsafeWrite {
		return response.JSON(w, &databaseQueryResult{
			Columns:              []string{},
			Rows:                 []map[string]string{},
			Message:              "UPDATE/DELETE without WHERE requires confirmation",
			StatementType:        statementType,
			RequiresConfirmation: true,
			UnsafeWrite:          true,
			ErrorCode:            "unsafe_write_requires_confirmation",
		})
	}

	result, err := executeDirectDatabaseQuery(r.Context(), queryConnection, payload.Query, payload.Preview)
	if err != nil {
		return writeDatabaseError(w, "Unable to execute database query", err)
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

	// 结果统一成 Key/Value/TTL/Type，并按命令正确展开 HGETALL 等成对结构，
	// 避免表格截断观感或 field/value 拆散后与详情弹窗不一致。
	key := redisCommandKey(args)
	ttlLabel := ""
	keyType := ""
	if key != "" {
		if ttl, ttlErr := client.TTL(ctx, key).Result(); ttlErr == nil {
			ttlLabel = redisTTLLabel(ttl)
		}
		if typed, typeErr := client.Type(ctx, key).Result(); typeErr == nil {
			keyType = typed
		}
	}

	rows, columns := redisCommandResultRows(args, value, keyType, ttlLabel)
	return &databaseQueryResult{
		Columns:   columns,
		Rows:      rows,
		Message:   fmt.Sprintf("%d row(s)", len(rows)),
		RedisKey:  key,
		RedisType: keyType,
	}, nil
}

// redisCommandKey 识别常见单 Key 命令的目标 Key，用于补充 TTL/Type 列。
func redisCommandKey(args []string) string {
	if len(args) < 2 {
		return ""
	}

	switch strings.ToUpper(args[0]) {
	case "GET", "GETDEL", "GETEX", "DUMP", "EXISTS", "TTL", "PTTL", "TYPE", "STRLEN",
		"HGET", "HGETALL", "HKEYS", "HVALS", "HLEN",
		"LLEN", "LRANGE", "LINDEX",
		"SMEMBERS", "SCARD", "SSCAN",
		"ZCARD", "ZRANGE", "ZREVRANGE", "ZSCORE":
		return args[1]
	default:
		return ""
	}
}

// redisCommandResultRows 生成表格行；Key/Type 已上移到结果头，行内只保留值与 TTL。
// list/set/zset 增加 Index（0,1,2...），hash 保留 Field。
func redisCommandResultRows(args []string, value any, keyType string, ttl string) ([]map[string]string, []string) {
	command := ""
	if len(args) > 0 {
		command = strings.ToUpper(args[0])
	}

	switch command {
	case "HGETALL":
		return redisHashPairsToRows(value, ttl)
	case "ZRANGE", "ZREVRANGE":
		if redisCommandHasWithScores(args) {
			return redisZSetPairsToRows(value, ttl)
		}
	}

	switch keyType {
	case "list", "set", "zset":
		return redisIndexedValueRows(value, ttl)
	case "hash":
		return redisHashPairsToRows(value, ttl)
	default:
		return redisScalarValueRows(value, ttl)
	}
}

func redisCommandHasWithScores(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(arg, "WITHSCORES") {
			return true
		}
	}
	return false
}

func redisScalarValueRows(value any, ttl string) ([]map[string]string, []string) {
	columns := []string{"Value", "TTL"}
	if value == nil {
		return []map[string]string{}, columns
	}

	return []map[string]string{
		{
			"Value": databaseValueToString(value),
			"TTL":   ttl,
		},
	}, columns
}

func redisIndexedValueRows(value any, ttl string) ([]map[string]string, []string) {
	columns := []string{"Index", "Value", "TTL"}
	rows := []map[string]string{}

	appendItem := func(item any) {
		rows = append(rows, map[string]string{
			"Index": strconv.Itoa(len(rows)),
			"Value": databaseValueToString(item),
			"TTL":   ttl,
		})
	}

	switch v := value.(type) {
	case nil:
		return rows, columns
	case []any:
		for _, item := range v {
			appendItem(item)
		}
	case []string:
		for _, item := range v {
			appendItem(item)
		}
	default:
		appendItem(v)
	}

	return rows, columns
}

func redisHashPairsToRows(value any, ttl string) ([]map[string]string, []string) {
	columns := []string{"Field", "Value", "TTL"}
	rows := []map[string]string{}
	items, ok := value.([]any)
	if !ok {
		if value != nil {
			rows = append(rows, map[string]string{
				"Field": "",
				"Value": databaseValueToString(value),
				"TTL":   ttl,
			})
		}
		return rows, columns
	}

	for i := 0; i+1 < len(items); i += 2 {
		rows = append(rows, map[string]string{
			"Field": databaseValueToString(items[i]),
			"Value": databaseValueToString(items[i+1]),
			"TTL":   ttl,
		})
	}
	return rows, columns
}

func redisZSetPairsToRows(value any, ttl string) ([]map[string]string, []string) {
	columns := []string{"Index", "Value", "TTL"}
	rows := []map[string]string{}
	items, ok := value.([]any)
	if !ok {
		if value != nil {
			rows = append(rows, map[string]string{
				"Index": "0",
				"Value": databaseValueToString(value),
				"TTL":   ttl,
			})
		}
		return rows, columns
	}

	for i := 0; i+1 < len(items); i += 2 {
		member := databaseValueToString(items[i])
		score := databaseValueToString(items[i+1])
		rows = append(rows, map[string]string{
			"Index": strconv.Itoa(len(rows)),
			"Value": member + " (" + score + ")",
			"TTL":   ttl,
		})
	}
	return rows, columns
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

func redisValueRows(value any, ttl string) []map[string]string {
	rows := []map[string]string{}

	appendValue := func(v any) {
		row := map[string]string{
			"Index": strconv.Itoa(len(rows) + 1),
			"Value": databaseValueToString(v),
		}
		if ttl != "" {
			row["TTL"] = ttl
		}
		rows = append(rows, row)
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

func isUnsafeWriteStatement(query string) bool {
	statementType := databaseStatementType(query)
	if statementType != "update" && statementType != "delete" {
		return false
	}

	return !containsSQLKeywordOutsideLiterals(query, "where")
}

// 写入保护只关心 WHERE 是否出现在真正的 SQL 结构里；
// 这里先去掉注释和字符串字面量，避免因为文本内容里包含 where 而误放行危险 UPDATE/DELETE。
func containsSQLKeywordOutsideLiterals(query string, keyword string) bool {
	normalized := stripSQLLiteralsAndComments(query)
	fields := strings.FieldsFunc(normalized, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	})

	for _, field := range fields {
		if strings.EqualFold(field, keyword) {
			return true
		}
	}

	return false
}

func stripSQLLiteralsAndComments(query string) string {
	var builder strings.Builder
	var quote rune
	escaped := false
	inLineComment := false
	inBlockComment := false
	runes := []rune(query)

	for i := 0; i < len(runes); i++ {
		current := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if inLineComment {
			if current == '\n' {
				inLineComment = false
				builder.WriteRune(current)
			}
			continue
		}

		if inBlockComment {
			if current == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' && quote != '\'' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			builder.WriteRune(' ')
			continue
		}

		if current == '-' && next == '-' {
			inLineComment = true
			i++
			continue
		}
		if current == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		if current == '\'' || current == '"' || current == '`' {
			quote = current
			builder.WriteRune(' ')
			continue
		}

		builder.WriteRune(current)
	}

	return builder.String()
}

func writeDatabaseError(w http.ResponseWriter, message string, err error) *httperror.HandlerError {
	code, friendlyMessage, status := classifyDatabaseError(err)
	if friendlyMessage == "" {
		friendlyMessage = message
	}
	return response.JSONWithStatus(w, databaseErrorResponse{
		Message:   friendlyMessage,
		Details:   err.Error(),
		ErrorCode: code,
	}, status)
}

// 数据库错误来自不同驱动，统一映射成前端可展示的诊断码和中文可翻译消息。
func classifyDatabaseError(err error) (string, string, int) {
	if err == nil {
		return "unknown", "Database operation failed", http.StatusInternalServerError
	}

	if errors.Is(err, context.Canceled) {
		return "request_canceled", "Query was canceled", http.StatusRequestTimeout
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", "Query timed out", http.StatusGatewayTimeout
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1045:
			return "authentication_failed", "Database authentication failed", http.StatusUnauthorized
		case 1049:
			return "database_not_found", "Database does not exist", http.StatusBadRequest
		case 1146:
			return "object_not_found", "Database object does not exist", http.StatusBadRequest
		case 1064:
			return "sql_error", "SQL syntax error", http.StatusBadRequest
		}
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch string(pqErr.Code) {
		case "28P01":
			return "authentication_failed", "Database authentication failed", http.StatusUnauthorized
		case "3D000":
			return "database_not_found", "Database does not exist", http.StatusBadRequest
		case "42P01":
			return "object_not_found", "Database object does not exist", http.StatusBadRequest
		case "42601":
			return "sql_error", "SQL syntax error", http.StatusBadRequest
		case "42501":
			return "permission_denied", "Database permission denied", http.StatusForbidden
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout", "Database connection timed out", http.StatusGatewayTimeout
	}

	lowerErr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lowerErr, "connection refused"),
		strings.Contains(lowerErr, "no such host"),
		strings.Contains(lowerErr, "network is unreachable"),
		strings.Contains(lowerErr, "i/o timeout"):
		return "network_unreachable", "Database host is unreachable", http.StatusBadGateway
	case strings.Contains(lowerErr, "unable to upgrade to tcp"),
		strings.Contains(lowerErr, "agent"):
		return "agent_unreachable", "Portainer Agent is unreachable", http.StatusBadGateway
	case strings.Contains(lowerErr, "access denied"),
		strings.Contains(lowerErr, "password authentication failed"):
		return "authentication_failed", "Database authentication failed", http.StatusUnauthorized
	case strings.Contains(lowerErr, "syntax"):
		return "sql_error", "SQL syntax error", http.StatusBadRequest
	}

	return "database_error", "Database operation failed", http.StatusInternalServerError
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
