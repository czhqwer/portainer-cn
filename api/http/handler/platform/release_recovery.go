package platform

import (
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/dataservices/platformreleaselock"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

// releaseRetryRecovery 只面向 recovery-failed/interrupted 的人工恢复入口；
// handler 先做状态校验和历史对象加载，再把真实运行时动作交给执行器。
func (handler *Handler) releaseRetryRecovery(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "releaseId")
	if handlerErr != nil {
		return handlerErr
	}
	release, handlerErr := handler.requireReleasePermission(r, portainer.PlatformReleaseID(id), platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if handlerErr := handler.requireReleaseRuntimePermission(r, release); handlerErr != nil {
		return handlerErr
	}
	if handler.ReleaseRecoveryExecutor == nil {
		return writePlatformError(
			w,
			http.StatusServiceUnavailable,
			errPlatformUnsupportedOperation,
			"Docker release recovery executor is not configured.",
			platformservice.ReleaseFailureReasonExecutorUnavailable,
			nil,
		)
	}

	var executionRequest platformservice.ReleaseExecutionRequest
	err := handler.DataStore.ViewTx(func(tx dataservices.DataStoreTx) error {
		release, err := tx.PlatformRelease().Read(portainer.PlatformReleaseID(id))
		if err != nil {
			return err
		}
		if !releaseCanRetryRecovery(release.Status) {
			return validationFailedError("Release recovery can only be retried from interrupted or recovery-failed")
		}
		if err := validateReleaseTransition(release.Status, portainer.PlatformReleaseStatusRecovering); err != nil {
			return validationFailedError(err.Error())
		}

		executionRequest, err = releaseExecutionRequestFromRelease(tx, release)
		return err
	})
	if err != nil {
		return handler.convertError(err)
	}

	result, err := handler.ReleaseRecoveryExecutor.RetryRecovery(r.Context(), executionRequest)
	if err != nil {
		return handler.convertError(err)
	}
	if err := handler.persistReleaseRecoveryResult(r, result, portainer.PlatformAuditActionReleaseRetryRecovery); err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, result.Release)
}

// releaseCleanupRuntime 用于清理失败发布遗留的候选容器或失败正式容器；
// 该动作不会自动 resolve 发布，锁是否保留仍由 release 当前状态决定。
func (handler *Handler) releaseCleanupRuntime(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "releaseId")
	if handlerErr != nil {
		return handlerErr
	}
	release, handlerErr := handler.requireReleasePermission(r, portainer.PlatformReleaseID(id), platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if handlerErr := handler.requireReleaseRuntimePermission(r, release); handlerErr != nil {
		return handlerErr
	}
	if handler.ReleaseRecoveryExecutor == nil {
		return writePlatformError(
			w,
			http.StatusServiceUnavailable,
			errPlatformUnsupportedOperation,
			"Docker release recovery executor is not configured.",
			platformservice.ReleaseFailureReasonExecutorUnavailable,
			nil,
		)
	}

	var cleanupRequest platformservice.ReleaseCleanupRequest
	err := handler.DataStore.ViewTx(func(tx dataservices.DataStoreTx) error {
		release, err := tx.PlatformRelease().Read(portainer.PlatformReleaseID(id))
		if err != nil {
			return err
		}
		if !releaseCanCleanupRuntime(release.Status) {
			return validationFailedError("Release runtime can only be cleaned from failed, interrupted or recovery-failed")
		}

		deployment, err := tx.PlatformServiceDeployment().Read(release.ServiceDeploymentID)
		if err != nil {
			return err
		}
		cleanupRequest = platformservice.ReleaseCleanupRequest{
			Release:    *release,
			Deployment: *deployment,
		}
		return nil
	})
	if err != nil {
		return handler.convertError(err)
	}

	result, err := handler.ReleaseRecoveryExecutor.CleanupRuntime(r.Context(), cleanupRequest)
	if err != nil {
		return handler.convertError(err)
	}
	if err := handler.persistReleaseCleanupResult(r, result); err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, result)
}

func releaseCanRetryRecovery(status portainer.PlatformReleaseStatus) bool {
	return status == portainer.PlatformReleaseStatusRecoveryFailed || status == portainer.PlatformReleaseStatusInterrupted
}

func releaseCanCleanupRuntime(status portainer.PlatformReleaseStatus) bool {
	return status == portainer.PlatformReleaseStatusFailed ||
		status == portainer.PlatformReleaseStatusRecoveryFailed ||
		status == portainer.PlatformReleaseStatusInterrupted
}

