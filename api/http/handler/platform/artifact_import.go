package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

const artifactImportTimeout = 10 * time.Minute

type importArtifactPayload struct {
	EndpointID portainer.EndpointID `json:"EndpointId"`
}

func (payload *importArtifactPayload) Validate(_ *http.Request) error {
	if payload.EndpointID <= 0 {
		return errors.New("EndpointId is required")
	}
	return nil
}

// artifactArchiveImport 先持久化短租约，再在事务外做归档读取和 Docker 导入；任何失败只影响该 Artifact，绝不修改当前 Release。
func (handler *Handler) artifactArchiveImport(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "artifactId")
	if handlerErr != nil {
		return handlerErr
	}
	artifact, handlerErr := handler.requireArtifactPermission(r, portainer.PlatformArtifactID(id), platformPermissionArtifactRelease)
	if handlerErr != nil {
		return handlerErr
	}
	var payload importArtifactPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if handlerErr := handler.requireEndpointAccess(r, payload.EndpointID); handlerErr != nil {
		return handlerErr
	}
	if handler.ArchiveImageImporter == nil || handler.FileService == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Archive import is unavailable", "ARCHIVE_IMPORT_UNAVAILABLE", nil)
	}

	leaseID := uuid.NewString()
	now := time.Now().Unix()
	var importArtifact *portainer.PlatformArtifact
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		importArtifact, err = readActiveArtifact(tx, artifact.ID)
		if err != nil {
			return err
		}
		if importArtifact.Type != portainer.PlatformArtifactTypeDockerTar && importArtifact.Type != portainer.PlatformArtifactTypeOCIArchive {
			return validationFailedError("Artifact type cannot be imported")
		}
		if importArtifact.StorageProvider != portainer.PlatformStorageProviderLocal || importArtifact.StoragePath == "" {
			return validationFailedError("Artifact source is unavailable")
		}
		if artifactTaskActive(*importArtifact, now) {
			return conflictError("Artifact task is already running")
		}
		importArtifact.Status = portainer.PlatformArtifactStatusImporting
		importArtifact.TaskID = leaseID
		importArtifact.TaskLeaseExpiresAt = now + int64(artifactImportTimeout/time.Second)
		importArtifact.FailureReason = ""
		resetArtifactTaskEvents(importArtifact, "prepare", now)
		touchLifecycle(&importArtifact.PlatformLifecycle, now)
		return tx.PlatformArtifact().Update(importArtifact.ID, importArtifact)
	})
	if err != nil {
		return handler.convertError(err)
	}

	filePath, err := handler.artifactLocalFilePath(*importArtifact)
	if err != nil {
		return handler.finishArtifactImportFailure(w, r, *importArtifact, leaseID, payload.EndpointID, "ARCHIVE_SOURCE_UNAVAILABLE")
	}
	file, err := os.Open(filePath)
	if err != nil {
		return handler.finishArtifactImportFailure(w, r, *importArtifact, leaseID, payload.EndpointID, "ARCHIVE_SOURCE_UNAVAILABLE")
	}
	format := platformservice.ArchiveFormatDocker
	if importArtifact.Type == portainer.PlatformArtifactTypeOCIArchive {
		format = platformservice.ArchiveFormatOCI
	}
	handler.recordArtifactTaskEvent(importArtifact.ID, leaseID, "prepare", artifactTaskEventSucceeded, "")
	handler.recordArtifactTaskEvent(importArtifact.ID, leaseID, "validate-archive", artifactTaskEventRunning, "")
	metadata, validationErr := platformservice.ValidateImageArchive(file, format)
	_ = file.Close()
	if validationErr != nil {
		return handler.finishArtifactImportFailure(w, r, *importArtifact, leaseID, payload.EndpointID, archiveValidationReason(validationErr))
	}
	handler.recordArtifactTaskEvent(importArtifact.ID, leaseID, "validate-archive", artifactTaskEventSucceeded, "")
	file, err = os.Open(filePath)
	if err != nil {
		return handler.finishArtifactImportFailure(w, r, *importArtifact, leaseID, payload.EndpointID, "ARCHIVE_SOURCE_UNAVAILABLE")
	}
	defer file.Close()
	candidateRef := fmt.Sprintf("portainer-platform-import/%d:%s", importArtifact.ID, leaseID[:12])
	ctx, cancel := context.WithTimeout(r.Context(), artifactImportTimeout)
	defer cancel()
	handler.recordArtifactTaskEvent(importArtifact.ID, leaseID, "import-image", artifactTaskEventRunning, "")
	result, err := handler.ArchiveImageImporter.Import(ctx, platformservice.ArchiveImportRequest{EndpointID: int(payload.EndpointID), Archive: file, SourceRef: metadata.SourceRef, CandidateRef: candidateRef, Architecture: metadata.Architecture})
	if err != nil {
		_ = handler.ArchiveImageImporter.Cleanup(context.Background(), int(payload.EndpointID), candidateRef)
		return handler.finishArtifactImportFailure(w, r, *importArtifact, leaseID, payload.EndpointID, "IMAGE_IMPORT_FAILED")
	}
	handler.recordArtifactTaskEvent(importArtifact.ID, leaseID, "import-image", artifactTaskEventSucceeded, "")
	if result.CandidateRef == "" || result.ImageID == "" {
		_ = handler.ArchiveImageImporter.Cleanup(context.Background(), int(payload.EndpointID), candidateRef)
		return handler.finishArtifactImportFailure(w, r, *importArtifact, leaseID, payload.EndpointID, "IMAGE_IMPORT_FAILED")
	}
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformArtifact().Read(importArtifact.ID)
		if err != nil || current.TaskID != leaseID {
			return errors.New("artifact import lease lost")
		}
		current.Status = portainer.PlatformArtifactStatusBuilt
		current.CandidateImageRef = result.CandidateRef
		current.CandidateImageID = result.ImageID
		current.TaskID = ""
		current.TaskLeaseExpiresAt = 0
		current.FailureReason = ""
		appendArtifactTaskEvent(current, "complete", artifactTaskEventSucceeded, "", time.Now().Unix())
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformArtifact().Update(current.ID, current); err != nil {
			return err
		}
		return handler.createArtifactImportAudit(tx, r, *current, payload.EndpointID, portainer.PlatformAuditResultSuccess, "")
	})
	if err != nil {
		_ = handler.ArchiveImageImporter.Cleanup(context.Background(), int(payload.EndpointID), result.CandidateRef)
		return handler.finishArtifactImportFailure(w, r, *importArtifact, leaseID, payload.EndpointID, "IMAGE_IMPORT_FAILED")
	}
	updated, _ := handler.DataStore.PlatformArtifact().Read(importArtifact.ID)
	return response.JSON(w, artifactResponse(*updated))
}

