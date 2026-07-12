package platform

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

const artifactCleanupLeaseSeconds int64 = 10 * 60

var errArtifactCleanupBlocked = errors.New("artifact cleanup is blocked")

// artifactOriginalCleanup 只删除平台保留的本地原始制品。它不会删除 Artifact、Release、快照或 registry 镜像，
// 并在事务外执行文件 I/O，以便清理失败不影响任何线上 Release 或历史事实。
func (handler *Handler) artifactOriginalCleanup(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "artifactId")
	if handlerErr != nil {
		return handlerErr
	}
	artifact, handlerErr := handler.requireArtifactPermission(r, portainer.PlatformArtifactID(id), platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if handler.FileService == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Artifact cleanup is unavailable", "ARTIFACT_CLEANUP_UNAVAILABLE", nil)
	}

	leaseID := uuid.NewString()
	now := time.Now().Unix()
	var cleanupArtifact *portainer.PlatformArtifact
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		cleanupArtifact, err = readActiveArtifact(tx, artifact.ID)
		if err != nil {
			return err
		}
		if artifactTaskActive(*cleanupArtifact, now) || !cleanupArtifact.Retained || !cleanupArtifact.Cleanable || !artifactOriginalCanBeCleaned(tx, *cleanupArtifact) {
			return errArtifactCleanupBlocked
		}
		cleanupArtifact.TaskID = leaseID
		cleanupArtifact.TaskLeaseExpiresAt = now + artifactCleanupLeaseSeconds
		cleanupArtifact.Cleanable = false
		touchLifecycle(&cleanupArtifact.PlatformLifecycle, now)
		return tx.PlatformArtifact().Update(cleanupArtifact.ID, cleanupArtifact)
	})
	if err != nil {
		if errors.Is(err, errArtifactCleanupBlocked) {
			handler.recordArtifactCleanupAudit(r, *artifact, portainer.PlatformAuditActionArtifactCleanupBlocked, portainer.PlatformAuditResultFailed, "ARTIFACT_CLEANUP_BLOCKED")
			return writePlatformError(w, http.StatusConflict, errPlatformValidationFailed, "Artifact cleanup is blocked", "ARTIFACT_CLEANUP_BLOCKED", nil)
		}
		return handler.convertError(err)
	}

	path, err := handler.artifactLocalFilePath(*cleanupArtifact)
	if err == nil {
		err = os.Remove(path)
	}
	if err != nil && !os.IsNotExist(err) {
		handler.finishArtifactCleanup(r, *cleanupArtifact, leaseID, false, "ARTIFACT_CLEANUP_FAILED")
		return writePlatformError(w, http.StatusBadRequest, errPlatformValidationFailed, "Artifact cleanup failed", "ARTIFACT_CLEANUP_FAILED", nil)
	}
	if err := handler.finishArtifactCleanup(r, *cleanupArtifact, leaseID, true, ""); err != nil {
		return handler.convertError(err)
	}
	return response.Empty(w)
}

func (handler *Handler) finishArtifactCleanup(r *http.Request, artifact portainer.PlatformArtifact, leaseID string, succeeded bool, reason string) error {
	return handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformArtifact().Read(artifact.ID)
		if err != nil || current.TaskID != leaseID {
			return errors.New("artifact cleanup lease lost")
		}
		current.TaskID = ""
		current.TaskLeaseExpiresAt = 0
		if succeeded {
			current.Retained = false
			current.Cleanable = false
			current.StoragePath = ""
			current.FailureReason = ""
		} else {
			current.Cleanable = true
			current.FailureReason = reason
		}
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformArtifact().Update(current.ID, current); err != nil {
			return err
		}
		action := portainer.PlatformAuditActionArtifactCleaned
		result := portainer.PlatformAuditResultSuccess
		if !succeeded {
			action = portainer.PlatformAuditActionArtifactCleanupFailed
			result = portainer.PlatformAuditResultFailed
		}
		return handler.createArtifactCleanupAudit(tx, r, *current, action, result, reason)
	})
}

func (handler *Handler) recordArtifactCleanupAudit(r *http.Request, artifact portainer.PlatformArtifact, action portainer.PlatformAuditAction, result portainer.PlatformAuditResult, reason string) {
	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		return handler.createArtifactCleanupAudit(tx, r, artifact, action, result, reason)
	})
}

func (handler *Handler) createArtifactCleanupAudit(tx dataservices.DataStoreTx, r *http.Request, artifact portainer.PlatformArtifact, action portainer.PlatformAuditAction, result portainer.PlatformAuditResult, reason string) error {
	return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{
		Action:              action,
		Result:              result,
		ProjectID:           artifact.ProjectID,
		ApplicationID:       artifact.ApplicationID,
		ServiceDefinitionID: artifact.ServiceDefinitionID,
		ArtifactID:          artifact.ID,
		FailureReason:       reason,
		AfterSummary: map[string]any{
			"artifactId":  artifact.ID,
			"retained":    artifact.Retained,
			"cleanable":   artifact.Cleanable,
			"status":      artifact.Status,
			"sha256":      artifact.SHA256,
			"imageTag":    artifact.ImageTag,
			"imageDigest": artifact.ImageDigest,
		},
	})
}
