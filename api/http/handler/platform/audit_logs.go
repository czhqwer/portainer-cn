package platform

import (
	"net"
	"net/http"
	"sort"
	"strconv"
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
		if filters.environmentID != 0 && log.EnvironmentID != filters.environmentID {
			return false
		}
		if filters.applicationID != 0 && log.ApplicationID != filters.applicationID {
			return false
		}
		if filters.serviceDefinitionID != 0 && log.ServiceDefinitionID != filters.serviceDefinitionID {
			return false
		}
		if filters.artifactID != 0 && log.ArtifactID != filters.artifactID {
			return false
		}
		if filters.operatorUserID != 0 && log.OperatorUserID != filters.operatorUserID {
			return false
		}
		if filters.action != "" && log.Action != filters.action {
			return false
		}
		if filters.result != "" && log.Result != filters.result {
			return false
		}
		if filters.fromTimestamp != 0 && log.Timestamp < filters.fromTimestamp {
			return false
		}
		if filters.toTimestamp != 0 && log.Timestamp > filters.toTimestamp {
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
	environmentID       portainer.PlatformEnvironmentID
	applicationID       portainer.PlatformApplicationID
	serviceDefinitionID portainer.PlatformServiceDefinitionID
	releaseID           portainer.PlatformReleaseID
	serviceDeploymentID portainer.PlatformServiceDeploymentID
	artifactID          portainer.PlatformArtifactID
	operatorUserID      portainer.UserID
	action              portainer.PlatformAuditAction
	result              portainer.PlatformAuditResult
	fromTimestamp       int64
	toTimestamp         int64
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
	environmentID, err := optionalQueryID(r, "environmentId")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	applicationID, err := optionalQueryID(r, "applicationId")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	serviceDefinitionID, err := optionalQueryID(r, "serviceDefinitionId")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	artifactID, err := optionalQueryID(r, "artifactId")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	operatorUserID, err := optionalQueryID(r, "operatorUserId")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	fromTimestamp, err := optionalAuditTimestamp(r, "from")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	toTimestamp, err := optionalAuditTimestamp(r, "to")
	if err != nil {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	if toTimestamp != 0 && fromTimestamp != 0 && toTimestamp < fromTimestamp {
		return auditLogFilters{}, httperror.BadRequest(errPlatformInvalidRequest, strconv.ErrSyntax)
	}

	return auditLogFilters{
		projectID:           portainer.PlatformProjectID(projectID),
		environmentID:       portainer.PlatformEnvironmentID(environmentID),
		applicationID:       portainer.PlatformApplicationID(applicationID),
		serviceDefinitionID: portainer.PlatformServiceDefinitionID(serviceDefinitionID),
		releaseID:           portainer.PlatformReleaseID(releaseID),
		serviceDeploymentID: portainer.PlatformServiceDeploymentID(serviceDeploymentID),
		artifactID:          portainer.PlatformArtifactID(artifactID),
		operatorUserID:      portainer.UserID(operatorUserID),
		action:              portainer.PlatformAuditAction(r.URL.Query().Get("action")),
		result:              portainer.PlatformAuditResult(r.URL.Query().Get("result")),
		fromTimestamp:       fromTimestamp,
		toTimestamp:         toTimestamp,
	}, nil
}

func optionalAuditTimestamp(r *http.Request, name string) (int64, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return 0, nil
	}
	timestamp, err := strconv.ParseInt(value, 10, 64)
	if err != nil || timestamp < 0 {
		return 0, strconv.ErrSyntax
	}

	return timestamp, nil
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
	return handler.createPlatformAuditLog(tx, r, audit)
}

// createPlatformAuditLog 统一填充请求元数据并写入审计。业务调用方只能提交摘要，
// 让敏感配置的明文、密文和完整请求体没有机会进入审计存储。
func (handler *Handler) createPlatformAuditLog(tx dataservices.DataStoreTx, r *http.Request, audit *portainer.PlatformAuditLog) error {
	if audit == nil {
		return nil
	}
	if audit.Timestamp == 0 {
		audit.Timestamp = time.Now().Unix()
	}
	handler.fillPlatformAuditRequestFields(tx, r, audit)

	return tx.PlatformAuditLog().Create(audit)
}

func configSetAuditSummary(configSet portainer.PlatformConfigSet) map[string]any {
	sensitiveCount := 0
	for _, entry := range configSet.Entries {
		if entry.Sensitive {
			sensitiveCount++
		}
	}

	return map[string]any{
		"configSetId":         configSet.ID,
		"scopeType":           string(configSet.ScopeType),
		"scopeId":             configSet.ScopeID,
		"name":                configSet.Name,
		"revision":            configSet.Revision,
		"entryCount":          len(configSet.Entries),
		"sensitiveEntryCount": sensitiveCount,
	}
}

func projectPermissionsAuditSummary(project portainer.PlatformProject) map[string]any {
	return map[string]any{
		"projectId":         project.ID,
		"memberPolicyCount": len(project.MemberPolicies),
		"teamPolicyCount":   len(project.TeamPolicies),
	}
}

func (handler *Handler) createConfigSetAuditLog(tx dataservices.DataStoreTx, r *http.Request, action portainer.PlatformAuditAction, result portainer.PlatformAuditResult, configSet portainer.PlatformConfigSet, before map[string]any, sensitiveFields []string) error {
	return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{
		Action:              action,
		Result:              result,
		ProjectID:           configSet.ProjectID,
		EnvironmentID:       configSetEnvironmentID(configSet),
		ServiceDeploymentID: configSetServiceDeploymentID(configSet),
		BeforeSummary:       before,
		AfterSummary:        configSetAuditSummary(configSet),
		SensitiveFields:     append([]string(nil), sensitiveFields...),
	})
}

