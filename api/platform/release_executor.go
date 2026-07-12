package platform

import (
	"context"
	"errors"
	"time"

	portainer "github.com/portainer/portainer/api"
)

const (
	ReleaseFailureReasonExecutorUnavailable        = "EXECUTOR_UNAVAILABLE"
	ReleaseFailureReasonTargetNotConfigured        = "TARGET_NOT_CONFIGURED"
	ReleaseFailureReasonTargetModeUnsupported      = "TARGET_MODE_UNSUPPORTED"
	ReleaseFailureReasonRuntimeCleanupFailed       = "RUNTIME_CLEANUP_FAILED"
	ReleaseFailureReasonProductionRequiresVerify   = "PRODUCTION_REQUIRES_VERIFIED"
	ReleaseFailureReasonImagePullFailed            = "IMAGE_PULL_FAILED"
	ReleaseFailureReasonCandidateStartFailed       = "CANDIDATE_START_FAILED"
	ReleaseFailureReasonCandidateHealthFailed      = "CANDIDATE_HEALTH_FAILED"
	ReleaseFailureReasonCandidateCleanupFailed     = "CANDIDATE_CLEANUP_FAILED"
	ReleaseFailureReasonSwitchFailed               = "SWITCH_FAILED"
	ReleaseFailureReasonFinalHealthFailed          = "FINAL_HEALTH_FAILED"
	ReleaseFailureReasonRecoveryFailed             = "RECOVERY_FAILED"
	ReleaseFailureReasonHealthcheckFailed          = "HEALTHCHECK_FAILED"
	ReleaseFailureReasonHealthcheckHostUnreachable = "HEALTHCHECK_HOST_UNREACHABLE"
	ReleaseFailureReasonRuntimeOperationFailed     = "RUNTIME_OPERATION_FAILED"
	ReleaseFailureReasonGatewayCutoverFailed       = "GATEWAY_CUTOVER_FAILED"
)

// ReleaseExecutor owns the runtime side of a platform release. The HTTP layer
// persists the release and lock first, then delegates Docker/Agent work through
// this interface so later workerization does not change the control-plane API.
type ReleaseExecutor interface {
	Execute(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error)
}

type ReleaseRecoveryExecutor interface {
	RetryRecovery(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error)
	CleanupRuntime(ctx context.Context, request ReleaseCleanupRequest) (ReleaseCleanupResult, error)
}

type ReleaseExecutionRequest struct {
	Project           portainer.PlatformProject
	Environment       portainer.PlatformEnvironment
	Application       portainer.PlatformApplication
	ServiceDefinition portainer.PlatformServiceDefinition
	Deployment        portainer.PlatformServiceDeployment
	Artifact          portainer.PlatformArtifact
	Release           portainer.PlatformRelease
	Progress          ReleaseProgressReporter
}

// GatewayCutover 在新运行时最终健康检查通过后、Release 提交成功前更新中心网关。
// 它返回的是不可变摘要；真实配置文件和私钥仍保留在受控目录中，不能进入 Release JSON。
type GatewayCutover interface {
	Cutover(ctx context.Context, request ReleaseExecutionRequest, release portainer.PlatformRelease) (portainer.PlatformGatewaySnapshot, error)
}

// ReleaseProgressReporter 让控制面在每个 Docker 操作间持久化结构化发布事实；
// 回调只接收脱敏后的 Release 记录，避免把运行时凭据或原始驱动输出带到 HTTP 客户端。
type ReleaseProgressReporter func(ReleaseExecutionResult)

type ReleaseExecutionResult struct {
	Release    portainer.PlatformRelease
	Deployment *portainer.PlatformServiceDeployment
}

type ReleaseCleanupRequest struct {
	Release    portainer.PlatformRelease
	Deployment portainer.PlatformServiceDeployment
}

type ReleaseCleanupResult struct {
	Release            portainer.PlatformRelease
	DeletedRuntimeRefs []portainer.RuntimeRef
	FailedRuntimeRefs  []portainer.RuntimeRef
}

