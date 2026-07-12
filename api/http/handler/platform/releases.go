package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
		_, err = effectiveConfigForDeployment(tx, *deployment)
		if err != nil {
			return validationFailedError(err.Error())
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

		release = newQueuedRelease(payload, refs.deployment, refs.artifact, effectiveConfig, secretSnapshots, userID, idempotencyKeyHash, payloadHash, now)

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
		result, err := handler.ReleaseExecutor.Execute(r.Context(), platformservice.ReleaseExecutionRequest{
			Project:           *refs.project,
			Environment:       *refs.environment,
			Application:       *refs.application,
			ServiceDefinition: *refs.service,
			Deployment:        *refs.deployment,
			Artifact:          *refs.artifact,
			Release:           *release,
		})
		if err != nil {
			return handler.convertError(err)
		}
		if err := handler.persistReleaseExecutionResult(r, result); err != nil {
			return handler.convertError(err)
		}
		release = &result.Release
	}

	return response.JSONWithStatus(w, releaseResponse(release), http.StatusAccepted)
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
	if artifact.Type != portainer.PlatformArtifactTypeImage || artifact.SourceType != portainer.PlatformArtifactSourceImageReference || artifact.ImageRef == "" {
		return nil, validationFailedError("Only image-reference artifacts are supported in V0.1")
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

func newQueuedRelease(payload createReleasePayload, deployment *portainer.PlatformServiceDeployment, artifact *portainer.PlatformArtifact, effectiveConfig portainer.PlatformEffectiveConfigSnapshot, secretSnapshots []portainer.PlatformSecretSnapshot, userID portainer.UserID, idempotencyKeyHash string, payloadHash string, now int64) *portainer.PlatformRelease {
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
