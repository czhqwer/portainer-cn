package platform

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

const maxCapacityThresholdBytes int64 = 1 << 50

type platformCapacitySummary struct {
	ProjectID              portainer.PlatformProjectID     `json:"ProjectId"`
	EnvironmentID          portainer.PlatformEnvironmentID `json:"EnvironmentId,omitempty"`
	ArtifactCount          int                             `json:"ArtifactCount"`
	ArtifactBytes          int64                           `json:"ArtifactBytes"`
	RetainedArtifactCount  int                             `json:"RetainedArtifactCount"`
	RetainedArtifactBytes  int64                           `json:"RetainedArtifactBytes"`
	CleanableArtifactCount int                             `json:"CleanableArtifactCount"`
	CleanableArtifactBytes int64                           `json:"CleanableArtifactBytes"`
	WarningThresholdBytes  int64                           `json:"WarningThresholdBytes,omitempty"`
	ThresholdStatus        string                          `json:"ThresholdStatus"`
	ObjectStorageCapacity  string                          `json:"ObjectStorageCapacity"`
}

type platformFailureDiagnostic struct {
	Reason string `json:"Reason"`
	Source string `json:"Source"`
	Count  int    `json:"Count"`
}

type platformFailureDiagnosticsResponse struct {
	ProjectID     portainer.PlatformProjectID     `json:"ProjectId"`
	EnvironmentID portainer.PlatformEnvironmentID `json:"EnvironmentId,omitempty"`
	From          int64                           `json:"From,omitempty"`
	To            int64                           `json:"To,omitempty"`
	Items         []platformFailureDiagnostic     `json:"Items"`
}

// projectCapacitySummary 只依据制品元数据和 Release 快照聚合容量，绝不递归扫描宿主机
// 路径。对象存储可用容量在 S3 协议中没有可靠的通用语义，明确返回 not-supported。
func (handler *Handler) projectCapacitySummary(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionView); handlerErr != nil {
		return handlerErr
	}
	environmentID, threshold, handlerErr := capacityQuery(r, portainer.PlatformProjectID(projectID), handler)
	if handlerErr != nil {
		return handlerErr
	}
	artifacts, err := handler.DataStore.PlatformArtifact().ReadAll(func(artifact portainer.PlatformArtifact) bool {
		return artifact.ProjectID == portainer.PlatformProjectID(projectID) && isActive(artifact.PlatformLifecycle)
	})
	if err != nil {
		return handler.convertError(err)
	}

	if environmentID != 0 {
		artifacts, err = handler.artifactsReferencedByEnvironment(portainer.PlatformProjectID(projectID), environmentID, artifacts)
		if err != nil {
			return handler.convertError(err)
		}
	}

	result := platformCapacitySummary{ProjectID: portainer.PlatformProjectID(projectID), EnvironmentID: environmentID, WarningThresholdBytes: threshold, ThresholdStatus: "not-configured", ObjectStorageCapacity: "not-supported"}
	for _, artifact := range artifacts {
		result.ArtifactCount++
		result.ArtifactBytes += artifact.Size
		if artifact.Retained {
			result.RetainedArtifactCount++
			result.RetainedArtifactBytes += artifact.Size
		}
		if artifact.Cleanable {
			result.CleanableArtifactCount++
			result.CleanableArtifactBytes += artifact.Size
		}
	}
	if threshold > 0 {
		result.ThresholdStatus = "ok"
		if result.ArtifactBytes >= threshold {
			result.ThresholdStatus = "warning"
		}
	}
	return response.JSON(w, result)
}

