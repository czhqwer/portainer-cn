package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

const (
	artifactUploadJarLimit   int64 = 1 << 30
	artifactUploadDistLimit  int64 = 512 << 20
	artifactUploadTarLimit   int64 = 10 << 30
	artifactUploadFieldLimit       = 16 << 10
)

type artifactUploadMetadata struct {
	projectID           portainer.PlatformProjectID
	applicationID       portainer.PlatformApplicationID
	serviceDefinitionID portainer.PlatformServiceDefinitionID
	name                string
	version             string
	artifactType        portainer.PlatformArtifactType
	expectedSHA256      string
}

// artifactUpload 通过流式 multipart 保存原始制品。文件不会进入请求内存，也不会在
// BoltDB 中保存服务器路径；只有完成 hash、格式初检和归属校验后才会原子移动并创建 Artifact。
func (handler *Handler) artifactUpload(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	if handler.FileService == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Artifact upload storage is unavailable", "ARTIFACT_STORAGE_UNAVAILABLE", nil)
	}

	// 先限定整个请求，避免攻击者利用大量 multipart 字段绕过单文件上限。
	r.Body = http.MaxBytesReader(w, r.Body, artifactUploadTarLimit+artifactUploadFieldLimit*16)
	reader, err := r.MultipartReader()
	if err != nil {
		return artifactUploadError(w, http.StatusBadRequest, "ARTIFACT_UPLOAD_INVALID")
	}

	metadata := artifactUploadMetadata{}
	metadataSeen := map[string]bool{}
	var projectAuthorized bool

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return artifactUploadError(w, http.StatusBadRequest, "ARTIFACT_UPLOAD_INVALID")
		}

		name := part.FormName()
		if name == "file" {
			if !projectAuthorized {
				return artifactUploadError(w, http.StatusBadRequest, "ARTIFACT_UPLOAD_METADATA_INVALID")
			}
			return handler.persistUploadedArtifact(w, r, part, metadata)
		}
		if name == "" || metadataSeen[name] {
			return artifactUploadError(w, http.StatusBadRequest, "ARTIFACT_UPLOAD_METADATA_INVALID")
		}
		metadataSeen[name] = true

		value, err := readArtifactUploadField(part)
		if err != nil {
			return artifactUploadError(w, http.StatusBadRequest, "ARTIFACT_UPLOAD_METADATA_INVALID")
		}
		if err := metadata.set(name, value); err != nil {
			return artifactUploadError(w, http.StatusBadRequest, "ARTIFACT_UPLOAD_METADATA_INVALID")
		}
		if name == "ProjectId" {
			if metadata.projectID <= 0 {
				return artifactUploadError(w, http.StatusBadRequest, "ARTIFACT_UPLOAD_METADATA_INVALID")
			}
			if _, handlerErr := handler.requireProjectPermission(r, metadata.projectID, platformPermissionArtifactRelease); handlerErr != nil {
				return handlerErr
			}
			projectAuthorized = true
		}
	}

	return artifactUploadError(w, http.StatusBadRequest, "ARTIFACT_FILE_REQUIRED")
}

func (metadata *artifactUploadMetadata) set(name, value string) error {
	value = strings.TrimSpace(value)

	switch name {
	case "ProjectId":
		id, err := strconv.Atoi(value)
		metadata.projectID = portainer.PlatformProjectID(id)
		return err
	case "ApplicationId":
		id, err := strconv.Atoi(value)
		metadata.applicationID = portainer.PlatformApplicationID(id)
		return err
	case "ServiceDefinitionId":
		id, err := strconv.Atoi(value)
		metadata.serviceDefinitionID = portainer.PlatformServiceDefinitionID(id)
		return err
	case "Name":
		metadata.name = value
	case "Version":
		metadata.version = value
	case "Type":
		metadata.artifactType = portainer.PlatformArtifactType(value)
	case "ExpectedSHA256":
		metadata.expectedSHA256 = strings.ToLower(value)
	default:
		return fmt.Errorf("unsupported upload field")
	}

	return nil
}

func readArtifactUploadField(part io.Reader) (string, error) {
	value, err := io.ReadAll(io.LimitReader(part, artifactUploadFieldLimit+1))
	if err != nil || len(value) > artifactUploadFieldLimit {
		return "", errors.New("upload field is invalid")
	}

	return string(value), nil
}

