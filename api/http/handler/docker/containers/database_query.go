package containers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
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
	Stderr        string              `json:"Stderr,omitempty"`
}

func (handler *Handler) databaseConnectionQuery(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	containerID, err := request.RetrieveRouteVariableValue(r, "containerId")
	if err != nil {
		return httperror.BadRequest("Invalid container identifier route variable", err)
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, containerID)
	if httpErr != nil {
		return httpErr
	}

	var payload databaseQueryPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return httperror.BadRequest("Invalid request payload", err)
	}

	client, _, httpErr := handler.prepareDatabaseContainerAccess(r, containerID)
	if httpErr != nil {
		return httpErr
	}
	defer client.Close()

	queryConnection := *connection
	if payload.Database != "" {
		queryConnection.Database = payload.Database
	}

	result, err := executeDatabaseQuery(r.Context(), client, containerID, queryConnection, payload.Query, payload.Preview)
	if err != nil {
		return httperror.InternalServerError("Unable to execute database query", err)
	}

	return response.JSON(w, result)
}

func executeDatabaseQuery(parentCtx context.Context, client *dockerclient.Client, containerID string, connection portainer.DatabaseConnection, query string, preview bool) (*databaseQueryResult, error) {
	timeout := connection.QueryTimeout
	if timeout < minQueryTimeout {
		timeout = defaultQueryTimeout
	}
	if timeout > maxQueryTimeout {
		timeout = maxQueryTimeout
	}

	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(timeout)*time.Second)
	defer cancel()

	statementType := databaseStatementType(query)
	if preview {
		if connection.Type == "redis" {
			return nil, errors.New("preview is not supported for Redis commands")
		}
		if statementType != "update" && statementType != "delete" {
			return nil, errors.New("preview is only supported for update and delete statements")
		}
	}

	commandQuery := query
	if preview {
		commandQuery = previewDatabaseQuery(connection.Type, query)
	}

	cmd, env, err := databaseExecCommand(connection, commandQuery)
	if err != nil {
		return nil, err
	}
	start := time.Now()

	exec, err := client.ContainerExecCreate(ctx, containerID, dockercontainer.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Env:          env,
		Cmd:          cmd,
	})
	if err != nil {
		return nil, err
	}

	attach, err := client.ContainerExecAttach(ctx, exec.ID, dockercontainer.ExecAttachOptions{})
	if err != nil {
		return nil, err
	}
	defer attach.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attach.Reader); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	inspect, err := client.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return nil, err
	}

	if inspect.ExitCode != 0 {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("query command failed with exit code %d: %s", inspect.ExitCode, errText)
	}

	result := parseDatabaseOutput(connection.Type, stdout.String())
	if preview {
		result = previewDatabaseResult(connection.Type, statementType, stdout.String(), result)
	} else {
		result.StatementType = statementType
	}
	result.Stderr = strings.TrimSpace(stderr.String())
	result.Duration = time.Since(start).Seconds()
	if result.Message == "" {
		result.Message = fmt.Sprintf("%d row(s)", len(result.Rows))
	}

	return result, nil
}

func databaseExecCommand(connection portainer.DatabaseConnection, query string) ([]string, []string, error) {
	env := []string{
		"PORTAINER_DB_HOST=" + connection.Host,
		"PORTAINER_DB_PORT=" + strconv.Itoa(connection.Port),
	}

	if connection.Password != "" {
		switch connection.Type {
		case "mysql", "mariadb":
			env = append(env, "MYSQL_PWD="+connection.Password)
		case "postgres":
			env = append(env, "PGPASSWORD="+connection.Password)
		case "redis":
			env = append(env, "REDISCLI_AUTH="+connection.Password)
		}
	}

	switch connection.Type {
	case "mysql", "mariadb":
		binary := "mysql"
		if connection.Type == "mariadb" {
			binary = "mariadb"
		}
		cmd := []string{binary, "-h", connection.Host, "-P", strconv.Itoa(connection.Port), "--batch", "--raw", "--default-character-set=utf8mb4", "-e", query}
		if connection.Username != "" {
			cmd = append(cmd[:5], append([]string{"-u", connection.Username}, cmd[5:]...)...)
		}
		if connection.Database != "" {
			cmd = append(cmd, connection.Database)
		}
		return cmd, env, nil

	case "postgres":
		cmd := []string{"psql", "-h", connection.Host, "-p", strconv.Itoa(connection.Port), "-X", "-A", "-F", "\t", "-P", "footer=off", "-c", query}
		if connection.Username != "" {
			cmd = append([]string{cmd[0], "-U", connection.Username}, cmd[1:]...)
		}
		if connection.Database != "" {
			cmd = append(cmd, "-d", connection.Database)
		}
		return cmd, env, nil

	case "redis":
		cmd := []string{"redis-cli", "-h", connection.Host, "-p", strconv.Itoa(connection.Port), "--raw"}
		args, err := splitRedisCommand(query)
		if err != nil {
			return nil, nil, err
		}
		cmd = append(cmd, args...)
		return cmd, env, nil
	}

	return []string{}, env, nil
}