func configSetEnvironmentID(configSet portainer.PlatformConfigSet) portainer.PlatformEnvironmentID {
	if configSet.ScopeType == portainer.PlatformConfigScopeEnvironment {
		return portainer.PlatformEnvironmentID(configSet.ScopeID)
	}

	return 0
}

func configSetServiceDeploymentID(configSet portainer.PlatformConfigSet) portainer.PlatformServiceDeploymentID {
	if configSet.ScopeType == portainer.PlatformConfigScopeServiceDeployment {
		return portainer.PlatformServiceDeploymentID(configSet.ScopeID)
	}

	return 0
}

func (handler *Handler) createProjectPermissionsAuditLog(tx dataservices.DataStoreTx, r *http.Request, before, after portainer.PlatformProject) error {
	return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{
		Action:        portainer.PlatformAuditActionProjectPermissionsUpdated,
		Result:        portainer.PlatformAuditResultSuccess,
		ProjectID:     after.ID,
		BeforeSummary: projectPermissionsAuditSummary(before),
		AfterSummary:  projectPermissionsAuditSummary(after),
	})
}

// recordPlatformDeniedAudit 不能让审计写入失败改变原本的拒绝结果，因此以 best-effort
// 方式执行；记录的只有授权类别和资源 ID，不包含请求体、配置值或 Endpoint 凭据。
func (handler *Handler) recordPlatformDeniedAudit(r *http.Request, projectID portainer.PlatformProjectID, environmentID portainer.PlatformEnvironmentID, permission string, reason string) {
	if handler == nil || handler.DataStore == nil {
		return
	}

	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{
			Action:        portainer.PlatformAuditActionAccessDenied,
			Result:        portainer.PlatformAuditResultDenied,
			ProjectID:     projectID,
			EnvironmentID: environmentID,
			AfterSummary: map[string]any{
				"permission": permission,
				"reason":     reason,
			},
			FailureReason: reason,
		})
	})
}

func (handler *Handler) createSensitiveConfigAuditLog(tx dataservices.DataStoreTx, r *http.Request, action portainer.PlatformAuditAction, configSet portainer.PlatformConfigSet, fields []string) error {
	if len(fields) == 0 {
		return nil
	}

	return handler.createConfigSetAuditLog(tx, r, action, portainer.PlatformAuditResultSuccess, configSet, nil, fields)
}

func sensitiveConfigEntryKeys(entries []portainer.PlatformConfigEntry) []string {
	keys := make([]string, 0)
	for _, entry := range entries {
		if entry.Sensitive {
			keys = append(keys, entry.Key)
		}
	}
	sort.Strings(keys)

	return keys
}

// sensitiveConfigChanges 只比较加密元数据和引用，不接触明文值；审计记录只使用返回的字段名。
func sensitiveConfigChanges(before, after []portainer.PlatformConfigEntry) map[portainer.PlatformAuditAction][]string {
	beforeByKey := make(map[string]portainer.PlatformConfigEntry)
	afterByKey := make(map[string]portainer.PlatformConfigEntry)
	for _, entry := range before {
		if entry.Sensitive {
			beforeByKey[entry.Key] = entry
		}
	}
	for _, entry := range after {
		if entry.Sensitive {
			afterByKey[entry.Key] = entry
		}
	}

	changes := map[portainer.PlatformAuditAction][]string{}
	for key, previous := range beforeByKey {
		current, found := afterByKey[key]
		if !found {
			changes[portainer.PlatformAuditActionSecretDeleted] = append(changes[portainer.PlatformAuditActionSecretDeleted], key)
			continue
		}
		if previous.ValueType != current.ValueType || previous.Value != current.Value || previous.Hash != current.Hash || previous.CipherText != current.CipherText || previous.EncryptionVersion != current.EncryptionVersion || previous.HasValue != current.HasValue || previous.Required != current.Required {
			changes[portainer.PlatformAuditActionSecretUpdated] = append(changes[portainer.PlatformAuditActionSecretUpdated], key)
		}
	}
	for key := range afterByKey {
		if _, found := beforeByKey[key]; !found {
			changes[portainer.PlatformAuditActionSecretCreated] = append(changes[portainer.PlatformAuditActionSecretCreated], key)
		}
	}
	for action := range changes {
		sort.Strings(changes[action])
	}

	return changes
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

// releaseAuditActionForRelease keeps rollback outcomes distinguishable from normal deployment
// lifecycle events. A manual rollback is a new Release fact, but its audit trail must remain
// searchable as a rollback even when the executor reaches a recovery-failed terminal state.
func releaseAuditActionForRelease(release portainer.PlatformRelease) portainer.PlatformAuditAction {
	if release.TriggerType == portainer.PlatformReleaseTriggerRollback {
		if release.Status == portainer.PlatformReleaseStatusSucceeded {
			return portainer.PlatformAuditActionRollbackSucceeded
		}
		switch release.Status {
		case portainer.PlatformReleaseStatusFailed,
			portainer.PlatformReleaseStatusRecoveryFailed,
			portainer.PlatformReleaseStatusCanceled,
			portainer.PlatformReleaseStatusInterrupted,
			portainer.PlatformReleaseStatusResolved:
			return portainer.PlatformAuditActionRollbackFailed
		}
		return ""
	}

	return releaseAuditActionForStatus(release.Status)
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