type RuntimeDriver interface {
	PullImage(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget) error
	StartCandidate(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget) (portainer.RuntimeRef, []portainer.PlatformPublishedPort, error)
	ValidateRuntime(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, runtimeRef portainer.RuntimeRef, ports []portainer.PlatformPublishedPort) (portainer.PlatformHealthCheckResult, error)
	DeleteRuntime(ctx context.Context, runtimeRef portainer.RuntimeRef) error
	Switch(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, snapshot CandidateValidationSnapshot) (RuntimeSwitchResult, error)
	Recover(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget) error
}

type CandidateValidationSnapshot struct {
	RuntimeRef       portainer.RuntimeRef
	PublishedPorts   []portainer.PlatformPublishedPort
	HealthCheck      portainer.PlatformHealthCheckResult
	ValidationPassed bool
	ValidatedAt      int64
}

type RuntimeSwitchResult struct {
	CurrentRuntimeRef   portainer.RuntimeRef
	PublishedPorts      []portainer.PlatformPublishedPort
	RetainedRuntimeRefs []portainer.RuntimeRef
}

type SingleTargetExecutor struct {
	driver         RuntimeDriver
	gatewayCutover GatewayCutover
	now            func() time.Time
}

func (executor *SingleTargetExecutor) WithGatewayCutover(cutover GatewayCutover) *SingleTargetExecutor {
	executor.gatewayCutover = cutover
	return executor
}

func reportReleaseProgress(request ReleaseExecutionRequest, release portainer.PlatformRelease) {
	if request.Progress != nil {
		request.Progress(ReleaseExecutionResult{Release: release})
	}
}

func NewSingleTargetExecutor(driver RuntimeDriver) *SingleTargetExecutor {
	return &SingleTargetExecutor{
		driver: driver,
		now:    time.Now,
	}
}

