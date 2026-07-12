package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/dataservices/platformreleaselock"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"

	"github.com/segmentio/encoding/json"
)

const (
	releaseIdempotencyMismatchReason       = "IDEMPOTENCY_PAYLOAD_MISMATCH"
	releaseLockedReason                    = "RELEASE_LOCKED"
	idempotencyKeyHeader                   = "Idempotency-Key"
	idempotencyTTLSeconds            int64 = 24 * 60 * 60
)

type releaseCreateResponse struct {
	ReleaseID portainer.PlatformReleaseID     `json:"ReleaseId"`
	Status    portainer.PlatformReleaseStatus `json:"Status"`
	PollURL   string                          `json:"PollUrl"`
	Blocked   bool                            `json:"Blocked,omitempty"`
	Reason    string                          `json:"Reason,omitempty"`
}

type releaseValidateResponse struct {
	Valid                bool                                  `json:"Valid"`
	Executable           bool                                  `json:"Executable"`
	Code                 string                                `json:"Code,omitempty"`
	Reason               string                                `json:"Reason,omitempty"`
	Message              string                                `json:"Message,omitempty"`
	ServiceDeploymentID  portainer.PlatformServiceDeploymentID `json:"ServiceDeploymentId"`
	ArtifactID           portainer.PlatformArtifactID          `json:"ArtifactId"`
	ExpectedSpecRevision int                                   `json:"ExpectedSpecRevision"`
}

type platformCodedError struct {
	status  int
	code    string
	message string
	reason  string
	data    any
}

func (err *platformCodedError) Error() string {
	return err.message
}

func (handler *Handler) releaseList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	filters, handlerErr := releaseListFilters(r)
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

	releases, err := handler.DataStore.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		if visibleProjectIDs != nil && !visibleProjectIDs[release.ProjectID] {
			return false
		}
		if filters.projectID != 0 && release.ProjectID != filters.projectID {
			return false
		}
		if filters.environmentID != 0 && release.EnvironmentID != filters.environmentID {
			return false
		}
		if filters.applicationID != 0 && release.ApplicationID != filters.applicationID {
			return false
		}
		if filters.serviceDefinitionID != 0 && release.ServiceDefinitionID != filters.serviceDefinitionID {
			return false
		}
		if filters.serviceDeploymentID != 0 && release.ServiceDeploymentID != filters.serviceDeploymentID {
			return false
		}
		if filters.status != "" && release.Status != filters.status {
			return false
		}

		return true
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, redactReleases(releases))
}

func (handler *Handler) releaseInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "releaseId")
	if handlerErr != nil {
		return handlerErr
	}

	release, handlerErr := handler.requireReleasePermission(r, portainer.PlatformReleaseID(id), platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}

	return response.JSON(w, redactRelease(*release))
}

func (handler *Handler) releaseValidate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	var payload createReleasePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if handlerErr := handler.requireReleaseRequestPermission(r, payload); handlerErr != nil {
		return handlerErr
	}

	err := handler.DataStore.ViewTx(func(tx dataservices.DataStoreTx) error {
		deployment, _, err := validateReleaseReferences(tx, payload)
		if err != nil {
			return err
		}
		effectiveConfig, err := effectiveConfigForDeployment(tx, *deployment)
		if err != nil {
			return validationFailedError(err.Error())
		}
		if _, err = databaseBindingSnapshotsForDeployment(tx, *deployment, effectiveConfig); err != nil {
			return err
		}
		_, err = secretSnapshotsForDeployment(tx, *deployment)
		if err != nil {
			return validationFailedError(err.Error())
		}
		return nil
	})
	if err != nil {
		return handler.convertError(err)
	}
	if reason := handler.preflightServiceDatabaseBindings(r.Context(), payload.ServiceDeploymentID); reason != "" {
		return response.JSON(w, releaseValidateResponse{Valid: false, Executable: false, Code: errPlatformValidationFailed, Reason: reason, Message: "Database binding preflight failed.", ServiceDeploymentID: payload.ServiceDeploymentID, ArtifactID: payload.ArtifactID, ExpectedSpecRevision: payload.ExpectedSpecRevision})
	}

	if handler.ReleaseExecutor == nil {
		return response.JSON(w, releaseValidateResponse{
			Valid:                true,
			Executable:           false,
			Code:                 errPlatformUnsupportedOperation,
			Reason:               platformservice.ReleaseFailureReasonExecutorUnavailable,
			Message:              "Docker release executor is not configured.",
			ServiceDeploymentID:  payload.ServiceDeploymentID,
			ArtifactID:           payload.ArtifactID,
			ExpectedSpecRevision: payload.ExpectedSpecRevision,
		})
	}

	return response.JSON(w, releaseValidateResponse{
		Valid:                true,
		Executable:           true,
		Message:              "Gate 0B has passed; release execution can start.",
		ServiceDeploymentID:  payload.ServiceDeploymentID,
		ArtifactID:           payload.ArtifactID,
		ExpectedSpecRevision: payload.ExpectedSpecRevision,
	})
}

