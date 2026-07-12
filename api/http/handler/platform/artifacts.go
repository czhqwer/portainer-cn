package platform

import (
	"net/http"
	"strconv"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type artifactValidationResponse struct {
	Valid      bool                         `json:"Valid"`
	Reason     string                       `json:"Reason,omitempty"`
	ArtifactID portainer.PlatformArtifactID `json:"ArtifactId"`
	ImageRef   string                       `json:"ImageRef,omitempty"`
}

func (handler *Handler) artifactList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	filters, handlerErr := artifactListFilters(r)
	if handlerErr != nil {
		return handlerErr
	}
	visibleProjectIDs, handlerErr := handler.visibleProjectIDs(r)
	if handlerErr != nil {
		return handlerErr
	}
	if filters.projectID != 0 {
		if _, handlerErr := handler.requireProjectPermission(r, filters.projectID, platformPermissionView); handlerErr != nil {
			return handlerErr
		}
	}

	artifacts, err := handler.DataStore.PlatformArtifact().ReadAll(func(artifact portainer.PlatformArtifact) bool {
		if visibleProjectIDs != nil && !visibleProjectIDs[artifact.ProjectID] {
			return false
		}
		if !filters.includeArchived && !isActive(artifact.PlatformLifecycle) {
			return false
		}
		if filters.projectID != 0 && artifact.ProjectID != filters.projectID {
			return false
		}
		if filters.applicationID != 0 && artifact.ApplicationID != filters.applicationID {
			return false
		}
		if filters.serviceDefinitionID != 0 && artifact.ServiceDefinitionID != filters.serviceDefinitionID {
			return false
		}

		return true
	})
	if err != nil {
		return handler.convertError(err)
	}
	for i := range artifacts {
		artifacts[i] = artifactResponse(artifacts[i])
	}

	return response.JSON(w, artifacts)
}

func (handler *Handler) artifactInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "artifactId")
	if handlerErr != nil {
		return handlerErr
	}

	artifact, handlerErr := handler.requireArtifactPermission(r, portainer.PlatformArtifactID(id), platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}

	return response.JSON(w, artifactResponse(*artifact))
}

// artifactResponse 只向普通制品 API 返回可追溯元数据，不暴露内部对象键。
// 后续读取原始文件只能由受控服务基于 Artifact ID 完成，避免前端或日志把存储布局当作公开接口。
func artifactResponse(artifact portainer.PlatformArtifact) portainer.PlatformArtifact {
	artifact.StoragePath = ""
	artifact.SourcePath = ""
	return artifact
}

