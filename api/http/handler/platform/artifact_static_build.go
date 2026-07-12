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

const artifactStaticBuildTimeout = 10 * time.Minute

type buildStaticArtifactPayload struct {
	EndpointID   portainer.EndpointID            `json:"EndpointId"`
	Mode         platformservice.DistMode        `json:"Mode"`
	CachePolicy  platformservice.DistCachePolicy `json:"CachePolicy,omitempty"`
	NotFoundPage string                          `json:"NotFoundPage,omitempty"`
}

func (payload *buildStaticArtifactPayload) Validate(_ *http.Request) error {
	if payload.EndpointID <= 0 {
		return errors.New("EndpointId is required")
	}
	return platformservice.ValidateDistBuildOptions(platformservice.DistBuildOptions{
		Mode:         payload.Mode,
		CachePolicy:  payload.CachePolicy,
		NotFoundPage: payload.NotFoundPage,
	})
}

// artifactStaticBuild 先写入 Artifact 短租约，再在事务外创建受控静态镜像。ZIP、Nginx 配置和 Docker 输出
// 都不会进入审计或错误响应；失败只更新本次 Artifact，绝不影响正在运行的 Release。
func (handler *Handler) artifactStaticBuild(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "artifactId")
	if handlerErr != nil {
		return handlerErr
	}
	artifact, handlerErr := handler.requireArtifactPermission(r, portainer.PlatformArtifactID(id), platformPermissionArtifactRelease)
	if handlerErr != nil {
		return handlerErr
	}
	var payload buildStaticArtifactPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if handlerErr := handler.requireEndpointAccess(r, payload.EndpointID); handlerErr != nil {
		return handlerErr
	}
	if handler.StaticImageBuilder == nil || handler.FileService == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Static artifact build is unavailable", "STATIC_BUILD_UNAVAILABLE", nil)
	}

	options := platformservice.DistBuildOptions{
		Mode:         payload.Mode,
		CachePolicy:  payload.CachePolicy,
		NotFoundPage: payload.NotFoundPage,
	}
	leaseID := uuid.NewString()
	now := time.Now().Unix()
	var buildArtifact *portainer.PlatformArtifact
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		buildArtifact, err = readActiveArtifact(tx, artifact.ID)
		if err != nil {
			return err
		}
		if buildArtifact.Type != portainer.PlatformArtifactTypeFrontendDist || buildArtifact.StorageProvider != portainer.PlatformStorageProviderLocal || buildArtifact.StoragePath == "" {
			return validationFailedError("Artifact cannot use static template")
		}
		if buildArtifact.Status == portainer.PlatformArtifactStatusBuilding && buildArtifact.TaskLeaseExpiresAt > now {
			return conflictError("Artifact build is already running")
		}
		buildArtifact.Status = portainer.PlatformArtifactStatusBuilding
		buildArtifact.TaskID = leaseID
		buildArtifact.TaskLeaseExpiresAt = now + int64(artifactStaticBuildTimeout/time.Second)
		buildArtifact.BuildTemplate = platformservice.StaticBuildTemplateName(options)
		buildArtifact.FailureReason = ""
		touchLifecycle(&buildArtifact.PlatformLifecycle, now)
		return tx.PlatformArtifact().Update(buildArtifact.ID, buildArtifact)
	})
	if err != nil {
		return handler.convertError(err)
	}

	filePath, err := handler.artifactLocalFilePath(*buildArtifact)
	if err != nil {
		return handler.finishArtifactStaticBuild(r, *buildArtifact, leaseID, payload.EndpointID, options, "CONTROLLED_BUILD_FAILED")
	}
	temporaryDirectory := filepath.Join(handler.FileService.GetDatastorePath(), "platform-artifacts", ".tmp")
	contextFile, err := platformservice.NewStaticDistBuildContextFile(filePath, temporaryDirectory, options)
	if err != nil {
		return handler.finishArtifactStaticBuild(r, *buildArtifact, leaseID, payload.EndpointID, options, platformservice.DistBuildFailureReason(err))
	}
	defer func() {
		_ = contextFile.Close()
		_ = os.Remove(contextFile.Name())
	}()

	candidateRef := fmt.Sprintf("portainer-platform-static/%d:%s", buildArtifact.ID, leaseID[:12])
	ctx, cancel := context.WithTimeout(r.Context(), artifactStaticBuildTimeout)
	defer cancel()
	result, err := handler.StaticImageBuilder.Build(ctx, platformservice.StaticImageBuildRequest{
		EndpointID:   int(payload.EndpointID),
		Context:      contextFile,
		CandidateRef: candidateRef,
	})
	if err != nil || result.CandidateRef == "" || result.ImageID == "" {
		_ = handler.StaticImageBuilder.Cleanup(context.Background(), int(payload.EndpointID), candidateRef)
		return handler.finishArtifactStaticBuild(r, *buildArtifact, leaseID, payload.EndpointID, options, "CONTROLLED_BUILD_FAILED")
	}

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformArtifact().Read(buildArtifact.ID)
		if err != nil || current.TaskID != leaseID {
			return errors.New("static artifact build lease lost")
		}
		current.Status = portainer.PlatformArtifactStatusBuilt
		current.CandidateImageRef = result.CandidateRef
		current.CandidateImageID = result.ImageID
		current.TaskLeaseExpiresAt = 0
		current.FailureReason = ""
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformArtifact().Update(current.ID, current); err != nil {
			return err
		}
		return handler.createArtifactStaticBuildAudit(tx, r, *current, payload.EndpointID, options, portainer.PlatformAuditResultSuccess, "")
	})
	if err != nil {
		_ = handler.StaticImageBuilder.Cleanup(context.Background(), int(payload.EndpointID), result.CandidateRef)
		return handler.finishArtifactStaticBuild(r, *buildArtifact, leaseID, payload.EndpointID, options, "CONTROLLED_BUILD_FAILED")
	}
	updated, _ := handler.DataStore.PlatformArtifact().Read(buildArtifact.ID)
	return response.JSON(w, artifactResponse(*updated))
}