func (handler *Handler) persistUploadedArtifact(w http.ResponseWriter, r *http.Request, part *multipart.Part, metadata artifactUploadMetadata) *httperror.HandlerError {
	if metadata.projectID <= 0 || metadata.name == "" || metadata.version == "" || metadata.artifactType == "" {
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusBadRequest, "ARTIFACT_UPLOAD_METADATA_INVALID")
	}
	if err := validateExpectedArtifactSHA256(metadata.expectedSHA256); err != nil {
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusBadRequest, "ARTIFACT_UPLOAD_METADATA_INVALID")
	}

	fileName, limit, err := artifactUploadFileSpec(part.FileName(), metadata.artifactType)
	if err != nil {
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusBadRequest, "ARTIFACT_TYPE_INVALID")
	}

	storageKey := filepath.ToSlash(filepath.Join("uploads", uuid.NewString()))
	root := filepath.Join(handler.FileService.GetDatastorePath(), "platform-artifacts")
	temporaryDirectory := filepath.Join(root, ".tmp", uuid.NewString())
	finalPath := filepath.Join(root, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(temporaryDirectory, 0o700); err != nil {
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusInternalServerError, "ARTIFACT_UPLOAD_FAILED")
	}
	defer os.RemoveAll(temporaryDirectory)

	temporaryPath := filepath.Join(temporaryDirectory, "artifact")
	file, err := os.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusInternalServerError, "ARTIFACT_UPLOAD_FAILED")
	}

	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(part, limit+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || size > limit {
		_ = os.Remove(temporaryPath)
		if size > limit {
			return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusRequestEntityTooLarge, "ARTIFACT_SIZE_EXCEEDED")
		}
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusBadRequest, "ARTIFACT_UPLOAD_FAILED")
	}

	if err := validateUploadedFile(temporaryPath, metadata.artifactType); err != nil {
		_ = os.Remove(temporaryPath)
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusBadRequest, "ARTIFACT_TYPE_INVALID")
	}

	digest := hex.EncodeToString(hash.Sum(nil))
	if metadata.expectedSHA256 != "" && metadata.expectedSHA256 != digest {
		_ = os.Remove(temporaryPath)
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusBadRequest, "SHA256_MISMATCH")
	}

	if err := os.MkdirAll(filepath.Dir(finalPath), 0o700); err != nil {
		_ = os.Remove(temporaryPath)
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusInternalServerError, "ARTIFACT_UPLOAD_FAILED")
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		_ = os.Remove(temporaryPath)
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusInternalServerError, "ARTIFACT_UPLOAD_FAILED")
	}
	removeFinalFile := true
	defer func() {
		if removeFinalFile {
			_ = os.Remove(finalPath)
		}
	}()

	now := time.Now().Unix()
	artifact := &portainer.PlatformArtifact{
		ProjectID:           metadata.projectID,
		ApplicationID:       metadata.applicationID,
		ServiceDefinitionID: metadata.serviceDefinitionID,
		Name:                metadata.name,
		Version:             metadata.version,
		Type:                metadata.artifactType,
		SourceType:          portainer.PlatformArtifactSourceUpload,
		FileName:            fileName,
		Size:                size,
		SHA256:              digest,
		StorageProvider:     portainer.PlatformStorageProviderLocal,
		StoragePath:         storageKey,
		Retained:            true,
		Cleanable:           false,
		Traceability:        portainer.PlatformTraceabilityStrong,
		Status:              portainer.PlatformArtifactStatusUploaded,
		PlatformLifecycle:   newLifecycle(now),
	}

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if _, err := readActiveProject(tx, artifact.ProjectID); err != nil {
			return err
		}
		if err := validateArtifactOwnership(tx, artifact); err != nil {
			return err
		}
		if err := tx.PlatformArtifact().Create(artifact); err != nil {
			return err
		}

		return handler.createArtifactUploadAuditLog(tx, r, *artifact)
	})
	if err != nil {
		return handler.recordArtifactUploadFailure(w, r, metadata, http.StatusBadRequest, "ARTIFACT_UPLOAD_FAILED")
	}

	removeFinalFile = false
	return response.JSONWithStatus(w, artifactResponse(*artifact), http.StatusCreated)
}

func validateArtifactOwnership(tx dataservices.DataStoreTx, artifact *portainer.PlatformArtifact) error {
	if artifact.ApplicationID != 0 {
		application, err := readActiveApplication(tx, artifact.ApplicationID)
		if err != nil || application.ProjectID != artifact.ProjectID {
			return errors.New("artifact application ownership is invalid")
		}
	}
	if artifact.ServiceDefinitionID == 0 {
		return errors.New("artifact service definition is required")
	}
	service, err := readActiveServiceDefinition(tx, artifact.ServiceDefinitionID)
	if err != nil || service.ProjectID != artifact.ProjectID {
		return errors.New("artifact service ownership is invalid")
	}
	if artifact.ApplicationID == 0 {
		artifact.ApplicationID = service.ApplicationID
	} else if artifact.ApplicationID != service.ApplicationID {
		return errors.New("artifact service application ownership is invalid")
	}

	return nil
}