func (handler *Handler) releaseCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	var payload createReleasePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if handlerErr := handler.requireReleaseRequestPermission(r, payload); handlerErr != nil {
		return handlerErr
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	if idempotencyKey == "" {
		return validationFailed(errors.New("Idempotency-Key header is required"))
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}
	if handler.ReleaseExecutor == nil {
		return writePlatformError(
			w,
			http.StatusServiceUnavailable,
			errPlatformUnsupportedOperation,
			"Docker release executor is not configured.",
			platformservice.ReleaseFailureReasonExecutorUnavailable,
			nil,
		)
	}
	if reason := handler.preflightServiceDatabaseBindings(r.Context(), payload.ServiceDeploymentID); reason != "" {
		return writePlatformError(w, http.StatusBadRequest, errPlatformValidationFailed, "Database binding preflight failed.", reason, nil)
	}

	payloadHash, err := releasePayloadHash(payload)
	if err != nil {
		return handler.convertError(err)
	}
	idempotencyKeyHash := releaseIdempotencyHash(userID, payload.ServiceDeploymentID, idempotencyKey)
	now := time.Now().Unix()
	var release *portainer.PlatformRelease
	var refs *releaseReferenceSet
	shouldExecute := false

	// 发布请求先持久化 Release 与 ReleaseLock，再交给执行器推进；
	// 这样幂等复用和并发互斥不会依赖 Docker 操作是否已经开始。
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		existing, err := findIdempotentRelease(tx, payload.ServiceDeploymentID, userID, idempotencyKeyHash, now)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.PayloadHash != payloadHash {
				return &platformCodedError{
					status:  http.StatusConflict,
					code:    errPlatformIdempotencyPayloadMismatch,
					message: "Idempotency-Key was already used with a different payload.",
					reason:  releaseIdempotencyMismatchReason,
				}
			}
			release = existing
			return nil
		}

		lockID := platformreleaselock.LockIDForServiceDeployment(payload.ServiceDeploymentID)
		lock, err := tx.PlatformReleaseLock().Read(lockID)
		if err != nil && !tx.IsErrObjectNotFound(err) {
			return err
		}
		if lock != nil {
			lockedRelease, readErr := tx.PlatformRelease().Read(lock.ReleaseID)
			if readErr != nil && !tx.IsErrObjectNotFound(readErr) {
				return readErr
			}
			if lock.IdempotencyKeyHash == idempotencyKeyHash {
				if lock.PayloadHash != payloadHash {
					return &platformCodedError{
						status:  http.StatusConflict,
						code:    errPlatformIdempotencyPayloadMismatch,
						message: "Idempotency-Key was already used with a different payload.",
						reason:  releaseIdempotencyMismatchReason,
					}
				}
				release = lockedRelease
				return nil
			}
			if lockedRelease != nil && !releaseStatusReleasesLock(lockedRelease.Status) {
				return &platformCodedError{
					status:  http.StatusConflict,
					code:    errPlatformReleaseConflict,
					message: "ServiceDeployment already has an active release.",
					reason:  releaseLockedReason,
					data:    releaseResponse(lockedRelease),
				}
			}
			if lockedRelease == nil || releaseStatusReleasesLock(lockedRelease.Status) {
				if err := tx.PlatformReleaseLock().Delete(lockID); err != nil {
					return err
				}
			}
		}

		releaseRefs, err := validateReleaseReferenceSet(tx, payload)
		if err != nil {
			return err
		}
		refs = releaseRefs
		effectiveConfig, err := effectiveConfigForDeployment(tx, *refs.deployment)
		if err != nil {
			return validationFailedError(err.Error())
		}
		secretSnapshots, err := secretSnapshotsForDeployment(tx, *refs.deployment)
		if err != nil {
			return validationFailedError(err.Error())
		}
		databaseBindings, err := databaseBindingSnapshotsForDeployment(tx, *refs.deployment, effectiveConfig)
		if err != nil {
			return err
		}

		release = newQueuedRelease(payload, refs.deployment, refs.artifact, effectiveConfig, secretSnapshots, databaseBindings, userID, idempotencyKeyHash, payloadHash, now)

		if err := tx.PlatformRelease().Create(release); err != nil {
			return err
		}

		lock = &portainer.PlatformReleaseLock{
			ServiceDeploymentID: payload.ServiceDeploymentID,
			ReleaseID:           release.ID,
			IdempotencyKeyHash:  idempotencyKeyHash,
			PayloadHash:         payloadHash,
			LeaseOwner:          "platform-release-api",
			LeaseExpiresAt:      now + 15*60,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := tx.PlatformReleaseLock().Create(lock); err != nil {
			return err
		}

		if err := handler.createReleaseAuditLog(tx, r, portainer.PlatformAuditActionReleaseCreated, portainer.PlatformAuditResultSuccess, *release, nil, releaseAuditSummary(*release), ""); err != nil {
			return err
		}

		shouldExecute = true
		return nil
	})
	if err != nil {
		var codedErr *platformCodedError
		if errors.As(err, &codedErr) {
			return writePlatformError(w, codedErr.status, codedErr.code, codedErr.message, codedErr.reason, codedErr.data)
		}

		return handler.convertError(err)
	}

	if shouldExecute {
		executionRequest := platformservice.ReleaseExecutionRequest{
			Project:           *refs.project,
			Environment:       *refs.environment,
			Application:       *refs.application,
			ServiceDefinition: *refs.service,
			Deployment:        *refs.deployment,
			Artifact:          *refs.artifact,
			Release:           *release,
		}

		// 请求已经持久化为 queued Release 后立即返回 Release ID；Docker 操作在后台推进，
		// 这样浏览器可以轮询详情并看到每一步状态，HTTP 事务也不会持有网络或 Docker 调用。
		go handler.executeReleaseAsync(context.WithoutCancel(r.Context()), r, executionRequest)
	}

	return response.JSONWithStatus(w, releaseResponse(release), http.StatusAccepted)
}