func previewDatabaseQuery(connectionType portainer.DatabaseConnectionType, query string) string {
	statement := strings.TrimRight(strings.TrimSpace(query), ";")
	switch connectionType {
	case "mysql", "mariadb":
		return fmt.Sprintf("START TRANSACTION; %s; SELECT ROW_COUNT() AS RowsAffected; ROLLBACK;", statement)
	case "postgres":
		return fmt.Sprintf("BEGIN; %s; ROLLBACK;", statement)
	default:
		return query
	}
}

func previewDatabaseResult(connectionType portainer.DatabaseConnectionType, statementType string, output string, parsed *databaseQueryResult) *databaseQueryResult {
	rowsAffected := int64(0)

	switch connectionType {
	case "mysql", "mariadb":
		if len(parsed.Rows) > 0 {
			if value, ok := parsed.Rows[0]["RowsAffected"]; ok {
				rowsAffected, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			}
		}
	case "postgres":
		rowsAffected = postgresRowsAffected(output)
	}

	return &databaseQueryResult{
		Columns:       []string{},
		Rows:          []map[string]string{},
		RowsAffected:  rowsAffected,
		StatementType: statementType,
		Preview:       true,
		Message:       fmt.Sprintf("%d row(s) affected", rowsAffected),
	}
}

func postgresRowsAffected(output string) int64 {
	matches := regexp.MustCompile(`(?m)^(?:UPDATE|DELETE)\s+(\d+)\s*$`).FindStringSubmatch(output)
	if len(matches) != 2 {
		return 0
	}

	rowsAffected, _ := strconv.ParseInt(matches[1], 10, 64)
	return rowsAffected
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

func parseDatabaseOutput(connectionType portainer.DatabaseConnectionType, output string) *databaseQueryResult {
	if connectionType == "redis" {
		return parseRedisOutput(output)
	}

	return parseTabularOutput(output)
}

func parseTabularOutput(output string) *databaseQueryResult {
	lines := normalizedOutputLines(output)
	if len(lines) == 0 {
		return &databaseQueryResult{
			Columns: []string{},
			Rows:    []map[string]string{},
		}
	}

	if len(lines) == 1 && !strings.Contains(lines[0], "\t") {
		return &databaseQueryResult{
			Columns: []string{"Result"},
			Rows: []map[string]string{
				{"Result": lines[0]},
			},
		}
	}

	headers := strings.Split(lines[0], "\t")

	rows := make([]map[string]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		values := strings.Split(line, "\t")
		row := map[string]string{}
		for index, header := range headers {
			value := ""
			if index < len(values) {
				value = values[index]
			}
			row[header] = value
		}
		rows = append(rows, row)
	}

	return &databaseQueryResult{
		Columns: headers,
		Rows:    rows,
	}
}

func parseRedisOutput(output string) *databaseQueryResult {
	lines := normalizedOutputLines(output)
	rows := make([]map[string]string, 0, len(lines))
	for i, line := range lines {
		rows = append(rows, map[string]string{
			"Index": strconv.Itoa(i + 1),
			"Value": line,
		})
	}

	return &databaseQueryResult{
		Columns: []string{"Index", "Value"},
		Rows:    rows,
	}
}

func normalizedOutputLines(output string) []string {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	output = strings.TrimSpace(output)
	if output == "" {
		return []string{}
	}

	return strings.Split(output, "\n")
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
