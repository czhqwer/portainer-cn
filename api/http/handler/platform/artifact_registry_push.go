package platform

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	registryaccess "github.com/portainer/portainer/api/internal/registryutils/access"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

const artifactRegistryPushTimeout = 10 * time.Minute

var errArtifactRegistryTagConflict = errors.New("artifact registry tag conflict")

type pushArtifactPayload struct {
	EndpointID portainer.EndpointID `json:"EndpointId"`
	RegistryID portainer.RegistryID `json:"RegistryId"`
}

func (payload *pushArtifactPayload) Validate(_ *http.Request) error {
	if payload.EndpointID <= 0 || payload.RegistryID <= 0 {
		return errors.New("EndpointId and RegistryId are required")
	}
	return nil
}

// artifactRegistryPush 把已构建候选镜像推送为平台唯一 tag。事务只持久化租约和最终事实，
// Docker/registry 网络 I/O 始终在事务外执行，任何失败都不会改变现有 Release。
func (handler *Handler) artifactRegistryPush(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "artifactId")
	if handlerErr != nil {
		return handlerErr
	}
	artifact, handlerErr := handler.requireArtifactPermission(r, portainer.PlatformArtifactID(id), platformPermissionArtifactRelease)
	if handlerErr != nil {
		return handlerErr
	}
	var payload pushArtifactPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if handlerErr := handler.requireEndpointAccess(r, payload.EndpointID); handlerErr != nil {
		return handlerErr
	}
	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}
	registry, err := registryaccess.GetAccessibleRegistry(handler.DataStore, nil, userID, payload.EndpointID, payload.RegistryID)
	if err != nil {
		handler.recordPlatformDeniedAudit(r, artifact.ProjectID, 0, "registry-push", "Portainer registry permission is required")
		return platformAccessDenied()
	}
	if handler.RegistryImagePusher == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Registry push is unavailable", "REGISTRY_PUSH_UNAVAILABLE", nil)
	}

	leaseID := uuid.NewString()
	now := time.Now().Unix()
	var pushArtifact *portainer.PlatformArtifact
	targetRef := ""
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		pushArtifact, err = readActiveArtifact(tx, artifact.ID)
		if err != nil {
			return err
		}
		if artifactTaskActive(*pushArtifact, now) {
			return conflictError("Artifact task is already running")
		}
		if pushArtifact.Status != portainer.PlatformArtifactStatusBuilt || pushArtifact.CandidateImageRef == "" || pushArtifact.CandidateImageID == "" {
			return validationFailedError("Artifact has no candidate image to push")
		}
		project, err := readActiveProject(tx, pushArtifact.ProjectID)
		if err != nil {
			return err
		}
		service, err := readActiveServiceDefinition(tx, pushArtifact.ServiceDefinitionID)
		if err != nil || service.ProjectID != project.ID {
			return validationFailedError("Artifact service is unavailable")
		}
		targetRef, err = platformservice.PlatformRegistryTag(registry.URL, project.Slug, service.Slug, pushArtifact.Version, pushArtifact.ID)
		if err != nil {
			return validationFailedError("Registry target is invalid")
		}
		existing, err := tx.PlatformArtifact().ReadAll(func(existing portainer.PlatformArtifact) bool {
			return existing.ID != pushArtifact.ID && existing.ImageTag == targetRef
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return errArtifactRegistryTagConflict
		}
		pushArtifact.Status = portainer.PlatformArtifactStatusPushing
		pushArtifact.TaskID = leaseID
		pushArtifact.TaskLeaseExpiresAt = now + int64(artifactRegistryPushTimeout/time.Second)
		pushArtifact.FailureReason = ""
		resetArtifactTaskEvents(pushArtifact, "prepare", now)
		touchLifecycle(&pushArtifact.PlatformLifecycle, now)
		return tx.PlatformArtifact().Update(pushArtifact.ID, pushArtifact)
	})
	if err != nil {
		if errors.Is(err, errArtifactRegistryTagConflict) {
			handler.recordArtifactRegistryPushFailure(r, *artifact, payload, targetRef, "REGISTRY_TAG_CONFLICT")
			return writePlatformError(w, http.StatusConflict, errPlatformValidationFailed, "Registry tag is already owned", "REGISTRY_TAG_CONFLICT", nil)
		}
		return handler.convertError(err)
	}

	ctx, cancel := context.WithTimeout(r.Context(), artifactRegistryPushTimeout)
	defer cancel()
	handler.recordArtifactTaskEvent(pushArtifact.ID, leaseID, "prepare", artifactTaskEventSucceeded, "")
	handler.recordArtifactTaskEvent(pushArtifact.ID, leaseID, "push-image", artifactTaskEventRunning, "")
	result, err := handler.RegistryImagePusher.Push(ctx, platformservice.RegistryImagePushRequest{
		EndpointID:   int(payload.EndpointID),
		RegistryID:   payload.RegistryID,
		CandidateRef: pushArtifact.CandidateImageRef,
		TargetRef:    targetRef,
	})
	if err != nil || result.ImageRef == "" || result.ImageTag == "" || result.ImageDigest == "" {
		reason := platformservice.RegistryPushFailureReason(err)
		if err == nil {
			reason = "REGISTRY_DIGEST_UNAVAILABLE"
		}
		handler.finishArtifactRegistryPush(r, *pushArtifact, leaseID, payload, targetRef, reason)
		return writePlatformError(w, http.StatusBadRequest, errPlatformValidationFailed, "Registry push failed", reason, nil)
	}
	handler.recordArtifactTaskEvent(pushArtifact.ID, leaseID, "push-image", artifactTaskEventSucceeded, "")

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformArtifact().Read(pushArtifact.ID)
		if err != nil || current.TaskID != leaseID {
			return errors.New("artifact push lease lost")
		}
		current.Status = portainer.PlatformArtifactStatusReady
		current.ImageRef = result.ImageRef
		current.ImageTag = result.ImageTag
		current.ImageDigest = result.ImageDigest
		current.RegistryID = payload.RegistryID
		current.Traceability = portainer.PlatformTraceabilityStrong
		current.CandidateImageRef = ""
		current.CandidateImageID = ""
		current.TaskID = ""
		current.TaskLeaseExpiresAt = 0
		current.FailureReason = ""
		appendArtifactTaskEvent(current, "complete", artifactTaskEventSucceeded, "", time.Now().Unix())
		current.Cleanable = artifactOriginalCanBeCleaned(tx, *current)
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformArtifact().Update(current.ID, current); err != nil {
			return err
		}
		return handler.createArtifactRegistryPushAudit(tx, r, *current, payload, targetRef, portainer.PlatformAuditResultSuccess, "")
	})
	if err != nil {
		handler.finishArtifactRegistryPush(r, *pushArtifact, leaseID, payload, targetRef, "REGISTRY_PUSH_FAILED")
		return writePlatformError(w, http.StatusBadRequest, errPlatformValidationFailed, "Registry push failed", "REGISTRY_PUSH_FAILED", nil)
	}
	updated, _ := handler.DataStore.PlatformArtifact().Read(pushArtifact.ID)
	return response.JSON(w, artifactResponse(*updated))
}