func (handler *Handler) executeReleaseAsync(ctx context.Context, request *http.Request, executionRequest platformservice.ReleaseExecutionRequest) {
	executionRequest.Progress = func(progress platformservice.ReleaseExecutionResult) {
		if err := handler.persistReleaseExecutionResult(request, progress); err != nil {
			// 只记录 Release ID 和持久化动作，避免把 Docker 原始错误写入服务器日志。
			log.Printf("platform release progress persistence failed: release_id=%d", progress.Release.ID)
		}
	}

	result, err := handler.ReleaseExecutor.Execute(ctx, executionRequest)
	if err != nil {
		// 执行器返回 Go 错误时请求已回应，不能把未经脱敏的错误反射给用户；
		// 将 Release 留在需要人工处置的 interrupted 状态，避免误释放运行时锁。
		result = interruptedReleaseExecutionResult(executionRequest.Release)
	}
	if err := handler.persistReleaseExecutionResult(request, result); err != nil {
		log.Printf("platform release final persistence failed: release_id=%d", executionRequest.Release.ID)
	}
}

func interruptedReleaseExecutionResult(release portainer.PlatformRelease) platformservice.ReleaseExecutionResult {
	now := time.Now().Unix()
	release.Status = portainer.PlatformReleaseStatusInterrupted
	release.FailureReason = platformservice.ReleaseFailureReasonExecutorUnavailable
	release.ManualActionRequired = true
	release.FinishedAt = now
	release.LeaseOwner = "platform-release-interrupted"
	release.LeaseExpiresAt = now + 15*60

	return platformservice.ReleaseExecutionResult{Release: release}
}