func (executor *SingleTargetExecutor) Execute(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error) {
	release := request.Release
	deployment := request.Deployment
	now := executor.unixNow()

	release.StartedAt = now
	release.LeaseOwner = "platform-single-target-executor"
	release.LeaseExpiresAt = now + 15*60

	if executor.driver == nil {
		failRelease(&release, ReleaseFailureReasonExecutorUnavailable, "Docker release executor is not configured.", now)
		return ReleaseExecutionResult{Release: release}, nil
	}

	target, err := selectSingleWorkloadTarget(request.Environment)
	if err != nil {
		release.TargetSnapshot = targetSnapshotFromTarget(target, request.Deployment.DesiredSpec.Runtime.RuntimeDriver)
		reason := failureReasonForError(err)
		failRelease(&release, reason, safeReleaseFailureMessage(reason), now)
		return ReleaseExecutionResult{Release: release}, nil
	}
	release.TargetSnapshot = targetSnapshotFromTarget(target, request.Deployment.DesiredSpec.Runtime.RuntimeDriver)

	if request.Environment.IsProduction && request.Deployment.DesiredSpec.HealthCheck.VerificationLevel != portainer.PlatformHealthVerificationVerified {
		failRelease(&release, ReleaseFailureReasonProductionRequiresVerify, "Production releases require verified health checks in V0.1.", now)
		return ReleaseExecutionResult{Release: release}, nil
	}

	release.Status = portainer.PlatformReleaseStatusValidating
	appendStep(&release, "validate", portainer.PlatformReleaseStepStatusSucceeded, "", "Release references and V0.1 target are valid.", now, executor.unixNow(), portainer.RuntimeRef{})
	reportReleaseProgress(request, release)

	release.Status = portainer.PlatformReleaseStatusPulling
	reportReleaseProgress(request, release)
	stepStart := executor.unixNow()
	if err := executor.driver.PullImage(ctx, request, target); err != nil {
		message := safeReleaseFailureMessage(ReleaseFailureReasonImagePullFailed)
		appendStep(&release, "pull-image", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonImagePullFailed, message, stepStart, executor.unixNow(), portainer.RuntimeRef{})
		failRelease(&release, ReleaseFailureReasonImagePullFailed, message, executor.unixNow())
		return ReleaseExecutionResult{Release: release}, nil
	}
	appendStep(&release, "pull-image", portainer.PlatformReleaseStepStatusSucceeded, "", "Image pulled or already present.", stepStart, executor.unixNow(), portainer.RuntimeRef{})

	release.Status = portainer.PlatformReleaseStatusPreparing
	appendStep(&release, "prepare-runtime", portainer.PlatformReleaseStepStatusSucceeded, "", "Runtime configuration prepared.", executor.unixNow(), executor.unixNow(), portainer.RuntimeRef{})
	reportReleaseProgress(request, release)

	release.Status = portainer.PlatformReleaseStatusCandidateStarting
	reportReleaseProgress(request, release)
	stepStart = executor.unixNow()
	candidateRef, candidatePorts, err := executor.driver.StartCandidate(ctx, request, target)
	if err != nil {
		message := safeReleaseFailureMessage(ReleaseFailureReasonCandidateStartFailed)
		appendStep(&release, "start-candidate", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonCandidateStartFailed, message, stepStart, executor.unixNow(), portainer.RuntimeRef{})
		failRelease(&release, ReleaseFailureReasonCandidateStartFailed, message, executor.unixNow())
		return ReleaseExecutionResult{Release: release}, nil
	}
	release.RuntimeSnapshot.CandidateRuntimeRef = candidateRef
	appendStep(&release, "start-candidate", portainer.PlatformReleaseStepStatusSucceeded, "", "Candidate container started with random published ports.", stepStart, executor.unixNow(), candidateRef)

	release.Status = portainer.PlatformReleaseStatusCandidateChecking
	reportReleaseProgress(request, release)
	stepStart = executor.unixNow()
	health, err := executor.driver.ValidateRuntime(ctx, request, target, candidateRef, candidatePorts)
	release.HealthCheckResult = health
	// 运行时驱动的原始诊断可能包含内部地址；Release 只保存固定失败原因生成的安全消息。
	release.HealthCheckResult.ErrorMessage = ""
	if err != nil || health.Status == portainer.PlatformHealthCheckStatusFailed {
		reason := ReleaseFailureReasonCandidateHealthFailed
		if err != nil {
			reason = failureReasonForError(err)
			if reason == ReleaseFailureReasonRuntimeOperationFailed {
				reason = ReleaseFailureReasonCandidateHealthFailed
			}
		}
		message := safeReleaseFailureMessage(reason)
		appendStep(&release, "check-candidate", portainer.PlatformReleaseStepStatusFailed, reason, message, stepStart, executor.unixNow(), candidateRef)
		_ = executor.driver.DeleteRuntime(ctx, candidateRef)
		failRelease(&release, reason, message, executor.unixNow())
		return ReleaseExecutionResult{Release: release}, nil
	}
	appendStep(&release, "check-candidate", portainer.PlatformReleaseStepStatusSucceeded, "", "Candidate health check passed.", stepStart, executor.unixNow(), candidateRef)
	reportReleaseProgress(request, release)

	stepStart = executor.unixNow()
	if err := executor.driver.DeleteRuntime(ctx, candidateRef); err != nil {
		message := safeReleaseFailureMessage(ReleaseFailureReasonCandidateCleanupFailed)
		appendStep(&release, "cleanup-candidate", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonCandidateCleanupFailed, message, stepStart, executor.unixNow(), candidateRef)
		release.ManualActionRequired = true
		failRelease(&release, ReleaseFailureReasonCandidateCleanupFailed, message, executor.unixNow())
		return ReleaseExecutionResult{Release: release}, nil
	}
	appendStep(&release, "cleanup-candidate", portainer.PlatformReleaseStepStatusSucceeded, "", "Candidate validation snapshot persisted; candidate container removed before switch.", stepStart, executor.unixNow(), candidateRef)

	snapshot := CandidateValidationSnapshot{
		RuntimeRef:       candidateRef,
		PublishedPorts:   candidatePorts,
		HealthCheck:      health,
		ValidationPassed: true,
		ValidatedAt:      executor.unixNow(),
	}

	release.Status = portainer.PlatformReleaseStatusSwitching
	reportReleaseProgress(request, release)
	stepStart = executor.unixNow()
	switchResult, err := executor.driver.Switch(ctx, request, target, snapshot)
	if err != nil {
		message := safeReleaseFailureMessage(ReleaseFailureReasonSwitchFailed)
		appendStep(&release, "switch-runtime", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonSwitchFailed, message, stepStart, executor.unixNow(), portainer.RuntimeRef{})
		return executor.recover(ctx, request, target, release, deployment, ReleaseFailureReasonSwitchFailed, errors.New(message))
	}
	release.RuntimeSnapshot.PreviousRuntimeRef = request.Deployment.CurrentRuntimeRef
	release.RuntimeSnapshot.CurrentRuntimeRef = switchResult.CurrentRuntimeRef
	release.RuntimeSnapshot.PublishedPorts = switchResult.PublishedPorts
	release.RuntimeSnapshot.RetainedRuntimeRefs = switchResult.RetainedRuntimeRefs
	appendStep(&release, "switch-runtime", portainer.PlatformReleaseStepStatusSucceeded, "", "Formal container switched to desired published ports.", stepStart, executor.unixNow(), switchResult.CurrentRuntimeRef)

	release.Status = portainer.PlatformReleaseStatusFinalChecking
	reportReleaseProgress(request, release)
	stepStart = executor.unixNow()
	finalHealth, err := executor.driver.ValidateRuntime(ctx, request, target, switchResult.CurrentRuntimeRef, switchResult.PublishedPorts)
	release.HealthCheckResult = finalHealth
	release.HealthCheckResult.ErrorMessage = ""
	if err != nil || finalHealth.Status == portainer.PlatformHealthCheckStatusFailed {
		reason := ReleaseFailureReasonFinalHealthFailed
		if err != nil {
			reason = failureReasonForError(err)
			if reason == ReleaseFailureReasonRuntimeOperationFailed {
				reason = ReleaseFailureReasonFinalHealthFailed
			}
		}
		message := safeReleaseFailureMessage(reason)
		appendStep(&release, "check-current", portainer.PlatformReleaseStepStatusFailed, reason, message, stepStart, executor.unixNow(), switchResult.CurrentRuntimeRef)
		_ = executor.driver.DeleteRuntime(ctx, switchResult.CurrentRuntimeRef)
		return executor.recover(ctx, request, target, release, deployment, reason, errors.New(message))
	}
	appendStep(&release, "check-current", portainer.PlatformReleaseStepStatusSucceeded, "", "Formal container health check passed.", stepStart, executor.unixNow(), switchResult.CurrentRuntimeRef)

	if executor.gatewayCutover != nil {
		stepStart = executor.unixNow()
		gatewaySnapshot, err := executor.gatewayCutover.Cutover(ctx, request, release)
		if err != nil {
			message := safeReleaseFailureMessage(ReleaseFailureReasonGatewayCutoverFailed)
			appendStep(&release, "apply-gateway", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonGatewayCutoverFailed, message, stepStart, executor.unixNow(), switchResult.CurrentRuntimeRef)
			return executor.recover(ctx, request, target, release, deployment, ReleaseFailureReasonGatewayCutoverFailed, errors.New(message))
		}
		release.GatewaySnapshot = gatewaySnapshot
		if gatewaySnapshot.ConfigHash != "" {
			appendStep(&release, "apply-gateway", portainer.PlatformReleaseStepStatusSucceeded, "", "Gateway configuration applied.", stepStart, executor.unixNow(), switchResult.CurrentRuntimeRef)
		}
	}

	now = executor.unixNow()
	release.Status = portainer.PlatformReleaseStatusSucceeded
	release.FailureReason = ""
	release.ManualActionRequired = false
	release.ResolutionAction = portainer.PlatformReleaseResolutionNone
	release.FinishedAt = now
	release.LeaseExpiresAt = 0
	release.LeaseOwner = ""

	deployment.CurrentServingReleaseID = release.ID
	deployment.CurrentArtifactID = request.Artifact.ID
	deployment.CurrentRuntimeRef = switchResult.CurrentRuntimeRef
	deployment.CurrentImage = request.Artifact.ImageRef
	deployment.LastDeployedSpecRevision = deployment.SpecRevision
	deployment.LastDeployedAt = now
	deployment.DriftStatus = portainer.PlatformDeploymentDriftNone
	deployment.PlatformLifecycle.ResourceVersion++
	deployment.PlatformLifecycle.UpdatedAt = now

	return ReleaseExecutionResult{Release: release, Deployment: &deployment}, nil
}

