package useractivity

import (
	"encoding/csv"
	"net/http"
	"slices"
	"strconv"
	"strings"

	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type authLogsResponse struct {
	Logs       []portainer.UserAuthenticationLog `json:"logs"`
	TotalCount int                              `json:"totalCount"`
}

func (handler *Handler) authLogs(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	options := parseQuery(r)
	logs, err := handler.filteredAuthLogs(options)
	if err != nil {
		return httperror.InternalServerError("Unable to retrieve authentication logs", err)
	}

	return response.JSON(w, authLogsResponse{
		Logs:       paginate(logs, options.offset, options.limit),
		TotalCount: len(logs),
	})
}

func (handler *Handler) authLogsCSV(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	options := parseQuery(r)
	options.limit = 0

	logs, err := handler.filteredAuthLogs(options)
	if err != nil {
		return httperror.InternalServerError("Unable to retrieve authentication logs", err)
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=authentication-logs.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write([]string{"Time", "Origin", "Context", "User", "Result"}); err != nil {
		return httperror.InternalServerError("Unable to write authentication logs csv", err)
	}

	for _, logEntry := range logs {
		if err := writer.Write([]string{
			strconv.FormatInt(logEntry.Timestamp, 10),
			logEntry.Origin,
			strconv.Itoa(int(logEntry.Context)),
			logEntry.Username,
			strconv.Itoa(int(logEntry.Type)),
		}); err != nil {
			return httperror.InternalServerError("Unable to write authentication logs csv", err)
		}
	}

	return nil
}

func (handler *Handler) filteredAuthLogs(options queryOptions) ([]portainer.UserAuthenticationLog, error) {
	logs, err := handler.DataStore.UserAuthenticationLog().ReadAll()
	if err != nil {
		return nil, err
	}

	filtered := make([]portainer.UserAuthenticationLog, 0, len(logs))
	for _, logEntry := range logs {
		if !matchTime(logEntry.Timestamp, options) {
			continue
		}
		if len(options.contexts) > 0 && !options.contexts[int(logEntry.Context)] {
			continue
		}
		if len(options.types) > 0 && !options.types[int(logEntry.Type)] {
			continue
		}
		if options.keyword != "" && !strings.Contains(strings.ToLower(logEntry.Username+" "+logEntry.Origin), options.keyword) {
			continue
		}

		filtered = append(filtered, logEntry)
	}

	sortAuthLogs(filtered, options)
	return filtered, nil
}

func sortAuthLogs(logs []portainer.UserAuthenticationLog, options queryOptions) {
	sortBy := strings.ToLower(options.sortBy)
	if sortBy == "" {
		sortBy = "timestamp"
	}

	slices.SortFunc(logs, func(a, b portainer.UserAuthenticationLog) int {
		var result int
		switch sortBy {
		case "origin":
			result = strings.Compare(a.Origin, b.Origin)
		case "context":
			result = int(a.Context) - int(b.Context)
		case "username", "user":
			result = strings.Compare(a.Username, b.Username)
		case "type", "result":
			result = int(a.Type) - int(b.Type)
		default:
			result = int(a.Timestamp - b.Timestamp)
		}

		if options.sortDesc {
			return -result
		}

		return result
	})
}

func matchTime(timestamp int64, options queryOptions) bool {
	if options.after > 0 && timestamp < options.after {
		return false
	}
	if options.before > 0 && timestamp > options.before {
		return false
	}

	return true
}

