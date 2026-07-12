package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

const artifactStorageRequestTimeout = time.Minute

type createArtifactStoragePayload struct {
	Name                 string                        `json:"Name"`
	Endpoint             string                        `json:"Endpoint"`
	Region               string                        `json:"Region,omitempty"`
	Bucket               string                        `json:"Bucket"`
	PathPrefix           string                        `json:"PathPrefix,omitempty"`
	UseTLS               bool                          `json:"UseTLS"`
	SkipTLSVerify        bool                          `json:"SkipTLSVerify"`
	AuthorizedProjectIDs []portainer.PlatformProjectID `json:"AuthorizedProjectIds"`
	AccessKey            string                        `json:"AccessKey"`
	SecretKey            string                        `json:"SecretKey"`
}

func (payload *createArtifactStoragePayload) Validate(_ *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Endpoint = strings.TrimSpace(payload.Endpoint)
	payload.Region = strings.TrimSpace(payload.Region)
	payload.Bucket = strings.TrimSpace(payload.Bucket)
	payload.PathPrefix = strings.TrimSpace(payload.PathPrefix)
	payload.AccessKey = strings.TrimSpace(payload.AccessKey)
	payload.SecretKey = strings.TrimSpace(payload.SecretKey)
	if payload.Name == "" || payload.Endpoint == "" || payload.Bucket == "" {
		return errors.New("artifact storage name, endpoint and bucket are required")
	}
	if payload.AccessKey == "" || payload.SecretKey == "" {
		return errors.New("artifact storage credentials are required")
	}
	if len(payload.AuthorizedProjectIDs) == 0 {
		return errors.New("artifact storage authorized projects are required")
	}
	return nil
}

type updateArtifactStoragePayload struct {
	ResourceVersion      int                            `json:"ResourceVersion"`
	Name                 *string                        `json:"Name,omitempty"`
	Endpoint             *string                        `json:"Endpoint,omitempty"`
	Region               *string                        `json:"Region,omitempty"`
	Bucket               *string                        `json:"Bucket,omitempty"`
	PathPrefix           *string                        `json:"PathPrefix,omitempty"`
	UseTLS               *bool                          `json:"UseTLS,omitempty"`
	SkipTLSVerify        *bool                          `json:"SkipTLSVerify,omitempty"`
	AuthorizedProjectIDs *[]portainer.PlatformProjectID `json:"AuthorizedProjectIds,omitempty"`
	AccessKey            *string                        `json:"AccessKey,omitempty"`
	SecretKey            *string                        `json:"SecretKey,omitempty"`
}

func (payload *updateArtifactStoragePayload) Validate(_ *http.Request) error {
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	for _, value := range []*string{payload.Name, payload.Endpoint, payload.Region, payload.Bucket, payload.PathPrefix, payload.AccessKey, payload.SecretKey} {
		if value != nil {
			trimmed := strings.TrimSpace(*value)
			*value = trimmed
		}
	}
	if (payload.AccessKey == nil) != (payload.SecretKey == nil) {
		return errors.New("artifact storage credentials must be provided together")
	}
	if payload.AccessKey != nil && (*payload.AccessKey == "" || *payload.SecretKey == "") {
		return errors.New("artifact storage credentials are required")
	}
	return nil
}

type artifactStorageResponse struct {
	ID                    portainer.PlatformArtifactStorageID       `json:"Id"`
	Name                  string                                    `json:"Name"`
	Provider              portainer.PlatformArtifactStorageProvider `json:"Provider"`
	Endpoint              string                                    `json:"Endpoint"`
	Region                string                                    `json:"Region,omitempty"`
	Bucket                string                                    `json:"Bucket"`
	PathPrefix            string                                    `json:"PathPrefix,omitempty"`
	UseTLS                bool                                      `json:"UseTLS"`
	SkipTLSVerify         bool                                      `json:"SkipTLSVerify"`
	AuthorizedProjectIDs  []portainer.PlatformProjectID             `json:"AuthorizedProjectIds"`
	CredentialsConfigured bool                                      `json:"CredentialsConfigured"`
	portainer.PlatformLifecycle
}

type artifactStorageSelectionResponse struct {
	ID       portainer.PlatformArtifactStorageID       `json:"Id"`
	Name     string                                    `json:"Name"`
	Provider portainer.PlatformArtifactStorageProvider `json:"Provider"`
}

type artifactStorageTestResponse struct {
	Success bool `json:"Success"`
}