func (executor *SingleTargetExecutor) recover(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, release portainer.PlatformRelease, deployment portainer.PlatformServiceDeployment, reason string, cause error) (ReleaseExecutionResult, error) {
	release.Status = portainer.PlatformReleaseStatusRecovering
	reportReleaseProgress(request, release)
	stepStart := executor.unixNow()
	if err := executor.driver.Recover(ctx, request, target); err != nil {
		message := safeReleaseFailureMessage(ReleaseFailureReasonRecoveryFailed)
		appendStep(&release, "recover-previous", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonRecoveryFailed, message, stepStart, executor.unixNow(), request.Deployment.CurrentRuntimeRef)
		release.Status = portainer.PlatformReleaseStatusRecoveryFailed
		release.FailureReason = ReleaseFailureReasonRecoveryFailed
		release.ManualActionRequired = true
		release.FinishedAt = executor.unixNow()
		return ReleaseExecutionResult{Release: release}, nil
	}

	appendStep(&release, "recover-previous", portainer.PlatformReleaseStepStatusSucceeded, "", "Previous runtime restored.", stepStart, executor.unixNow(), request.Deployment.CurrentRuntimeRef)
	failRelease(&release, reason, cause.Error(), executor.unixNow())
	deployment.DriftStatus = portainer.PlatformDeploymentDriftNone

	return ReleaseExecutionResult{Release: release}, nil
}

