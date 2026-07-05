package security

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"

	"github.com/rs/zerolog/log"
	"github.com/segmentio/encoding/json"
)

type auditResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *auditResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}

	return w.ResponseWriter.Write(data)
}

func (bouncer *RequestBouncer) mwAuditActivity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isActivityAuditCandidate(r) {
			next.ServeHTTP(w, r)
			return
		}

		recorder := &auditResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		if recorder.statusCode == 0 {
			recorder.statusCode = http.StatusOK
		}

		if !shouldAuditActivity(r, recorder.statusCode) {
			return
		}

		tokenData, err := RetrieveTokenData(r)
		if err != nil {
			log.Debug().Err(err).Msg("skipping activity audit log without token data")
			return
		}

		if err := bouncer.logActivity(tokenData.Username, r, recorder.statusCode); err != nil {
			log.Warn().Err(err).Msg("failed to write user activity log")
		}
	})
}

func shouldAuditActivity(r *http.Request, statusCode int) bool {
	if statusCode >= http.StatusBadRequest {
		return false
	}

	return isActivityAuditCandidate(r)
}

func isActivityAuditCandidate(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}

	path := r.URL.Path
	return !strings.HasPrefix(path, "/api/auth") &&
		!strings.HasPrefix(path, "/api/useractivity") &&
		!strings.HasPrefix(path, "/api/websocket")
}

func (bouncer *RequestBouncer) logActivity(username string, r *http.Request, statusCode int) error {
	payload, err := activityPayload(r, statusCode)
	if err != nil {
		return err
	}

	logEntry := &portainer.UserActivityLog{
		Timestamp: time.Now().Unix(),
		Context:   activityContext(r.URL.Path),
		Action:    activityAction(r.Method, r.URL.Path),
		Username:  username,
		Payload:   payload,
	}

	return bouncer.dataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if err := CleanExpiredAuditLogs(tx); err != nil {
			return err
		}

		return tx.UserActivityLog().Create(logEntry)
	})
}

func activityPayload(r *http.Request, statusCode int) (string, error) {
	payload := map[string]any{
		"method":     r.Method,
		"path":       r.URL.Path,
		"statusCode": statusCode,
	}

	if resourceID := lastPathSegment(r.URL.Path); resourceID != "" {
		payload["resourceId"] = resourceID
	}

	if endpointID := endpointIDFromPath(r.URL.Path); endpointID != "" {
		payload["endpointId"] = endpointID
	}

	if len(r.URL.Query()) > 0 {
		payload["query"] = r.URL.Query()
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

func activityContext(path string) string {
	if endpointID := endpointIDFromPath(path); endpointID != "" {
		return "Environment " + endpointID
	}

	return "Portainer"
}

func activityAction(method string, path string) string {
	resource := activityResource(path)

	switch method {
	case http.MethodPost:
		return "Create " + resource
	case http.MethodPut, http.MethodPatch:
		return "Update " + resource
	case http.MethodDelete:
		return "Delete " + resource
	default:
		return method + " " + path
	}
}

func activityResource(path string) string {
	segments := pathSegments(path)
	if len(segments) == 0 {
		return "resource"
	}

	for i, segment := range segments {
		switch segment {
		case "stacks":
			return "stack"
		case "users":
			return "user"
		case "teams":
			return "team"
		case "team_memberships":
			return "team membership"
		case "endpoints":
			if i+2 < len(segments) {
				return singular(segments[i+2])
			}
			return "environment"
		case "endpoint_groups":
			return "environment group"
		case "registries":
			return "registry"
		case "resource_controls":
			return "resource control"
		case "tags":
			return "tag"
		case "webhooks":
			return "webhook"
		case "settings":
			return "settings"
		}
	}

	return singular(segments[len(segments)-1])
}

func endpointIDFromPath(path string) string {
	segments := pathSegments(path)
	for i, segment := range segments {
		if segment == "endpoints" && i+1 < len(segments) {
			if _, err := strconv.Atoi(segments[i+1]); err == nil {
				return segments[i+1]
			}
		}
	}

	return ""
}

func lastPathSegment(path string) string {
	segments := pathSegments(path)
	if len(segments) == 0 {
		return ""
	}

	return segments[len(segments)-1]
}

func pathSegments(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && part != "api" {
			segments = append(segments, part)
		}
	}

	return segments
}

func singular(resource string) string {
	resource = strings.ReplaceAll(resource, "_", " ")
	if strings.HasSuffix(resource, "ies") {
		return strings.TrimSuffix(resource, "ies") + "y"
	}
	if strings.HasSuffix(resource, "s") {
		return strings.TrimSuffix(resource, "s")
	}

	return resource
}

func CleanExpiredAuditLogs(tx dataservices.DataStoreTx) error {
	settings, err := tx.Settings().Settings()
	if err != nil {
		return err
	}

	retentionDays := settings.AuditLogRetentionDays
	if retentionDays == 0 {
		retentionDays = portainer.DefaultAuditLogRetentionDays
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()

	activityLogs, err := tx.UserActivityLog().ReadAll(func(logEntry portainer.UserActivityLog) bool {
		return logEntry.Timestamp < cutoff
	})
	if err != nil {
		return err
	}

	for _, logEntry := range activityLogs {
		if err := tx.UserActivityLog().Delete(logEntry.ID); err != nil {
			return err
		}
	}

	authLogs, err := tx.UserAuthenticationLog().ReadAll(func(logEntry portainer.UserAuthenticationLog) bool {
		return logEntry.Timestamp < cutoff
	})
	if err != nil {
		return err
	}

	for _, logEntry := range authLogs {
		if err := tx.UserAuthenticationLog().Delete(logEntry.ID); err != nil {
			return err
		}
	}

	return nil
}