// artifactStorageList 是管理员维护受管 endpoint 的唯一入口；项目成员不应获得 bucket、endpoint 或凭据元数据。
func (handler *Handler) artifactStorageList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	if handlerErr := handler.requireGlobalAdmin(r); handlerErr != nil {
		return handlerErr
	}

	storages, err := handler.DataStore.PlatformArtifactStorage().ReadAll(func(storage portainer.PlatformArtifactStorage) bool {
		return isActive(storage.PlatformLifecycle)
	})
	if err != nil {
		return handler.convertError(err)
	}

	result := make([]artifactStorageResponse, len(storages))
	for i := range storages {
		result[i] = redactArtifactStorage(storages[i])
	}
	return response.JSON(w, result)
}

func (handler *Handler) artifactStorageCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	if handlerErr := handler.requireGlobalAdmin(r); handlerErr != nil {
		return handlerErr
	}

	var payload createArtifactStoragePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	storage := portainer.NewPlatformArtifactStorage()
	storage.Name = payload.Name
	storage.Endpoint = payload.Endpoint
	storage.Region = payload.Region
	storage.Bucket = payload.Bucket
	storage.PathPrefix = payload.PathPrefix
	storage.UseTLS = payload.UseTLS
	storage.SkipTLSVerify = payload.SkipTLSVerify
	storage.AuthorizedProjectIDs = append([]portainer.PlatformProjectID(nil), payload.AuthorizedProjectIDs...)
	storage.PlatformLifecycle = newLifecycle(time.Now().Unix())
	if err := handler.setArtifactStorageCredentials(&storage, payload.AccessKey, payload.SecretKey); err != nil {
		return validationFailedError("Artifact storage credentials are unavailable")
	}

	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if err := validateArtifactStorageProjects(tx, storage.AuthorizedProjectIDs); err != nil {
			return err
		}
		if err := tx.PlatformArtifactStorage().Create(&storage); err != nil {
			return err
		}
		return handler.createArtifactStorageAuditLog(tx, r, portainer.PlatformAuditActionArtifactStorageCreated, portainer.PlatformAuditResultSuccess, storage, "")
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSONWithStatus(w, redactArtifactStorage(storage), http.StatusCreated)
}

func (handler *Handler) artifactStorageUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	if handlerErr := handler.requireGlobalAdmin(r); handlerErr != nil {
		return handlerErr
	}
	id, handlerErr := handler.routeID(r, "storageId")
	if handlerErr != nil {
		return handlerErr
	}
	var payload updateArtifactStoragePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	var storage *portainer.PlatformArtifactStorage
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		storage, err = tx.PlatformArtifactStorage().Read(portainer.PlatformArtifactStorageID(id))
		if err != nil {
			return err
		}
		if !isActive(storage.PlatformLifecycle) {
			return notFoundError("Artifact storage is archived")
		}
		if err := requireResourceVersion(storage.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}
		if payload.Name != nil {
			storage.Name = *payload.Name
		}
		if payload.Endpoint != nil {
			storage.Endpoint = *payload.Endpoint
		}
		if payload.Region != nil {
			storage.Region = *payload.Region
		}
		if payload.Bucket != nil {
			storage.Bucket = *payload.Bucket
		}
		if payload.PathPrefix != nil {
			storage.PathPrefix = *payload.PathPrefix
		}
		if payload.UseTLS != nil {
			storage.UseTLS = *payload.UseTLS
		}
		if payload.SkipTLSVerify != nil {
			storage.SkipTLSVerify = *payload.SkipTLSVerify
		}
		if payload.AuthorizedProjectIDs != nil {
			storage.AuthorizedProjectIDs = append([]portainer.PlatformProjectID(nil), (*payload.AuthorizedProjectIDs)...)
		}
		if payload.AccessKey != nil || payload.SecretKey != nil {
			if payload.AccessKey == nil || payload.SecretKey == nil {
				return validationFailedError("Artifact storage credentials must be provided together")
			}
			if err := handler.setArtifactStorageCredentials(storage, *payload.AccessKey, *payload.SecretKey); err != nil {
				return validationFailedError("Artifact storage credentials are unavailable")
			}
		}
		if err := validateArtifactStorageProjects(tx, storage.AuthorizedProjectIDs); err != nil {
			return err
		}
		touchLifecycle(&storage.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformArtifactStorage().Update(storage.ID, storage); err != nil {
			return err
		}
		return handler.createArtifactStorageAuditLog(tx, r, portainer.PlatformAuditActionArtifactStorageUpdated, portainer.PlatformAuditResultSuccess, *storage, "")
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, redactArtifactStorage(*storage))
}