func artifactUploadFileSpec(originalName string, artifactType portainer.PlatformArtifactType) (string, int64, error) {
	fileName := filepath.Base(strings.TrimSpace(originalName))
	if fileName == "." || fileName == "" || fileName != originalName || strings.ContainsAny(originalName, "/\\") {
		return "", 0, errors.New("file name is unsafe")
	}

	extension := strings.ToLower(filepath.Ext(fileName))
	switch artifactType {
	case portainer.PlatformArtifactTypeJavaJar:
		if extension == ".jar" {
			return fileName, artifactUploadJarLimit, nil
		}
	case portainer.PlatformArtifactTypeFrontendDist:
		if extension == ".zip" {
			return fileName, artifactUploadDistLimit, nil
		}
	case portainer.PlatformArtifactTypeDockerTar, portainer.PlatformArtifactTypeOCIArchive:
		if extension == ".tar" {
			return fileName, artifactUploadTarLimit, nil
		}
	}

	return "", 0, errors.New("artifact extension is not supported")
}

func validateExpectedArtifactSHA256(value string) error {
	if value == "" {
		return nil
	}
	if len(value) != sha256.Size*2 {
		return errors.New("expected SHA256 is invalid")
	}
	_, err := hex.DecodeString(value)
	return err
}

func validateUploadedFile(filePath string, artifactType portainer.PlatformArtifactType) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	header := make([]byte, 512)
	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	header = header[:n]

	switch artifactType {
	case portainer.PlatformArtifactTypeJavaJar, portainer.PlatformArtifactTypeFrontendDist:
		if len(header) < 4 || header[0] != 'P' || header[1] != 'K' {
			return errors.New("zip signature is invalid")
		}
	case portainer.PlatformArtifactTypeDockerTar, portainer.PlatformArtifactTypeOCIArchive:
		if len(header) < 512 || string(header[257:262]) != "ustar" {
			return errors.New("tar signature is invalid")
		}
	}

	return nil
}

func (handler *Handler) createArtifactUploadAuditLog(tx dataservices.DataStoreTx, r *http.Request, artifact portainer.PlatformArtifact) error {
	return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{
		Action:              portainer.PlatformAuditActionArtifactUploaded,
		Result:              portainer.PlatformAuditResultSuccess,
		ProjectID:           artifact.ProjectID,
		ApplicationID:       artifact.ApplicationID,
		ServiceDefinitionID: artifact.ServiceDefinitionID,
		ArtifactID:          artifact.ID,
		AfterSummary: map[string]any{
			"artifactId": artifact.ID,
			"fileName":   artifact.FileName,
			"type":       artifact.Type,
			"size":       artifact.Size,
			"sha256":     artifact.SHA256,
			"source":     artifact.SourceType,
			"status":     artifact.Status,
		},
	})
}

// recordArtifactUploadFailure 以 best-effort 写入失败事实。上传失败本身不能因为审计存储短暂异常
// 改变响应或遗留文件；摘要只保留用户提供的业务标识与 reason，不包含临时路径、内容或 hash 期望值。
func (handler *Handler) recordArtifactUploadFailure(w http.ResponseWriter, r *http.Request, metadata artifactUploadMetadata, status int, reason string) *httperror.HandlerError {
	if handler != nil && handler.DataStore != nil && metadata.projectID > 0 {
		_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
			return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{
				Action:              portainer.PlatformAuditActionArtifactUploadFailed,
				Result:              portainer.PlatformAuditResultFailed,
				ProjectID:           metadata.projectID,
				ApplicationID:       metadata.applicationID,
				ServiceDefinitionID: metadata.serviceDefinitionID,
				FailureReason:       reason,
				AfterSummary: map[string]any{
					"name":    metadata.name,
					"version": metadata.version,
					"type":    metadata.artifactType,
					"source":  portainer.PlatformArtifactSourceUpload,
					"reason":  reason,
				},
			})
		})
	}

	return artifactUploadError(w, status, reason)
}

func artifactUploadError(w http.ResponseWriter, status int, reason string) *httperror.HandlerError {
	return writePlatformError(w, status, errPlatformValidationFailed, "Artifact upload failed", reason, nil)
}