// projectFailureDiagnostics 只聚合阶段化 Release 和结构化审计中的稳定 reason，避免把
// Docker、数据库、网关或对象存储的原始错误及内部拓扑重新暴露给项目用户。
func (handler *Handler) projectFailureDiagnostics(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}
	environmentID, from, to, handlerErr := diagnosticsQuery(r, portainer.PlatformProjectID(projectID), handler)
	if handlerErr != nil {
		return handlerErr
	}
	counts := map[string]int{}
	releases, err := handler.DataStore.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		return release.ProjectID == portainer.PlatformProjectID(projectID) && (environmentID == 0 || release.EnvironmentID == environmentID) && release.FailureReason != "" && (from == 0 || release.FinishedAt >= from) && (to == 0 || release.FinishedAt <= to)
	})
	if err != nil {
		return handler.convertError(err)
	}
	for _, release := range releases {
		counts["release:"+release.FailureReason]++
	}
	audits, err := handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool {
		return audit.ProjectID == portainer.PlatformProjectID(projectID) && (environmentID == 0 || audit.EnvironmentID == environmentID) && audit.FailureReason != "" && (from == 0 || audit.Timestamp >= from) && (to == 0 || audit.Timestamp <= to)
	})
	if err != nil {
		return handler.convertError(err)
	}
	for _, audit := range audits {
		counts["audit:"+audit.FailureReason]++
	}
	items := make([]platformFailureDiagnostic, 0, len(counts))
	for key, count := range counts {
		items = append(items, platformFailureDiagnostic{Source: key[:len(key)-len(reasonFromDiagnosticKey(key))-1], Reason: reasonFromDiagnosticKey(key), Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		if items[i].Source != items[j].Source {
			return items[i].Source < items[j].Source
		}
		return items[i].Reason < items[j].Reason
	})
	return response.JSON(w, platformFailureDiagnosticsResponse{ProjectID: portainer.PlatformProjectID(projectID), EnvironmentID: environmentID, From: from, To: to, Items: items})
}

func (handler *Handler) artifactsReferencedByEnvironment(projectID portainer.PlatformProjectID, environmentID portainer.PlatformEnvironmentID, artifacts []portainer.PlatformArtifact) ([]portainer.PlatformArtifact, error) {
	releases, err := handler.DataStore.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		return release.ProjectID == projectID && release.EnvironmentID == environmentID && release.ArtifactID > 0
	})
	if err != nil {
		return nil, err
	}
	artifactIDs := make(map[portainer.PlatformArtifactID]struct{}, len(releases))
	for _, release := range releases {
		artifactIDs[release.ArtifactID] = struct{}{}
	}
	result := make([]portainer.PlatformArtifact, 0, len(artifactIDs))
	for _, artifact := range artifacts {
		if _, found := artifactIDs[artifact.ID]; found {
			result = append(result, artifact)
		}
	}
	return result, nil
}

func capacityQuery(r *http.Request, projectID portainer.PlatformProjectID, handler *Handler) (portainer.PlatformEnvironmentID, int64, *httperror.HandlerError) {
	environmentID, handlerErr := projectEnvironmentQuery(r, projectID, handler)
	if handlerErr != nil {
		return 0, 0, handlerErr
	}
	threshold, err := parseOptionalPositiveInt64(r.URL.Query().Get("warningThresholdBytes"))
	if err != nil || threshold > maxCapacityThresholdBytes {
		return 0, 0, validationFailed(errors.New("warningThresholdBytes is invalid"))
	}
	return environmentID, threshold, nil
}

func diagnosticsQuery(r *http.Request, projectID portainer.PlatformProjectID, handler *Handler) (portainer.PlatformEnvironmentID, int64, int64, *httperror.HandlerError) {
	environmentID, handlerErr := projectEnvironmentQuery(r, projectID, handler)
	if handlerErr != nil {
		return 0, 0, 0, handlerErr
	}
	from, err := parseOptionalPositiveInt64(r.URL.Query().Get("from"))
	if err != nil {
		return 0, 0, 0, validationFailed(errors.New("from is invalid"))
	}
	to, err := parseOptionalPositiveInt64(r.URL.Query().Get("to"))
	if err != nil || (from > 0 && to > 0 && from > to) {
		return 0, 0, 0, validationFailed(errors.New("to is invalid"))
	}
	return environmentID, from, to, nil
}

func projectEnvironmentQuery(r *http.Request, projectID portainer.PlatformProjectID, handler *Handler) (portainer.PlatformEnvironmentID, *httperror.HandlerError) {
	value := r.URL.Query().Get("environmentId")
	if value == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, validationFailed(errors.New("environmentId is invalid"))
	}
	environment, err := handler.DataStore.PlatformEnvironment().Read(portainer.PlatformEnvironmentID(id))
	if err != nil || environment.ProjectID != projectID {
		return 0, platformAccessDenied()
	}
	return environment.ID, nil
}

func parseOptionalPositiveInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid integer")
	}
	return parsed, nil
}

func reasonFromDiagnosticKey(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[i+1:]
		}
	}
	return key
}