// RetryRecovery 用于人工处置 recovery-failed/interrupted 发布时重试恢复上一版运行时；
// 它不把失败发布改写为成功发布，只在恢复成功后释放锁，让服务回到旧版本继续服务。
func (executor *SingleTargetExecutor) RetryRecovery(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error) {
	release := request.Release
	deployment := request.Deployment
	now := executor.unixNow()

	release.Status = portainer.PlatformReleaseStatusRecovering
	release.ManualActionRequired = false
	release.LeaseOwner = "platform-single-target-executor"
	release.LeaseExpiresAt = now + 15*60

	if executor.driver == nil {
		markRecoveryRetryFailed(&release, ReleaseFailureReasonExecutorUnavailable, "Docker release executor is not configured.", now)
		return ReleaseExecutionResult{Release: release}, nil
	}

	target, err := selectSingleWorkloadTarget(request.Environment)
	if err != nil {
		release.TargetSnapshot = targetSnapshotFromTarget(target, request.Deployment.DesiredSpec.Runtime.RuntimeDriver)
		reason := failureReasonForError(err)
		markRecoveryRetryFailed(&release, reason, safeReleaseFailureMessage(reason), now)
		return ReleaseExecutionResult{Release: release}, nil
	}
	release.TargetSnapshot = targetSnapshotFromTarget(target, request.Deployment.DesiredSpec.Runtime.RuntimeDriver)

	// 恢复重试只负责把上一版运行时重新拉起；原发布仍然按失败态收口，
	// 这样锁会释放，但不会误把这次失败发布记录成一次成功上线。
	stepStart := executor.unixNow()
	if err := executor.driver.Recover(ctx, request, target); err != nil {
		appendStep(&release, "retry-recovery", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonRecoveryFailed, safeReleaseFailureMessage(ReleaseFailureReasonRecoveryFailed), stepStart, executor.unixNow(), request.Deployment.CurrentRuntimeRef)
		release.Status = portainer.PlatformReleaseStatusRecoveryFailed
		release.FailureReason = ReleaseFailureReasonRecoveryFailed
		release.ManualActionRequired = true
		release.FinishedAt = executor.unixNow()
		return ReleaseExecutionResult{Release: release}, nil
	}

	now = executor.unixNow()
	appendStep(&release, "retry-recovery", portainer.PlatformReleaseStepStatusSucceeded, "", "Previous runtime restored by manual retry.", stepStart, now, request.Deployment.CurrentRuntimeRef)
	release.Status = portainer.PlatformReleaseStatusFailed
	release.ManualActionRequired = false
	release.FinishedAt = now
	release.LeaseOwner = ""
	release.LeaseExpiresAt = 0
	deployment.DriftStatus = portainer.PlatformDeploymentDriftNone
	deployment.UpdatedAt = now
	deployment.ResourceVersion++

	return ReleaseExecutionResult{Release: release, Deployment: &deployment}, nil
}

