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

type activityLogsResponse struct {
	Logs       []portainer.UserActivityLog `json:"logs"`
	TotalCount int                        `json:"totalCount"`
}

func (handler *Handler) activityLogs(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	options := parseQuery(r)
	logs, err := handler.filteredActivityLogs(options)
	if err != nil {
		return httperror.InternalServerError("Unable to retrieve activity logs", err)
	}

	return response.JSON(w, activityLogsResponse{
		Logs:       paginate(logs, options.offset, options.limit),
		TotalCount: len(logs),
	})
}

func (handler *Handler) activityLogsCSV(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	options := parseQuery(r)
	options.limit = 0

	logs, err := handler.filteredActivityLogs(options)
	if err != nil {
		return httperror.InternalServerError("Unable to retrieve activity logs", err)
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=activity-logs.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write([]string{"Time", "User", "Environment", "Action", "Payload"}); err != nil {
		return httperror.InternalServerError("Unable to write activity logs csv", err)
	}

	for _, logEntry := range logs {
		if err := writer.Write([]string{
			strconv.FormatInt(logEntry.Timestamp, 10),
			logEntry.Username,
			logEntry.Context,
			logEntry.Action,
			logEntry.Payload,
		}); err != nil {
			return httperror.InternalServerError("Unable to write activity logs csv", err)
		}
	}

	return nil
}

func (handler *Handler) filteredActivityLogs(options queryOptions) ([]portainer.UserActivityLog, error) {
	logs, err := handler.DataStore.UserActivityLog().ReadAll()
	if err != nil {
		return nil, err
	}

	filtered := make([]portainer.UserActivityLog, 0, len(logs))
	for _, logEntry := range logs {
		if !matchTime(logEntry.Timestamp, options) {
			continue
		}
		if options.keyword != "" && !strings.Contains(strings.ToLower(logEntry.Username+" "+logEntry.Context+" "+logEntry.Action), options.keyword) {
			continue
		}

		filtered = append(filtered, logEntry)
	}

	sortActivityLogs(filtered, options)
	return filtered, nil
}

func sortActivityLogs(logs []portainer.UserActivityLog, options queryOptions) {
	sortBy := strings.ToLower(options.sortBy)
	if sortBy == "" {
		sortBy = "timestamp"
	}

	slices.SortFunc(logs, func(a, b portainer.UserActivityLog) int {
		var result int
		switch sortBy {
		case "context":
			result = strings.Compare(a.Context, b.Context)
		case "action":
			result = strings.Compare(a.Action, b.Action)
		case "username", "user":
			result = strings.Compare(a.Username, b.Username)
		default:
			result = int(a.Timestamp - b.Timestamp)
		}

		if options.sortDesc {
			return -result
		}

		return result
	})
}