// artifactStorageTest 在数据库事务之外执行网络探测，避免慢速 S3 endpoint 长时间占用 BoltDB 写锁。
func (handler *Handler) artifactStorageTest(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	if handlerErr := handler.requireGlobalAdmin(r); handlerErr != nil {
		return handlerErr
	}
	id, handlerErr := handler.routeID(r, "storageId")
	if handlerErr != nil {
		return handlerErr
	}
	storage, err := handler.DataStore.PlatformArtifactStorage().Read(portainer.PlatformArtifactStorageID(id))
	if err != nil || !isActive(storage.PlatformLifecycle) {
		return notFoundError("Artifact storage is unavailable")
	}
	connection, err := handler.artifactStorageConnection(*storage)
	if err != nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Artifact storage is unavailable", "ARTIFACT_STORAGE_UNAVAILABLE", nil)
	}
	if handler.ArtifactStorageAdapter == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Artifact storage is unavailable", "ARTIFACT_STORAGE_UNAVAILABLE", nil)
	}

	ctx, cancel := contextWithArtifactStorageTimeout(r)
	defer cancel()
	err = handler.ArtifactStorageAdapter.Test(ctx, connection)
	result := portainer.PlatformAuditResultSuccess
	reason := ""
	if err != nil {
		result = portainer.PlatformAuditResultFailed
		reason = classifyArtifactStorageError(ctx)
	}
	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		return handler.createArtifactStorageAuditLog(tx, r, portainer.PlatformAuditActionArtifactStorageTested, result, *storage, reason)
	})
	if err != nil {
		return writePlatformError(w, http.StatusBadGateway, errPlatformValidationFailed, "Artifact storage test failed", reason, nil)
	}

	return response.JSON(w, artifactStorageTestResponse{Success: true})
}

func (handler *Handler) projectArtifactStorageList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionArtifactRelease); handlerErr != nil {
		return handlerErr
	}
	storages, err := handler.DataStore.PlatformArtifactStorage().ReadAll(func(storage portainer.PlatformArtifactStorage) bool {
		return isActive(storage.PlatformLifecycle) && artifactStorageAllowsProject(storage, portainer.PlatformProjectID(projectID))
	})
	if err != nil {
		return handler.convertError(err)
	}

	result := make([]artifactStorageSelectionResponse, len(storages))
	for i := range storages {
		result[i] = artifactStorageSelectionResponse{ID: storages[i].ID, Name: storages[i].Name, Provider: storages[i].Provider}
	}
	return response.JSON(w, result)
}

func (handler *Handler) artifactStorageObjectList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	storageID, handlerErr := handler.routeID(r, "storageId")
	if handlerErr != nil {
		return handlerErr
	}
	projectID, err := queryRequiredPlatformProjectID(r)
	if err != nil {
		return validationFailed(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, projectID, platformPermissionArtifactRelease); handlerErr != nil {
		return handlerErr
	}
	storage, handlerErr := handler.requireProjectArtifactStorage(r, storageID, projectID)
	if handlerErr != nil {
		return handlerErr
	}
	key, err := artifactStorageObjectKey(*storage, r.URL.Query().Get("prefix"))
	if err != nil {
		return validationFailed(err)
	}
	connection, err := handler.artifactStorageConnection(*storage)
	if err != nil || handler.ArtifactStorageAdapter == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Artifact storage is unavailable", "ARTIFACT_STORAGE_UNAVAILABLE", nil)
	}
	ctx, cancel := contextWithArtifactStorageTimeout(r)
	defer cancel()
	objects, err := handler.ArtifactStorageAdapter.ListObjects(ctx, connection, key)
	if err != nil {
		return writePlatformError(w, http.StatusBadGateway, errPlatformValidationFailed, "Artifact storage list failed", classifyArtifactStorageError(ctx), nil)
	}

	result := make([]artifactStorageObjectResponse, 0, len(objects))
	for _, object := range objects {
		relativePath, ok := artifactStorageRelativePath(*storage, object.Key)
		if !ok {
			continue
		}
		result = append(result, artifactStorageObjectResponse{Path: relativePath, Size: object.Size})
	}
	return response.JSON(w, result)
}

func redactArtifactStorage(storage portainer.PlatformArtifactStorage) artifactStorageResponse {
	return artifactStorageResponse{
		ID:                    storage.ID,
		Name:                  storage.Name,
		Provider:              storage.Provider,
		Endpoint:              storage.Endpoint,
		Region:                storage.Region,
		Bucket:                storage.Bucket,
		PathPrefix:            storage.PathPrefix,
		UseTLS:                storage.UseTLS,
		SkipTLSVerify:         storage.SkipTLSVerify,
		AuthorizedProjectIDs:  append([]portainer.PlatformProjectID(nil), storage.AuthorizedProjectIDs...),
		CredentialsConfigured: storage.AccessKeyCipherText != "" && storage.SecretKeyCipherText != "",
		PlatformLifecycle:     storage.PlatformLifecycle,
	}
}