// CleanupRuntime 清理发布失败后快照里记录的孤儿运行时资源；
// 当前仍被 ServiceDeployment 引用的 runtime 会被跳过，避免手工处置误删正在服务的容器。
func (executor *SingleTargetExecutor) CleanupRuntime(ctx context.Context, request ReleaseCleanupRequest) (ReleaseCleanupResult, error) {
	release := request.Release
	now := executor.unixNow()
	stepStart := now
	refs := cleanupRuntimeRefs(release, request.Deployment.CurrentRuntimeRef)

	if executor.driver == nil {
		appendStep(&release, "cleanup-runtime", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonExecutorUnavailable, "Docker release executor is not configured.", stepStart, now, portainer.RuntimeRef{})
		release.ManualActionRequired = true
		return ReleaseCleanupResult{Release: release, FailedRuntimeRefs: refs}, nil
	}
	if len(refs) == 0 {
		appendStep(&release, "cleanup-runtime", portainer.PlatformReleaseStepStatusSkipped, "", "No orphan runtime references require cleanup.", stepStart, now, portainer.RuntimeRef{})
		return ReleaseCleanupResult{Release: release}, nil
	}

	deletedRefs := make([]portainer.RuntimeRef, 0, len(refs))
	failedRefs := make([]portainer.RuntimeRef, 0)
	for _, ref := range refs {
		// 清理动作逐个执行并记录失败集合，避免一个孤儿容器删除失败时吞掉其它可清理对象。
		if err := executor.driver.DeleteRuntime(ctx, ref); err != nil {
			failedRefs = append(failedRefs, ref)
			continue
		}
		deletedRefs = append(deletedRefs, ref)
	}

	now = executor.unixNow()
	if len(failedRefs) > 0 {
		appendStep(&release, "cleanup-runtime", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonRuntimeCleanupFailed, "One or more runtime resources failed to clean up.", stepStart, now, failedRefs[0])
		release.ManualActionRequired = true
		return ReleaseCleanupResult{Release: release, DeletedRuntimeRefs: deletedRefs, FailedRuntimeRefs: failedRefs}, nil
	}

	appendStep(&release, "cleanup-runtime", portainer.PlatformReleaseStepStatusSucceeded, "", "Orphan runtime resources cleaned up.", stepStart, now, portainer.RuntimeRef{})

	return ReleaseCleanupResult{Release: release, DeletedRuntimeRefs: deletedRefs}, nil
}

func markRecoveryRetryFailed(release *portainer.PlatformRelease, reason string, message string, now int64) {
	appendStep(release, "retry-recovery", portainer.PlatformReleaseStepStatusFailed, reason, message, now, now, release.RuntimeSnapshot.PreviousRuntimeRef)
	release.Status = portainer.PlatformReleaseStatusRecoveryFailed
	release.FailureReason = reason
	release.HealthCheckResult.ErrorMessage = message
	release.ManualActionRequired = true
	release.FinishedAt = now
}

func cleanupRuntimeRefs(release portainer.PlatformRelease, servingRef portainer.RuntimeRef) []portainer.RuntimeRef {
	refs := make([]portainer.RuntimeRef, 0, 2+len(release.RuntimeSnapshot.RetainedRuntimeRefs))
	seen := map[string]bool{}
	appendCleanable := func(ref portainer.RuntimeRef) {
		if ref.ResourceID == "" || ref.ResourceID == servingRef.ResourceID || seen[ref.ResourceID] {
			return
		}
		seen[ref.ResourceID] = true
		refs = append(refs, ref)
	}

	appendCleanable(release.RuntimeSnapshot.CandidateRuntimeRef)
	appendCleanable(release.RuntimeSnapshot.CurrentRuntimeRef)
	for _, ref := range release.RuntimeSnapshot.RetainedRuntimeRefs {
		appendCleanable(ref)
	}

	return refs
}

func (executor *SingleTargetExecutor) unixNow() int64 {
	return executor.now().Unix()
}

