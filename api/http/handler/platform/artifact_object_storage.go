package platform

import (
	"errors"
	"net/http"
	"path"
	"strings"

	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
)

type fetchObjectStorageArtifactPayload struct {
	ProjectID           portainer.PlatformProjectID           `json:"ProjectId"`
	ApplicationID       portainer.PlatformApplicationID       `json:"ApplicationId,omitempty"`
	ServiceDefinitionID portainer.PlatformServiceDefinitionID `json:"ServiceDefinitionId"`
	StorageID           portainer.PlatformArtifactStorageID   `json:"StorageId"`
	ObjectPath          string                                `json:"ObjectPath"`
	Name                string                                `json:"Name"`
	Version             string                                `json:"Version"`
	Type                portainer.PlatformArtifactType        `json:"Type"`
	ExpectedSHA256      string                                `json:"ExpectedSHA256,omitempty"`
}

func (payload *fetchObjectStorageArtifactPayload) Validate(_ *http.Request) error {
	payload.ObjectPath = strings.TrimSpace(payload.ObjectPath)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Version = strings.TrimSpace(payload.Version)
	payload.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(payload.ExpectedSHA256))
	if payload.ProjectID <= 0 || payload.ServiceDefinitionID <= 0 || payload.StorageID <= 0 || payload.ObjectPath == "" || payload.Name == "" || payload.Version == "" || payload.Type == "" {
		return errors.New("object storage artifact metadata is required")
	}
	return nil
}

type artifactStorageObjectResponse struct {
	Path string `json:"Path"`
	Size int64  `json:"Size"`
}

// artifactObjectStorageFetch 只允许从管理员已登记、且显式授权给项目的配置读取相对对象路径。
// 先完成 metadata 大小检查，再流式落到批次 3 的临时文件管线，避免大对象或失败下载产生 Artifact 记录。
func (handler *Handler) artifactObjectStorageFetch(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	var payload fetchObjectStorageArtifactPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, payload.ProjectID, platformPermissionArtifactRelease); handlerErr != nil {
		return handlerErr
	}
	storage, handlerErr := handler.requireProjectArtifactStorage(r, int(payload.StorageID), payload.ProjectID)
	if handlerErr != nil {
		return handlerErr
	}
	objectKey, err := artifactStorageObjectKey(*storage, payload.ObjectPath)
	if err != nil || strings.TrimSpace(payload.ObjectPath) == "" {
		return validationFailed(errors.New("artifact object path is invalid"))
	}
	fileName := path.Base(objectKey)
	_, limit, err := artifactUploadFileSpec(fileName, payload.Type)
	if err != nil {
		return validationFailed(errors.New("artifact object type is invalid"))
	}
	if err := validateExpectedArtifactSHA256(strings.ToLower(strings.TrimSpace(payload.ExpectedSHA256))); err != nil {
		return validationFailed(errors.New("expected SHA256 is invalid"))
	}
	connection, err := handler.artifactStorageConnection(*storage)
	if err != nil || handler.ArtifactStorageAdapter == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Artifact storage is unavailable", "ARTIFACT_STORAGE_UNAVAILABLE", nil)
	}

	ctx, cancel := contextWithArtifactStorageTimeout(r)
	defer cancel()
	object, err := handler.ArtifactStorageAdapter.HeadObject(ctx, connection, objectKey)
	if err != nil {
		return handler.recordObjectStorageFetchFailure(w, r, payload, classifyArtifactStorageError(ctx), http.StatusBadGateway)
	}
	if object.Size < 0 || object.Size > limit {
		return handler.recordObjectStorageFetchFailure(w, r, payload, "ARTIFACT_SIZE_EXCEEDED", http.StatusRequestEntityTooLarge)
	}
	body, err := handler.ArtifactStorageAdapter.DownloadObject(ctx, connection, objectKey)
	if err != nil {
		return handler.recordObjectStorageFetchFailure(w, r, payload, classifyArtifactStorageError(ctx), http.StatusBadGateway)
	}
	defer body.Close()

	return handler.persistArtifactStream(w, r.WithContext(ctx), body, fileName, artifactUploadMetadata{
		projectID:           payload.ProjectID,
		applicationID:       payload.ApplicationID,
		serviceDefinitionID: payload.ServiceDefinitionID,
		name:                strings.TrimSpace(payload.Name),
		version:             strings.TrimSpace(payload.Version),
		artifactType:        payload.Type,
		expectedSHA256:      strings.ToLower(strings.TrimSpace(payload.ExpectedSHA256)),
	}, artifactInputOrigin{
		sourceType:         portainer.PlatformArtifactSourceObjectStorage,
		storageID:          storage.ID,
		sourcePath:         objectKey,
		expectedSize:       object.Size,
		successAuditAction: portainer.PlatformAuditActionArtifactFetched,
		failureAuditAction: portainer.PlatformAuditActionArtifactFetchFailed,
	})
}

func (handler *Handler) recordObjectStorageFetchFailure(w http.ResponseWriter, r *http.Request, payload fetchObjectStorageArtifactPayload, reason string, status int) *httperror.HandlerError {
	return handler.recordArtifactInputFailure(w, r, artifactUploadMetadata{
		projectID:           payload.ProjectID,
		applicationID:       payload.ApplicationID,
		serviceDefinitionID: payload.ServiceDefinitionID,
		name:                strings.TrimSpace(payload.Name),
		version:             strings.TrimSpace(payload.Version),
		artifactType:        payload.Type,
	}, artifactInputOrigin{
		sourceType:         portainer.PlatformArtifactSourceObjectStorage,
		storageID:          payload.StorageID,
		successAuditAction: portainer.PlatformAuditActionArtifactFetched,
		failureAuditAction: portainer.PlatformAuditActionArtifactFetchFailed,
	}, status, reason)
}

// artifactStorageObjectKey 将用户输入限制为配置前缀下的相对 S3 key，拒绝路径穿越、反斜杠和绝对路径。
func artifactStorageObjectKey(storage portainer.PlatformArtifactStorage, relativePath string) (string, error) {
	relativePath = strings.TrimSpace(relativePath)
	if strings.HasPrefix(relativePath, "/") || strings.Contains(relativePath, "\\") || (relativePath != "" && (strings.HasPrefix(relativePath, ".") || path.Clean(relativePath) != relativePath)) {
		return "", errors.New("artifact object path is unsafe")
	}
	if storage.PathPrefix == "" {
		return relativePath, nil
	}
	if relativePath == "" {
		return storage.PathPrefix, nil
	}
	return storage.PathPrefix + "/" + relativePath, nil
}

func artifactStorageRelativePath(storage portainer.PlatformArtifactStorage, objectKey string) (string, bool) {
	objectKey = strings.TrimSpace(objectKey)
	if strings.HasPrefix(objectKey, "/") || strings.Contains(objectKey, "\\") || objectKey == "" || strings.HasPrefix(objectKey, ".") || path.Clean(objectKey) != objectKey {
		return "", false
	}
	if storage.PathPrefix == "" {
		return objectKey, true
	}
	prefix := storage.PathPrefix + "/"
	if !strings.HasPrefix(objectKey, prefix) {
		return "", false
	}
	return strings.TrimPrefix(objectKey, prefix), true
}