func (handler *Handler) setArtifactStorageCredentials(storage *portainer.PlatformArtifactStorage, accessKey, secretKey string) error {
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	if accessKey == "" || secretKey == "" {
		return errors.New("artifact storage credentials are required")
	}
	cipher, err := platformservice.NewSecretCipher(handler.DataStore.Connection())
	if err != nil {
		return err
	}
	accessCipherText, _, err := cipher.Encrypt(accessKey)
	if err != nil {
		return err
	}
	secretCipherText, _, err := cipher.Encrypt(secretKey)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(accessKey + "\x00" + secretKey))
	storage.AccessKeyCipherText = accessCipherText
	storage.SecretKeyCipherText = secretCipherText
	storage.CredentialEncryptionVersion = portainer.PlatformArtifactStorageCredentialEncryptionVersion
	storage.CredentialHash = hex.EncodeToString(sum[:])
	return nil
}

// artifactStorageConnection 在真正发起 S3 调用的最后一刻解密凭据，避免明文跨越 datastore 或审计边界。
func (handler *Handler) artifactStorageConnection(storage portainer.PlatformArtifactStorage) (platformservice.ArtifactStorageConnection, error) {
	cipher, err := platformservice.NewSecretCipher(handler.DataStore.Connection())
	if err != nil {
		return platformservice.ArtifactStorageConnection{}, err
	}
	accessKey, err := cipher.Decrypt(storage.AccessKeyCipherText)
	if err != nil {
		return platformservice.ArtifactStorageConnection{}, err
	}
	secretKey, err := cipher.Decrypt(storage.SecretKeyCipherText)
	if err != nil {
		return platformservice.ArtifactStorageConnection{}, err
	}
	return platformservice.ArtifactStorageConnection{
		Endpoint:      storage.Endpoint,
		Region:        storage.Region,
		Bucket:        storage.Bucket,
		AccessKey:     accessKey,
		SecretKey:     secretKey,
		SkipTLSVerify: storage.SkipTLSVerify,
	}, nil
}

func validateArtifactStorageProjects(tx dataservices.DataStoreTx, projectIDs []portainer.PlatformProjectID) error {
	for _, projectID := range projectIDs {
		if _, err := readActiveProject(tx, projectID); err != nil {
			return err
		}
	}
	return nil
}

func artifactStorageAllowsProject(storage portainer.PlatformArtifactStorage, projectID portainer.PlatformProjectID) bool {
	for _, authorizedProjectID := range storage.AuthorizedProjectIDs {
		if authorizedProjectID == projectID {
			return true
		}
	}
	return false
}

func (handler *Handler) requireProjectArtifactStorage(r *http.Request, storageID int, projectID portainer.PlatformProjectID) (*portainer.PlatformArtifactStorage, *httperror.HandlerError) {
	storage, err := handler.DataStore.PlatformArtifactStorage().Read(portainer.PlatformArtifactStorageID(storageID))
	if err != nil || !isActive(storage.PlatformLifecycle) || !artifactStorageAllowsProject(*storage, projectID) {
		handler.recordPlatformDeniedAudit(r, projectID, 0, "artifact-storage", "artifact storage is not authorized for this project")
		return nil, platformAccessDenied()
	}
	return storage, nil
}

func (handler *Handler) createArtifactStorageAuditLog(tx dataservices.DataStoreTx, r *http.Request, action portainer.PlatformAuditAction, result portainer.PlatformAuditResult, storage portainer.PlatformArtifactStorage, reason string) error {
	return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{
		Action:        action,
		Result:        result,
		FailureReason: reason,
		AfterSummary: map[string]any{
			"artifactStorageId":      storage.ID,
			"name":                   storage.Name,
			"provider":               storage.Provider,
			"authorizedProjectCount": len(storage.AuthorizedProjectIDs),
			"credentialsConfigured":  storage.AccessKeyCipherText != "" && storage.SecretKeyCipherText != "",
		},
		SensitiveFields: []string{"accessKey", "secretKey"},
	})
}

func queryRequiredPlatformProjectID(r *http.Request) (portainer.PlatformProjectID, error) {
	value := strings.TrimSpace(r.URL.Query().Get("projectId"))
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, errors.New("project ID is required")
	}
	return portainer.PlatformProjectID(id), nil
}

func contextWithArtifactStorageTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), artifactStorageRequestTimeout)
}

func classifyArtifactStorageError(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "ARTIFACT_STORAGE_TIMEOUT"
	}
	return "ARTIFACT_STORAGE_REQUEST_FAILED"
}
