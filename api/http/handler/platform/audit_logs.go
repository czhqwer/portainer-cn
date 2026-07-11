package platform

import (
	"net"
	"net/http"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/http/security"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

func (handler *Handler) auditLogList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	filters, handlerErr := auditLogListFilters(r)
	if handlerErr != nil {
		return handlerErr
	}
	context, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return handler.convertError(err)
	}
	if !context.IsAdmin && filters.projectID == 0 {
		return platformAccessDenied()
	}
	if filters.projectID != 0 {
		if _, handlerErr := handler.requireProjectPermission(r, filters.projectID, platformPermissionManage); handlerErr != nil {
			return handlerErr
		}
	}

	logs, err := handler.DataStore.PlatformAuditLog().ReadAll(func(log portainer.PlatformAuditLog) bool {
		if filters.projectID != 0 && log.ProjectID != filters.projectID {
			return false
		}
		if filters.releaseID != 0 && log.ReleaseID != filters.releaseID {
			return false
		}
		if filters.serviceDeploymentID != 0 && log.ServiceDeploymentID != filters.serviceDeploymentID {
			return false
		}
		if filters.action != "" && log.Action != filters.action {
			return false
		}

		return true
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, logs)
}

type auditLogFilters struct {
	projectID           portainer.PlatformProjectID
	releaseID           portainer.PlatformReleaseID
	serviceDeploymentID portainer.PlatformServiceDeploymentID
	action              portainer.PlatformAuditAction
}

func auditLogListFilters(r *http.Request) (auditLogFilters, *httperror.HandlerError) {
	projectID, err := optionalQueryID(r, "projectId")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	releaseID, err := optionalQueryID(r, "releaseId")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	serviceDeploymentID, err := optionalQueryID(r, "serviceDeploymentId")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}

	return auditLogFilters{
		projectID:           portainer.PlatformProjectID(projectID),
		releaseID:           portainer.PlatformReleaseID(releaseID),
		serviceDeploymentID: portainer.PlatformServiceDeploymentID(serviceDeploymentID),
		action:              portainer.PlatformAuditAction(r.URL.Query().Get("action")),
	}, nil
}

func (handler *Handler) createReleaseAuditLog(tx dataservices.DataStoreTx, r *http.Request, action portainer.PlatformAuditAction, result portainer.PlatformAuditResult, release portainer.PlatformRelease, before map[string]any, after map[string]any, failureReason string) error {
	audit := &portainer.PlatformAuditLog{
		Timestamp:           time.Now().Unix(),
		Action:              action,
		Result:              result,
		ProjectID:           release.ProjectID,
		EnvironmentID:       release.EnvironmentID,
		ApplicationID:       release.ApplicationID,
		ServiceDefinitionID: release.ServiceDefinitionID,
		ServiceDeploymentID: release.ServiceDeploymentID,
		ArtifactID:          release.ArtifactID,
		ReleaseID:           release.ID,
		BeforeSummary:       before,
		AfterSummary:        after,
		FailureReason:       failureReason,
	}
	handler.fillPlatformAuditRequestFields(tx, r, audit)

	return tx.PlatformAuditLog().Create(audit)
}

func (handler *Handler) fillPlatformAuditRequestFields(tx dataservices.DataStoreTx, r *http.Request, audit *portainer.PlatformAuditLog) {
	if audit == nil || r == nil {
		return
	}

	audit.RequestID = firstNonEmptyHeader(r, "X-Request-Id", "X-Request-ID", "X-Correlation-Id")
	audit.IPAddress = platformRequestOrigin(r)
	audit.UserAgent = r.UserAgent()

	context, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return
	}
	audit.OperatorUserID = context.UserID
	if context.User != nil {
		audit.OperatorUsername = context.User.Username
		return
	}
	if tx == nil {
		return
	}
	user, err := tx.User().Read(context.UserID)
	if err == nil && user != nil {
		audit.OperatorUsername = user.Username
	}
}

func platformRequestOrigin(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := r.Header.Get(header)
		if value == "" {
			continue
		}

		origin := strings.TrimSpace(strings.Split(value, ",")[0])
		if origin != "" {
			return origin
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}

func firstNonEmptyHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			return value
		}
	}

	return ""
}

func releaseAuditSummary(release portainer.PlatformRelease) map[string]any {
	return map[string]any{
		"status":                 string(release.Status),
		"failureReason":          release.FailureReason,
		"manualActionRequired":   release.ManualActionRequired,
		"resolutionAction":       string(release.ResolutionAction),
		"currentRuntimeResource": release.RuntimeSnapshot.CurrentRuntimeRef.ResourceID,
	}
}

func releaseAuditActionForStatus(status portainer.PlatformReleaseStatus) portainer.PlatformAuditAction {
	switch status {
	case portainer.PlatformReleaseStatusSucceeded:
		return portainer.PlatformAuditActionReleaseSucceeded
	case portainer.PlatformReleaseStatusRecoveryFailed:
		return portainer.PlatformAuditActionReleaseRecoveryFailed
	case portainer.PlatformReleaseStatusFailed:
		return portainer.PlatformAuditActionReleaseFailed
	case portainer.PlatformReleaseStatusCanceled:
		return portainer.PlatformAuditActionReleaseCanceled
	default:
		return ""
	}
}

func releaseAuditResultForStatus(status portainer.PlatformReleaseStatus) portainer.PlatformAuditResult {
	switch status {
	case portainer.PlatformReleaseStatusSucceeded,
		portainer.PlatformReleaseStatusCanceled,
		portainer.PlatformReleaseStatusResolved:
		return portainer.PlatformAuditResultSuccess
	default:
		return portainer.PlatformAuditResultFailed
	}
}