func (handler *Handler) artifactImageReferenceCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	var payload createImageReferenceArtifactPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, payload.ProjectID, platformPermissionArtifactRelease); handlerErr != nil {
		return handlerErr
	}

	now := time.Now().Unix()
	artifact := &portainer.PlatformArtifact{
		ProjectID:           payload.ProjectID,
		ApplicationID:       payload.ApplicationID,
		ServiceDefinitionID: payload.ServiceDefinitionID,
		Name:                payload.Name,
		Version:             payload.Version,
		Type:                portainer.PlatformArtifactTypeImage,
		SourceType:          portainer.PlatformArtifactSourceImageReference,
		ImageRef:            payload.ImageRef,
		ImageDigest:         payload.ImageDigest,
		Traceability:        payload.Traceability,
		RegistryID:          payload.RegistryID,
		Retained:            payload.Retained,
		Cleanable:           payload.Cleanable,
		PlatformLifecycle:   newLifecycle(now),
	}
	if artifact.Traceability == "" {
		artifact.Traceability = portainer.PlatformTraceabilityWeak
	}

	// image-reference 只登记已有镜像坐标，方便后续发布引用；
	// 这里不拉取镜像、不解析 digest，也不验证 registry 凭据。
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if _, err := readActiveProject(tx, artifact.ProjectID); err != nil {
			return err
		}

		if artifact.ApplicationID != 0 {
			application, err := readActiveApplication(tx, artifact.ApplicationID)
			if err != nil {
				return err
			}
			if application.ProjectID != artifact.ProjectID {
				return validationFailedError("Application and artifact belong to different projects")
			}
		}

		if artifact.ServiceDefinitionID != 0 {
			service, err := readActiveServiceDefinition(tx, artifact.ServiceDefinitionID)
			if err != nil {
				return err
			}
			if service.ProjectID != artifact.ProjectID {
				return validationFailedError("Service and artifact belong to different projects")
			}
			if artifact.ApplicationID == 0 {
				artifact.ApplicationID = service.ApplicationID
			} else if artifact.ApplicationID != service.ApplicationID {
				return validationFailedError("Service and artifact application mismatch")
			}
		}

		existingArtifacts, err := tx.PlatformArtifact().ReadAll(func(existing portainer.PlatformArtifact) bool {
			return isActive(existing.PlatformLifecycle) &&
				existing.ProjectID == artifact.ProjectID &&
				existing.ServiceDefinitionID == artifact.ServiceDefinitionID &&
				existing.Version == artifact.Version &&
				existing.ImageRef == artifact.ImageRef
		})
		if err != nil {
			return err
		}
		if len(existingArtifacts) > 0 {
			return duplicateError("Artifact image reference already exists")
		}

		return tx.PlatformArtifact().Create(artifact)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSONWithStatus(w, artifact, http.StatusCreated)
}

func (handler *Handler) artifactValidate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "artifactId")
	if handlerErr != nil {
		return handlerErr
	}

	artifact, handlerErr := handler.requireArtifactPermission(r, portainer.PlatformArtifactID(id), platformPermissionArtifactRelease)
	if handlerErr != nil {
		return handlerErr
	}

	result := artifactValidationResponse{
		Valid:      artifact.Type == portainer.PlatformArtifactTypeImage && artifact.SourceType == portainer.PlatformArtifactSourceImageReference && artifact.ImageRef != "",
		ArtifactID: artifact.ID,
		ImageRef:   artifact.ImageRef,
	}
	if !result.Valid {
		result.Reason = "INVALID_IMAGE_REFERENCE"
	}

	return response.JSON(w, result)
}

func (handler *Handler) artifactArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "artifactId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireArtifactPermission(r, portainer.PlatformArtifactID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	now := time.Now().Unix()
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		artifact, err := tx.PlatformArtifact().Read(portainer.PlatformArtifactID(id))
		if err != nil {
			return err
		}
		if !isActive(artifact.PlatformLifecycle) {
			return notFoundError("Artifact is archived")
		}

		archiveLifecycle(&artifact.PlatformLifecycle, now, userID)

		return tx.PlatformArtifact().Update(artifact.ID, artifact)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.Empty(w)
}

type artifactFilters struct {
	projectID           portainer.PlatformProjectID
	applicationID       portainer.PlatformApplicationID
	serviceDefinitionID portainer.PlatformServiceDefinitionID
	includeArchived     bool
}

func artifactListFilters(r *http.Request) (artifactFilters, *httperror.HandlerError) {
	projectID, err := optionalQueryID(r, "projectId")
	if err != nil {
		return artifactFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	applicationID, err := optionalQueryID(r, "applicationId")
	if err != nil {
		return artifactFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	serviceDefinitionID, err := optionalQueryID(r, "serviceDefinitionId")
	if err != nil {
		return artifactFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}

	return artifactFilters{
		projectID:           portainer.PlatformProjectID(projectID),
		applicationID:       portainer.PlatformApplicationID(applicationID),
		serviceDefinitionID: portainer.PlatformServiceDefinitionID(serviceDefinitionID),
		includeArchived:     includeArchived(r),
	}, nil
}

func optionalQueryID(r *http.Request, name string) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return 0, nil
	}

	return strconv.Atoi(value)
}