func (handler *Handler) finishArtifactRegistryPush(r *http.Request, artifact portainer.PlatformArtifact, leaseID string, payload pushArtifactPayload, targetRef, reason string) {
	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformArtifact().Read(artifact.ID)
		if err != nil || current.TaskID != leaseID {
			return err
		}
		current.Status = portainer.PlatformArtifactStatusFailed
		current.TaskID = ""
		current.TaskLeaseExpiresAt = 0
		current.FailureReason = reason
		appendArtifactTaskEvent(current, "complete", artifactTaskEventFailed, reason, time.Now().Unix())
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformArtifact().Update(current.ID, current); err != nil {
			return err
		}
		return handler.createArtifactRegistryPushAudit(tx, r, *current, payload, targetRef, portainer.PlatformAuditResultFailed, reason)
	})
}

func (handler *Handler) recordArtifactRegistryPushFailure(r *http.Request, artifact portainer.PlatformArtifact, payload pushArtifactPayload, targetRef, reason string) {
	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		return handler.createArtifactRegistryPushAudit(tx, r, artifact, payload, targetRef, portainer.PlatformAuditResultFailed, reason)
	})
}

func (handler *Handler) createArtifactRegistryPushAudit(tx dataservices.DataStoreTx, r *http.Request, artifact portainer.PlatformArtifact, payload pushArtifactPayload, targetRef string, result portainer.PlatformAuditResult, reason string) error {
	action := portainer.PlatformAuditActionArtifactPushed
	if result != portainer.PlatformAuditResultSuccess {
		action = portainer.PlatformAuditActionArtifactPushFailed
	}
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
			"endpointId":  payload.EndpointID,
			"registryId":  payload.RegistryID,
			"status":      artifact.Status,
			"imageTag":    targetRef,
			"imageDigest": artifact.ImageDigest,
			"sha256":      artifact.SHA256,
		},
	})
}

func artifactOriginalCanBeCleaned(tx dataservices.DataStoreTx, artifact portainer.PlatformArtifact) bool {
	if !artifact.Retained || artifact.StorageProvider != portainer.PlatformStorageProviderLocal || artifact.StoragePath == "" {
		return false
	}
	releases, err := tx.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		return release.ArtifactID == artifact.ID || release.ArtifactSnapshot.ArtifactID == artifact.ID
	})
	return err == nil && len(releases) == 0
}