type releaseReferenceSet struct {
	project     *portainer.PlatformProject
	environment *portainer.PlatformEnvironment
	application *portainer.PlatformApplication
	service     *portainer.PlatformServiceDefinition
	deployment  *portainer.PlatformServiceDeployment
	artifact    *portainer.PlatformArtifact
}

func validateReleaseReferences(tx dataservices.DataStoreTx, payload createReleasePayload) (*portainer.PlatformServiceDeployment, *portainer.PlatformArtifact, error) {
	refs, err := validateReleaseReferenceSet(tx, payload)
	if err != nil {
		return nil, nil, err
	}

	return refs.deployment, refs.artifact, nil
}

func validateReleaseReferenceSet(tx dataservices.DataStoreTx, payload createReleasePayload) (*releaseReferenceSet, error) {
	project, err := readActiveProject(tx, payload.ProjectID)
	if err != nil {
		return nil, err
	}
	environment, err := readActiveEnvironment(tx, payload.EnvironmentID)
	if err != nil {
		return nil, err
	}
	application, err := readActiveApplication(tx, payload.ApplicationID)
	if err != nil {
		return nil, err
	}
	service, err := readActiveServiceDefinition(tx, payload.ServiceDefinitionID)
	if err != nil {
		return nil, err
	}
	deployment, err := readActiveServiceDeployment(tx, payload.ServiceDeploymentID)
	if err != nil {
		return nil, err
	}
	artifact, err := tx.PlatformArtifact().Read(payload.ArtifactID)
	if err != nil {
		return nil, err
	}
	if !isActive(artifact.PlatformLifecycle) {
		return nil, notFoundError("Artifact is archived")
	}

	if environment.ProjectID != payload.ProjectID ||
		application.ProjectID != payload.ProjectID ||
		service.ProjectID != payload.ProjectID ||
		deployment.ProjectID != payload.ProjectID ||
		artifact.ProjectID != payload.ProjectID {
		return nil, validationFailedError("Release resources belong to different projects")
	}
	if service.ApplicationID != payload.ApplicationID ||
		deployment.ApplicationID != payload.ApplicationID {
		return nil, validationFailedError("Release resources belong to different applications")
	}
	if deployment.ServiceDefinitionID != payload.ServiceDefinitionID {
		return nil, validationFailedError("ServiceDeployment does not belong to ServiceDefinition")
	}
	if deployment.EnvironmentID != payload.EnvironmentID {
		return nil, validationFailedError("ServiceDeployment does not belong to Environment")
	}
	if artifact.ApplicationID != 0 && artifact.ApplicationID != payload.ApplicationID {
		return nil, validationFailedError("Artifact does not belong to Application")
	}
	if artifact.ServiceDefinitionID != 0 && artifact.ServiceDefinitionID != payload.ServiceDefinitionID {
		return nil, validationFailedError("Artifact does not belong to ServiceDefinition")
	}
	if artifact.Type == portainer.PlatformArtifactTypeImage && artifact.SourceType == portainer.PlatformArtifactSourceImageReference && artifact.ImageRef != "" {
		// 保持阶段 1 的已有镜像兼容性；它们可以继续以弱追溯方式进入原有发布链路。
	} else if artifact.Status != portainer.PlatformArtifactStatusReady || artifact.ImageRef == "" || artifact.ImageDigest == "" || artifact.RegistryID <= 0 {
		// 阶段 3 文件制品只有在推送得到标准 tag 和 digest 后才可发布，避免 Docker Endpoint 直接使用本地候选镜像。
		return nil, validationFailedError("Artifact is not ready for release")
	}
	if deployment.SpecRevision != payload.ExpectedSpecRevision {
		return nil, validationFailedError("ExpectedSpecRevision does not match current deployment SpecRevision")
	}
	if err := portainer.ValidatePlatformDeploymentDesiredSpecV01(deployment.DesiredSpec); err != nil {
		return nil, err
	}

	return &releaseReferenceSet{
		project:     project,
		environment: environment,
		application: application,
		service:     service,
		deployment:  deployment,
		artifact:    artifact,
	}, nil
}