func (handler *Handler) finishArtifactStaticBuild(r *http.Request, artifact portainer.PlatformArtifact, leaseID string, endpointID portainer.EndpointID, options platformservice.DistBuildOptions, reason string) *httperror.HandlerError {
	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformArtifact().Read(artifact.ID)
		if err != nil || current.TaskID != leaseID {
			return err
		}
		current.Status = portainer.PlatformArtifactStatusFailed
		current.TaskLeaseExpiresAt = 0
		current.FailureReason = reason
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformArtifact().Update(current.ID, current); err != nil {
			return err
		}
		return handler.createArtifactStaticBuildAudit(tx, r, *current, endpointID, options, portainer.PlatformAuditResultFailed, reason)
	})
	return validationFailedError("Static artifact build failed")
}

func (handler *Handler) createArtifactStaticBuildAudit(tx dataservices.DataStoreTx, r *http.Request, artifact portainer.PlatformArtifact, endpointID portainer.EndpointID, options platformservice.DistBuildOptions, result portainer.PlatformAuditResult, reason string) error {
	action := portainer.PlatformAuditActionArtifactStaticBuilt
	if result != portainer.PlatformAuditResultSuccess {
		action = portainer.PlatformAuditActionArtifactStaticBuildFailed
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
			"artifactId":        artifact.ID,
			"endpointId":        endpointID,
			"type":              artifact.Type,
			"status":            artifact.Status,
			"buildTemplate":     artifact.BuildTemplate,
			"candidateImageRef": artifact.CandidateImageRef,
			"candidateImageId":  artifact.CandidateImageID,
			"mode":              options.Mode,
			"cachePolicy":       options.CachePolicy,
			"custom404":         options.NotFoundPage != "",
		},
	})
}