func releaseExecutionRequestFromRelease(tx dataservices.DataStoreTx, release *portainer.PlatformRelease) (platformservice.ReleaseExecutionRequest, error) {
	project, err := tx.PlatformProject().Read(release.ProjectID)
	if err != nil {
		return platformservice.ReleaseExecutionRequest{}, err
	}
	environment, err := tx.PlatformEnvironment().Read(release.EnvironmentID)
	if err != nil {
		return platformservice.ReleaseExecutionRequest{}, err
	}
	application, err := tx.PlatformApplication().Read(release.ApplicationID)
	if err != nil {
		return platformservice.ReleaseExecutionRequest{}, err
	}
	service, err := tx.PlatformServiceDefinition().Read(release.ServiceDefinitionID)
	if err != nil {
		return platformservice.ReleaseExecutionRequest{}, err
	}
	deployment, err := tx.PlatformServiceDeployment().Read(release.ServiceDeploymentID)
	if err != nil {
		return platformservice.ReleaseExecutionRequest{}, err
	}
	artifact, err := tx.PlatformArtifact().Read(release.ArtifactID)
	if err != nil {
		return platformservice.ReleaseExecutionRequest{}, err
	}

	// 人工恢复处理的是已经落库的历史发布，不能重新走新发布校验；
	// 这里直接按 release 关联对象组装执行请求，避免归档状态影响故障处置。
	return platformservice.ReleaseExecutionRequest{
		Project:           *project,
		Environment:       *environment,
		Application:       *application,
		ServiceDefinition: *service,
		Deployment:        *deployment,
		Artifact:          *artifact,
		Release:           *release,
	}, nil
}

func (handler *Handler) persistReleaseRecoveryResult(r *http.Request, result platformservice.ReleaseExecutionResult, action portainer.PlatformAuditAction) error {
	return handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		previous, err := tx.PlatformRelease().Read(result.Release.ID)
		if err != nil {
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
		if err := syncReleaseLock(tx, result.Release); err != nil {
			return err
		}

		auditResult, failureReason := releaseRecoveryAuditResult(result.Release)
		return handler.createReleaseAuditLog(tx, r, action, auditResult, result.Release, releaseAuditSummary(*previous), releaseAuditSummary(result.Release), failureReason)
	})
}

func (handler *Handler) persistReleaseCleanupResult(r *http.Request, result platformservice.ReleaseCleanupResult) error {
	return handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		previous, err := tx.PlatformRelease().Read(result.Release.ID)
		if err != nil {
			return err
		}
		if err := tx.PlatformRelease().Update(result.Release.ID, &result.Release); err != nil {
			return err
		}
		if err := syncReleaseLock(tx, result.Release); err != nil {
			return err
		}

		auditResult := portainer.PlatformAuditResultSuccess
		failureReason := ""
		if len(result.FailedRuntimeRefs) > 0 {
			auditResult = portainer.PlatformAuditResultFailed
			failureReason = platformservice.ReleaseFailureReasonRuntimeCleanupFailed
		}

		return handler.createReleaseAuditLog(tx, r, portainer.PlatformAuditActionReleaseCleanupRuntime, auditResult, result.Release, releaseAuditSummary(*previous), releaseAuditSummary(result.Release), failureReason)
	})
}

func syncReleaseLock(tx dataservices.DataStoreTx, release portainer.PlatformRelease) error {
	lockID := platformreleaselock.LockIDForServiceDeployment(release.ServiceDeploymentID)
	if releaseStatusReleasesLock(release.Status) {
		if err := tx.PlatformReleaseLock().Delete(lockID); err != nil && !tx.IsErrObjectNotFound(err) {
			return err
		}
		return nil
	}

	now := time.Now().Unix()
	lock, err := tx.PlatformReleaseLock().Read(lockID)
	if err != nil {
		if !tx.IsErrObjectNotFound(err) {
			return err
		}
		return tx.PlatformReleaseLock().Create(&portainer.PlatformReleaseLock{
			ServiceDeploymentID: release.ServiceDeploymentID,
			ReleaseID:           release.ID,
			LeaseOwner:          release.LeaseOwner,
			LeaseExpiresAt:      release.LeaseExpiresAt,
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}

	lock.ReleaseID = release.ID
	lock.LeaseOwner = release.LeaseOwner
	lock.LeaseExpiresAt = release.LeaseExpiresAt
	lock.UpdatedAt = now

	return tx.PlatformReleaseLock().Update(lockID, lock)
}

func releaseRecoveryAuditResult(release portainer.PlatformRelease) (portainer.PlatformAuditResult, string) {
	if release.Status == portainer.PlatformReleaseStatusRecoveryFailed {
		return portainer.PlatformAuditResultFailed, release.FailureReason
	}

	return portainer.PlatformAuditResultSuccess, ""
}