func findIdempotentRelease(tx dataservices.DataStoreTx, serviceDeploymentID portainer.PlatformServiceDeploymentID, userID portainer.UserID, idempotencyKeyHash string, now int64) (*portainer.PlatformRelease, error) {
	releases, err := tx.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		return release.ServiceDeploymentID == serviceDeploymentID &&
			release.OperatorUserID == userID &&
			release.IdempotencyKeyHash == idempotencyKeyHash &&
			now-release.CreatedAt <= idempotencyTTLSeconds
	})
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, nil
	}

	return &releases[0], nil
}

func newQueuedRelease(payload createReleasePayload, deployment *portainer.PlatformServiceDeployment, artifact *portainer.PlatformArtifact, effectiveConfig portainer.PlatformEffectiveConfigSnapshot, secretSnapshots []portainer.PlatformSecretSnapshot, databaseBindings []portainer.PlatformDatabaseBindingSnapshot, userID portainer.UserID, idempotencyKeyHash string, payloadHash string, now int64) *portainer.PlatformRelease {
	return &portainer.PlatformRelease{
		ProjectID:            payload.ProjectID,
		EnvironmentID:        payload.EnvironmentID,
		ApplicationID:        payload.ApplicationID,
		ServiceDefinitionID:  payload.ServiceDefinitionID,
		ServiceDeploymentID:  payload.ServiceDeploymentID,
		ArtifactID:           payload.ArtifactID,
		Version:              payload.Version,
		TriggerType:          payload.TriggerType,
		Strategy:             payload.Strategy,
		Status:               portainer.PlatformReleaseStatusQueued,
		OperatorUserID:       userID,
		IdempotencyKeyHash:   idempotencyKeyHash,
		PayloadHash:          payloadHash,
		ExpectedSpecRevision: payload.ExpectedSpecRevision,
		Image:                artifact.ImageRef,
		ImageDigest:          artifact.ImageDigest,
		Traceability:         artifact.Traceability,
		ArtifactSnapshot:     artifactSnapshotFromArtifact(artifact),
		ConfigSnapshot: portainer.PlatformServiceConfigSnapshot{
			SpecRevision:            deployment.SpecRevision,
			DesiredSpecSnapshot:     deployment.DesiredSpec,
			EffectiveConfigSnapshot: effectiveConfig,
			SecretSnapshots:         secretSnapshots,
			DatabaseBindings:        databaseBindings,
			ConfigHash:              effectiveConfig.Hash,
		},
		TargetSnapshot:    targetSnapshotFromDeployment(deployment),
		HealthCheckResult: portainer.PlatformHealthCheckResult{Status: portainer.PlatformHealthCheckStatusSkipped},
		ResolutionAction:  portainer.PlatformReleaseResolutionNone,
		CreatedAt:         now,
		QueueExpiresAt:    now + 600,
	}
}

func (handler *Handler) persistReleaseExecutionResult(r *http.Request, result platformservice.ReleaseExecutionResult) error {
	return handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		previous, err := tx.PlatformRelease().Read(result.Release.ID)
		if err != nil && !tx.IsErrObjectNotFound(err) {
			return err
		}
		if err := tx.PlatformRelease().Update(result.Release.ID, &result.Release); err != nil {
			return err
		}
		if result.Deployment != nil {
			if err := tx.PlatformServiceDeployment().Update(result.Deployment.ID, result.Deployment); err != nil {
				return err
			}
		}

		if action := releaseAuditActionForRelease(result.Release); action != "" {
			var before map[string]any
			if previous != nil {
				before = releaseAuditSummary(*previous)
			}
			if err := handler.createReleaseAuditLog(tx, r, action, releaseAuditResultForStatus(result.Release.Status), result.Release, before, releaseAuditSummary(result.Release), result.Release.FailureReason); err != nil {
				return err
			}
		}

		lockID := platformreleaselock.LockIDForServiceDeployment(result.Release.ServiceDeploymentID)
		if releaseStatusReleasesLock(result.Release.Status) {
			if err := tx.PlatformReleaseLock().Delete(lockID); err != nil && !tx.IsErrObjectNotFound(err) {
				return err
			}
			return nil
		}

		lock, err := tx.PlatformReleaseLock().Read(lockID)
		if err != nil {
			if tx.IsErrObjectNotFound(err) {
				return nil
			}
			return err
		}
		lock.ReleaseID = result.Release.ID
		lock.LeaseOwner = result.Release.LeaseOwner
		lock.LeaseExpiresAt = result.Release.LeaseExpiresAt
		lock.UpdatedAt = time.Now().Unix()

		return tx.PlatformReleaseLock().Update(lockID, lock)
	})
}