func (handler *Handler) artifactLocalFilePath(artifact portainer.PlatformArtifact) (string, error) {
	if artifact.StoragePath == "" || filepath.IsAbs(artifact.StoragePath) {
		return "", errors.New("artifact storage path is invalid")
	}
	root := filepath.Join(handler.FileService.GetDatastorePath(), "platform-artifacts")
	full := filepath.Join(root, filepath.FromSlash(artifact.StoragePath))
	relative, err := filepath.Rel(root, full)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || len(relative) >= 3 && relative[:3] == ".."+string(os.PathSeparator) {
		return "", errors.New("artifact storage path is invalid")
	}
	return full, nil
}
func archiveValidationReason(err error) string {
	if err == nil {
		return ""
	}
	if err.Error() == "archive architecture is unsupported" {
		return "ARCHIVE_ARCHITECTURE_UNSUPPORTED"
	}
	if err.Error() == "archive manifest is invalid" {
		return "ARCHIVE_MANIFEST_INVALID"
	}
	return "ARCHIVE_UNSAFE"
}
func (handler *Handler) finishArtifactImportFailure(w http.ResponseWriter, r *http.Request, artifact portainer.PlatformArtifact, leaseID string, endpointID portainer.EndpointID, reason string) *httperror.HandlerError {
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
		return handler.createArtifactImportAudit(tx, r, *current, endpointID, portainer.PlatformAuditResultFailed, reason)
	})
	return writePlatformError(w, http.StatusBadRequest, errPlatformValidationFailed, "Archive import failed", reason, nil)
}
func (handler *Handler) createArtifactImportAudit(tx dataservices.DataStoreTx, r *http.Request, artifact portainer.PlatformArtifact, endpointID portainer.EndpointID, result portainer.PlatformAuditResult, reason string) error {
	action := portainer.PlatformAuditActionArtifactImported
	if result != portainer.PlatformAuditResultSuccess {
		action = portainer.PlatformAuditActionArtifactImportFailed
	}
	return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: action, Result: result, ProjectID: artifact.ProjectID, ApplicationID: artifact.ApplicationID, ServiceDefinitionID: artifact.ServiceDefinitionID, ArtifactID: artifact.ID, FailureReason: reason, AfterSummary: map[string]any{"artifactId": artifact.ID, "endpointId": endpointID, "type": artifact.Type, "status": artifact.Status, "candidateImageId": artifact.CandidateImageID}})
}
