package platform

import (
	"context"
	"errors"
	"fmt"
	"time"

	portainer "github.com/portainer/portainer/api"
)

const (
	ReleaseFailureReasonExecutorUnavailable        = "EXECUTOR_UNAVAILABLE"
	ReleaseFailureReasonTargetNotConfigured        = "TARGET_NOT_CONFIGURED"
	ReleaseFailureReasonTargetModeUnsupported      = "TARGET_MODE_UNSUPPORTED"
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
)

// ReleaseExecutor owns the runtime side of a platform release. The HTTP layer
// persists the release and lock first, then delegates Docker/Agent work through
// this interface so later workerization does not change the control-plane API.
type ReleaseExecutor interface {
	Execute(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error)
}

type ReleaseExecutionRequest struct {
	Project           portainer.PlatformProject
	Environment       portainer.PlatformEnvironment
	Application       portainer.PlatformApplication
	ServiceDefinition portainer.PlatformServiceDefinition
	Deployment        portainer.PlatformServiceDeployment
	Artifact          portainer.PlatformArtifact
	Release           portainer.PlatformRelease
}

type ReleaseExecutionResult struct {
	Release    portainer.PlatformRelease
	Deployment *portainer.PlatformServiceDeployment
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
	driver RuntimeDriver
	now    func() time.Time
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
		failRelease(&release, failureReasonForError(err), err.Error(), now)
		return ReleaseExecutionResult{Release: release}, nil
	}
	release.TargetSnapshot = targetSnapshotFromTarget(target, request.Deployment.DesiredSpec.Runtime.RuntimeDriver)

	if request.Environment.IsProduction && request.Deployment.DesiredSpec.HealthCheck.VerificationLevel != portainer.PlatformHealthVerificationVerified {
		failRelease(&release, ReleaseFailureReasonProductionRequiresVerify, "Production releases require verified health checks in V0.1.", now)
		return ReleaseExecutionResult{Release: release}, nil
	}

	appendStep(&release, "validate", portainer.PlatformReleaseStepStatusSucceeded, "", "Release references and V0.1 target are valid.", now, executor.unixNow(), portainer.RuntimeRef{})

	release.Status = portainer.PlatformReleaseStatusPulling
	stepStart := executor.unixNow()
	if err := executor.driver.PullImage(ctx, request, target); err != nil {
		appendStep(&release, "pull-image", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonImagePullFailed, err.Error(), stepStart, executor.unixNow(), portainer.RuntimeRef{})
		failRelease(&release, ReleaseFailureReasonImagePullFailed, err.Error(), executor.unixNow())
		return ReleaseExecutionResult{Release: release}, nil
	}
	appendStep(&release, "pull-image", portainer.PlatformReleaseStepStatusSucceeded, "", "Image pulled or already present.", stepStart, executor.unixNow(), portainer.RuntimeRef{})

	release.Status = portainer.PlatformReleaseStatusPreparing
	appendStep(&release, "prepare-runtime", portainer.PlatformReleaseStepStatusSucceeded, "", "Runtime configuration prepared.", executor.unixNow(), executor.unixNow(), portainer.RuntimeRef{})

	release.Status = portainer.PlatformReleaseStatusCandidateStarting
	stepStart = executor.unixNow()
	candidateRef, candidatePorts, err := executor.driver.StartCandidate(ctx, request, target)
	if err != nil {
		appendStep(&release, "start-candidate", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonCandidateStartFailed, err.Error(), stepStart, executor.unixNow(), portainer.RuntimeRef{})
		failRelease(&release, ReleaseFailureReasonCandidateStartFailed, err.Error(), executor.unixNow())
		return ReleaseExecutionResult{Release: release}, nil
	}
	release.RuntimeSnapshot.CandidateRuntimeRef = candidateRef
	appendStep(&release, "start-candidate", portainer.PlatformReleaseStepStatusSucceeded, "", "Candidate container started with random published ports.", stepStart, executor.unixNow(), candidateRef)

	release.Status = portainer.PlatformReleaseStatusCandidateChecking
	stepStart = executor.unixNow()
	health, err := executor.driver.ValidateRuntime(ctx, request, target, candidateRef, candidatePorts)
	release.HealthCheckResult = health
	if err != nil || health.Status == portainer.PlatformHealthCheckStatusFailed {
		reason := ReleaseFailureReasonCandidateHealthFailed
		message := health.ErrorMessage
		if err != nil {
			message = err.Error()
			reason = failureReasonForError(err)
		}
		appendStep(&release, "check-candidate", portainer.PlatformReleaseStepStatusFailed, reason, message, stepStart, executor.unixNow(), candidateRef)
		_ = executor.driver.DeleteRuntime(ctx, candidateRef)
		failRelease(&release, reason, message, executor.unixNow())
		return ReleaseExecutionResult{Release: release}, nil
	}
	appendStep(&release, "check-candidate", portainer.PlatformReleaseStepStatusSucceeded, "", "Candidate health check passed.", stepStart, executor.unixNow(), candidateRef)

	stepStart = executor.unixNow()
	if err := executor.driver.DeleteRuntime(ctx, candidateRef); err != nil {
		appendStep(&release, "cleanup-candidate", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonCandidateCleanupFailed, err.Error(), stepStart, executor.unixNow(), candidateRef)
		release.ManualActionRequired = true
		failRelease(&release, ReleaseFailureReasonCandidateCleanupFailed, err.Error(), executor.unixNow())
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
	stepStart = executor.unixNow()
	switchResult, err := executor.driver.Switch(ctx, request, target, snapshot)
	if err != nil {
		appendStep(&release, "switch-runtime", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonSwitchFailed, err.Error(), stepStart, executor.unixNow(), portainer.RuntimeRef{})
		return executor.recover(ctx, request, target, release, deployment, ReleaseFailureReasonSwitchFailed, err)
	}
	release.RuntimeSnapshot.PreviousRuntimeRef = request.Deployment.CurrentRuntimeRef
	release.RuntimeSnapshot.CurrentRuntimeRef = switchResult.CurrentRuntimeRef
	release.RuntimeSnapshot.PublishedPorts = switchResult.PublishedPorts
	release.RuntimeSnapshot.RetainedRuntimeRefs = switchResult.RetainedRuntimeRefs
	appendStep(&release, "switch-runtime", portainer.PlatformReleaseStepStatusSucceeded, "", "Formal container switched to desired published ports.", stepStart, executor.unixNow(), switchResult.CurrentRuntimeRef)

	release.Status = portainer.PlatformReleaseStatusFinalChecking
	stepStart = executor.unixNow()
	finalHealth, err := executor.driver.ValidateRuntime(ctx, request, target, switchResult.CurrentRuntimeRef, switchResult.PublishedPorts)
	release.HealthCheckResult = finalHealth
	if err != nil || finalHealth.Status == portainer.PlatformHealthCheckStatusFailed {
		reason := ReleaseFailureReasonFinalHealthFailed
		message := finalHealth.ErrorMessage
		if err != nil {
			message = err.Error()
			reason = failureReasonForError(err)
		}
		appendStep(&release, "check-current", portainer.PlatformReleaseStepStatusFailed, reason, message, stepStart, executor.unixNow(), switchResult.CurrentRuntimeRef)
		_ = executor.driver.DeleteRuntime(ctx, switchResult.CurrentRuntimeRef)
		return executor.recover(ctx, request, target, release, deployment, reason, errors.New(message))
	}
	appendStep(&release, "check-current", portainer.PlatformReleaseStepStatusSucceeded, "", "Formal container health check passed.", stepStart, executor.unixNow(), switchResult.CurrentRuntimeRef)

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
	stepStart := executor.unixNow()
	if err := executor.driver.Recover(ctx, request, target); err != nil {
		appendStep(&release, "recover-previous", portainer.PlatformReleaseStepStatusFailed, ReleaseFailureReasonRecoveryFailed, err.Error(), stepStart, executor.unixNow(), request.Deployment.CurrentRuntimeRef)
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

	return fmt.Sprintf("%s", err)
}