func artifactSnapshotFromArtifact(artifact *portainer.PlatformArtifact) portainer.PlatformArtifactSnapshot {
	return portainer.PlatformArtifactSnapshot{
		ArtifactID:      artifact.ID,
		Name:            artifact.Name,
		Version:         artifact.Version,
		Type:            artifact.Type,
		SourceType:      artifact.SourceType,
		ImageRef:        artifact.ImageRef,
		ImageDigest:     artifact.ImageDigest,
		Traceability:    artifact.Traceability,
		RegistryID:      artifact.RegistryID,
		SHA256:          artifact.SHA256,
		Size:            artifact.Size,
		StorageID:       artifact.StorageID,
		StorageProvider: artifact.StorageProvider,
		StoragePath:     artifact.StoragePath,
		SourcePath:      artifact.SourcePath,
		Retained:        artifact.Retained,
		ImageTag:        artifact.ImageTag,
		BuildTemplate:   artifact.BuildTemplate,
	}
}

func targetSnapshotFromDeployment(deployment *portainer.PlatformServiceDeployment) portainer.PlatformTargetSnapshot {
	return portainer.PlatformTargetSnapshot{
		RuntimeDriver: deployment.DesiredSpec.Runtime.RuntimeDriver,
		ExecutorMode:  portainer.PlatformExecutorModeSingle,
	}
}

func releasePayloadHash(payload createReleasePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return hashString(string(data)), nil
}

func releaseIdempotencyHash(userID portainer.UserID, serviceDeploymentID portainer.PlatformServiceDeploymentID, key string) string {
	return hashString(fmt.Sprintf("%d:%d:%s", userID, serviceDeploymentID, key))
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))

	return hex.EncodeToString(sum[:])
}

func releaseResponse(release *portainer.PlatformRelease) releaseCreateResponse {
	if release == nil {
		return releaseCreateResponse{}
	}

	return releaseCreateResponse{
		ReleaseID: release.ID,
		Status:    release.Status,
		PollURL:   fmt.Sprintf("/api/platform/releases/%d", release.ID),
	}
}

type releaseFilters struct {
	projectID           portainer.PlatformProjectID
	environmentID       portainer.PlatformEnvironmentID
	applicationID       portainer.PlatformApplicationID
	serviceDefinitionID portainer.PlatformServiceDefinitionID
	serviceDeploymentID portainer.PlatformServiceDeploymentID
	status              portainer.PlatformReleaseStatus
}

func releaseListFilters(r *http.Request) (releaseFilters, *httperror.HandlerError) {
	projectID, err := optionalQueryID(r, "projectId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	environmentID, err := optionalQueryID(r, "environmentId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	applicationID, err := optionalQueryID(r, "applicationId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	serviceDefinitionID, err := optionalQueryID(r, "serviceDefinitionId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	serviceDeploymentID, err := optionalQueryID(r, "serviceDeploymentId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}

	return releaseFilters{
		projectID:           portainer.PlatformProjectID(projectID),
		environmentID:       portainer.PlatformEnvironmentID(environmentID),
		applicationID:       portainer.PlatformApplicationID(applicationID),
		serviceDefinitionID: portainer.PlatformServiceDefinitionID(serviceDefinitionID),
		serviceDeploymentID: portainer.PlatformServiceDeploymentID(serviceDeploymentID),
		status:              portainer.PlatformReleaseStatus(r.URL.Query().Get("status")),
	}, nil
}