func selectSingleWorkloadTarget(environment portainer.PlatformEnvironment) (portainer.PlatformDeploymentTarget, error) {
	if environment.TargetMode != "" && environment.TargetMode != portainer.PlatformTargetModeSingle {
		return portainer.PlatformDeploymentTarget{}, codedRuntimeError{reason: ReleaseFailureReasonTargetModeUnsupported, message: "Only single target mode is supported in V0.1."}
	}

	for _, target := range environment.Targets {
		if !target.Enabled {
			continue
		}
		if target.Role != "" && target.Role != portainer.PlatformDeploymentTargetRoleWorkload {
			continue
		}
		if target.EndpointID == 0 {
			continue
		}

		return target, nil
	}

	return portainer.PlatformDeploymentTarget{}, codedRuntimeError{reason: ReleaseFailureReasonTargetNotConfigured, message: "Environment has no enabled workload target."}
}

func targetSnapshotFromTarget(target portainer.PlatformDeploymentTarget, driver portainer.PlatformRuntimeDriver) portainer.PlatformTargetSnapshot {
	return portainer.PlatformTargetSnapshot{
		TargetMode:    portainer.PlatformTargetModeSingle,
		EndpointID:    target.EndpointID,
		NodeName:      target.NodeName,
		HostAddress:   target.HostAddress,
		RuntimeDriver: driver,
		ExecutorMode:  portainer.PlatformExecutorModeSingle,
	}
}

func appendStep(release *portainer.PlatformRelease, name string, status portainer.PlatformReleaseStepStatus, reason string, message string, startedAt int64, finishedAt int64, runtimeRef portainer.RuntimeRef) {
	release.Steps = append(release.Steps, portainer.PlatformReleaseStep{
		Name:       name,
		Status:     status,
		Reason:     reason,
		Message:    message,
		RuntimeRef: runtimeRef,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	})
}

func failRelease(release *portainer.PlatformRelease, reason string, message string, now int64) {
	release.Status = portainer.PlatformReleaseStatusFailed
	release.FailureReason = reason
	release.HealthCheckResult.ErrorMessage = message
	if release.HealthCheckResult.Status == "" {
		release.HealthCheckResult.Status = portainer.PlatformHealthCheckStatusFailed
	}
	release.FinishedAt = now
	release.LeaseOwner = ""
	release.LeaseExpiresAt = 0
}

// safeReleaseFailureMessage 只将固定的、可国际化替换的诊断文本写入 Release；
// Docker、registry 与健康检查驱动的原始错误可能包含凭据、签名地址或宿主机路径，不能进入 BoltDB 或 API 响应。
func safeReleaseFailureMessage(reason string) string {
	switch reason {
	case ReleaseFailureReasonExecutorUnavailable:
		return "Release executor is unavailable."
	case ReleaseFailureReasonTargetNotConfigured:
		return "No eligible deployment target is configured."
	case ReleaseFailureReasonTargetModeUnsupported:
		return "The deployment target mode is not supported."
	case ReleaseFailureReasonProductionRequiresVerify:
		return "Production releases require verified health checks."
	case ReleaseFailureReasonImagePullFailed:
		return "The image could not be pulled on the target endpoint."
	case ReleaseFailureReasonCandidateStartFailed:
		return "The candidate runtime could not be started."
	case ReleaseFailureReasonCandidateHealthFailed, ReleaseFailureReasonHealthcheckFailed, ReleaseFailureReasonHealthcheckHostUnreachable:
		return "The candidate health check did not pass."
	case ReleaseFailureReasonCandidateCleanupFailed, ReleaseFailureReasonRuntimeCleanupFailed:
		return "A temporary runtime resource could not be cleaned up."
	case ReleaseFailureReasonSwitchFailed:
		return "The formal runtime switch did not complete."
	case ReleaseFailureReasonFinalHealthFailed:
		return "The formal runtime health check did not pass."
	case ReleaseFailureReasonRecoveryFailed:
		return "The previous runtime could not be recovered automatically."
	case ReleaseFailureReasonGatewayCutoverFailed:
		return "The gateway configuration could not be applied; the previous runtime was restored."
	default:
		return "The runtime operation did not complete."
	}
}

type codedRuntimeError struct {
	reason  string
	message string
}

func (err codedRuntimeError) Error() string {
	return err.message
}

func failureReasonForError(err error) string {
	if err == nil {
		return ""
	}

	var coded codedRuntimeError
	if errors.As(err, &coded) {
		return coded.reason
	}

	return ReleaseFailureReasonRuntimeOperationFailed
}
