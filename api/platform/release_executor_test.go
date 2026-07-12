package platform

import (
	"context"
	"errors"
	"testing"
	"time"

	portainer "github.com/portainer/portainer/api"

	"github.com/stretchr/testify/require"
)

type fakeRuntimeDriver struct {
	calls              []string
	pullErr            error
	startErr           error
	candidateHealthErr error
	deleteErr          error
	switchErr          error
	finalHealthErr     error
	recoverErr         error
	validateCalls      int
}

func (driver *fakeRuntimeDriver) PullImage(context.Context, ReleaseExecutionRequest, portainer.PlatformDeploymentTarget) error {
	driver.calls = append(driver.calls, "pull")
	return driver.pullErr
}

func (driver *fakeRuntimeDriver) StartCandidate(context.Context, ReleaseExecutionRequest, portainer.PlatformDeploymentTarget) (portainer.RuntimeRef, []portainer.PlatformPublishedPort, error) {
	driver.calls = append(driver.calls, "start-candidate")
	if driver.startErr != nil {
		return portainer.RuntimeRef{}, nil, driver.startErr
	}

	ref := portainer.RuntimeRef{
		DriverID:     portainer.PlatformRuntimeDriverDockerContainer,
		EndpointID:   1,
		ResourceType: portainer.PlatformRuntimeResourceContainer,
		ResourceID:   "candidate",
		Name:         "candidate",
	}

	return ref, []portainer.PlatformPublishedPort{{Name: "http", ContainerPort: 80, HostPort: 32768, Protocol: portainer.PlatformPortProtocolTCP}}, nil
}

func (driver *fakeRuntimeDriver) ValidateRuntime(_ context.Context, _ ReleaseExecutionRequest, _ portainer.PlatformDeploymentTarget, runtimeRef portainer.RuntimeRef, _ []portainer.PlatformPublishedPort) (portainer.PlatformHealthCheckResult, error) {
	driver.validateCalls++
	driver.calls = append(driver.calls, "validate-"+runtimeRef.ResourceID)
	now := time.Now().Unix()
	result := portainer.PlatformHealthCheckResult{
		Level:      portainer.PlatformHealthVerificationVerified,
		Type:       portainer.PlatformHealthCheckTypeHTTP,
		Status:     portainer.PlatformHealthCheckStatusPassed,
		Target:     "http://127.0.0.1/health",
		StartedAt:  now,
		FinishedAt: now,
	}

	if driver.validateCalls == 1 && driver.candidateHealthErr != nil {
		result.Status = portainer.PlatformHealthCheckStatusFailed
		result.ErrorMessage = driver.candidateHealthErr.Error()
		return result, driver.candidateHealthErr
	}
	if driver.validateCalls == 2 && driver.finalHealthErr != nil {
		result.Status = portainer.PlatformHealthCheckStatusFailed
		result.ErrorMessage = driver.finalHealthErr.Error()
		return result, driver.finalHealthErr
	}

	return result, nil
}

func (driver *fakeRuntimeDriver) DeleteRuntime(_ context.Context, runtimeRef portainer.RuntimeRef) error {
	driver.calls = append(driver.calls, "delete-"+runtimeRef.ResourceID)
	return driver.deleteErr
}

func (driver *fakeRuntimeDriver) Switch(_ context.Context, _ ReleaseExecutionRequest, _ portainer.PlatformDeploymentTarget, _ CandidateValidationSnapshot) (RuntimeSwitchResult, error) {
	driver.calls = append(driver.calls, "switch")
	if driver.switchErr != nil {
		return RuntimeSwitchResult{}, driver.switchErr
	}

	return RuntimeSwitchResult{
		CurrentRuntimeRef: portainer.RuntimeRef{
			DriverID:     portainer.PlatformRuntimeDriverDockerContainer,
			EndpointID:   1,
			ResourceType: portainer.PlatformRuntimeResourceContainer,
			ResourceID:   "current",
			Name:         "current",
		},
		PublishedPorts:      []portainer.PlatformPublishedPort{{Name: "http", ContainerPort: 80, HostPort: 18080, Protocol: portainer.PlatformPortProtocolTCP}},
		RetainedRuntimeRefs: []portainer.RuntimeRef{{ResourceID: "previous"}},
	}, nil
}

func (driver *fakeRuntimeDriver) Recover(context.Context, ReleaseExecutionRequest, portainer.PlatformDeploymentTarget) error {
	driver.calls = append(driver.calls, "recover")
	return driver.recoverErr
}

func TestSingleTargetExecutorSucceeds(t *testing.T) {
	driver := &fakeRuntimeDriver{}
	executor := NewSingleTargetExecutor(driver)

	result, err := executor.Execute(context.Background(), sampleReleaseExecutionRequest())
	require.NoError(t, err)

	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, result.Release.Status)
	require.Equal(t, portainer.PlatformHealthCheckStatusPassed, result.Release.HealthCheckResult.Status)
	require.Equal(t, "candidate", result.Release.RuntimeSnapshot.CandidateRuntimeRef.ResourceID)
	require.Equal(t, "current", result.Release.RuntimeSnapshot.CurrentRuntimeRef.ResourceID)
	require.NotNil(t, result.Deployment)
	require.Equal(t, result.Release.ID, result.Deployment.CurrentServingReleaseID)
	require.Equal(t, result.Release.ArtifactID, result.Deployment.CurrentArtifactID)
	require.Equal(t, "registry.example.com/orders-api:1.0.0", result.Deployment.CurrentImage)
	require.Equal(t, []string{"pull", "start-candidate", "validate-candidate", "delete-candidate", "switch", "validate-current"}, driver.calls)
}

func TestMultiTargetExecutorRecordsEachWorkloadResult(t *testing.T) {
	driver := &fakeRuntimeDriver{}
	request := sampleReleaseExecutionRequest()
	request.Environment.TargetMode = portainer.PlatformTargetModeMulti
	request.Environment.Targets = []portainer.PlatformDeploymentTarget{
		{EndpointID: 2, NodeName: "worker-b", HostAddress: "10.0.0.12", Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true},
		{EndpointID: 1, NodeName: "worker-a", HostAddress: "10.0.0.11", Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true},
		{EndpointID: 3, NodeName: "gateway", Role: portainer.PlatformDeploymentTargetRoleGateway, Enabled: true},
	}

	result, err := NewMultiTargetExecutor(driver).Execute(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, result.Release.Status)
	require.Len(t, result.Release.TargetResults, 2)
	require.Equal(t, portainer.EndpointID(1), result.Release.TargetResults[0].EndpointID)
	require.Equal(t, portainer.PlatformReleaseTargetStatusSucceeded, result.Release.TargetResults[0].Status)
	require.Equal(t, 0, result.Release.TargetResults[0].BatchIndex)
	require.Equal(t, portainer.EndpointID(2), result.Release.TargetResults[1].EndpointID)
	require.Equal(t, 1, result.Release.TargetResults[1].BatchIndex)
	require.Equal(t, portainer.PlatformTargetModeMulti, result.Release.TargetSnapshot.TargetMode)
	require.Len(t, result.Release.TargetSnapshots, 2)
	require.Len(t, result.Release.BatchSnapshots, 2)
	require.NotNil(t, result.Deployment)
	require.Len(t, result.Deployment.CurrentTargetRuntimeRefs, 2)
}

func TestMultiTargetExecutorSkipsRemainingTargetsAfterFailure(t *testing.T) {
	driver := &fakeRuntimeDriver{candidateHealthErr: errors.New("candidate failed")}
	request := sampleReleaseExecutionRequest()
	request.Environment.TargetMode = portainer.PlatformTargetModeMulti
	request.Environment.Targets = []portainer.PlatformDeploymentTarget{
		{EndpointID: 1, NodeName: "worker-a", HostAddress: "10.0.0.11", Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true},
		{EndpointID: 2, NodeName: "worker-b", HostAddress: "10.0.0.12", Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true},
	}

	result, err := NewMultiTargetExecutor(driver).Execute(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformReleaseStatusFailed, result.Release.Status)
	require.Len(t, result.Release.TargetResults, 2)
	require.Equal(t, portainer.PlatformReleaseTargetStatusFailed, result.Release.TargetResults[0].Status)
	require.Equal(t, portainer.PlatformReleaseTargetStatusSkipped, result.Release.TargetResults[1].Status)
	require.Equal(t, "BATCH_PAUSED_AFTER_TARGET_FAILURE", result.Release.TargetResults[1].Reason)
}

func TestSingleTargetExecutorRecoversPreviousWhenGatewayCutoverFails(t *testing.T) {
	driver := &fakeRuntimeDriver{}
	executor := NewSingleTargetExecutor(driver).WithGatewayCutover(fakeGatewayCutover{err: errors.New("gateway reload failed")})

	result, err := executor.Execute(context.Background(), sampleReleaseExecutionRequest())
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformReleaseStatusFailed, result.Release.Status)
	require.Equal(t, ReleaseFailureReasonGatewayCutoverFailed, result.Release.FailureReason)
	require.Nil(t, result.Deployment)
	require.Contains(t, driver.calls, "recover")
	require.Equal(t, "apply-gateway", result.Release.Steps[len(result.Release.Steps)-2].Name)
}

type fakeGatewayCutover struct{ err error }

func (cutover fakeGatewayCutover) Cutover(context.Context, ReleaseExecutionRequest, portainer.PlatformRelease) (portainer.PlatformGatewaySnapshot, error) {
	if cutover.err != nil {
		return portainer.PlatformGatewaySnapshot{}, cutover.err
	}
	return portainer.PlatformGatewaySnapshot{ConfigHash: "abc"}, nil
}

func TestSingleTargetExecutorReportsStructuredProgress(t *testing.T) {
	driver := &fakeRuntimeDriver{}
	executor := NewSingleTargetExecutor(driver)
	request := sampleReleaseExecutionRequest()
	progress := make([]ReleaseExecutionResult, 0, 8)
	request.Progress = func(result ReleaseExecutionResult) {
		progress = append(progress, result)
	}

	_, err := executor.Execute(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, []portainer.PlatformReleaseStatus{
		portainer.PlatformReleaseStatusValidating,
		portainer.PlatformReleaseStatusPulling,
		portainer.PlatformReleaseStatusPreparing,
		portainer.PlatformReleaseStatusCandidateStarting,
		portainer.PlatformReleaseStatusCandidateChecking,
		portainer.PlatformReleaseStatusCandidateChecking,
		portainer.PlatformReleaseStatusSwitching,
		portainer.PlatformReleaseStatusFinalChecking,
	}, releaseStatuses(progress))
	require.Equal(t, "switch-runtime", progress[len(progress)-1].Release.Steps[len(progress[len(progress)-1].Release.Steps)-1].Name)
}

func releaseStatuses(progress []ReleaseExecutionResult) []portainer.PlatformReleaseStatus {
	statuses := make([]portainer.PlatformReleaseStatus, 0, len(progress))
	for _, item := range progress {
		statuses = append(statuses, item.Release.Status)
	}
	return statuses
}

func TestSingleTargetExecutorRecoversPreviousWhenSwitchFails(t *testing.T) {
	driver := &fakeRuntimeDriver{switchErr: errors.New("port is already allocated")}
	executor := NewSingleTargetExecutor(driver)

	result, err := executor.Execute(context.Background(), sampleReleaseExecutionRequest())
	require.NoError(t, err)

	require.Equal(t, portainer.PlatformReleaseStatusFailed, result.Release.Status)
	require.Equal(t, ReleaseFailureReasonSwitchFailed, result.Release.FailureReason)
	require.False(t, result.Release.ManualActionRequired)
	require.Nil(t, result.Deployment)
	require.Contains(t, driver.calls, "recover")
}

func TestSingleTargetExecutorKeepsLockWhenRecoveryFails(t *testing.T) {
	driver := &fakeRuntimeDriver{
		switchErr:  errors.New("start current failed"),
		recoverErr: errors.New("previous container is missing"),
	}
	executor := NewSingleTargetExecutor(driver)

	result, err := executor.Execute(context.Background(), sampleReleaseExecutionRequest())
	require.NoError(t, err)

	require.Equal(t, portainer.PlatformReleaseStatusRecoveryFailed, result.Release.Status)
	require.Equal(t, ReleaseFailureReasonRecoveryFailed, result.Release.FailureReason)
	require.True(t, result.Release.ManualActionRequired)
	require.Nil(t, result.Deployment)
}

func TestSingleTargetExecutorDeletesFailedCurrentBeforeRecovering(t *testing.T) {
	driver := &fakeRuntimeDriver{finalHealthErr: errors.New("final health failed")}
	executor := NewSingleTargetExecutor(driver)

	result, err := executor.Execute(context.Background(), sampleReleaseExecutionRequest())
	require.NoError(t, err)

	require.Equal(t, portainer.PlatformReleaseStatusFailed, result.Release.Status)
	require.Equal(t, safeReleaseFailureMessage(ReleaseFailureReasonFinalHealthFailed), result.Release.HealthCheckResult.ErrorMessage)
	require.Nil(t, result.Deployment)
	require.Contains(t, driver.calls, "delete-current")
	require.Contains(t, driver.calls, "recover")
	require.Less(t, indexOfCall(driver.calls, "delete-current"), indexOfCall(driver.calls, "recover"))
}

func TestSingleTargetExecutorRetryRecoveryRestoresPreviousAndReleasesManualAction(t *testing.T) {
	driver := &fakeRuntimeDriver{}
	executor := NewSingleTargetExecutor(driver)
	request := sampleReleaseExecutionRequest()
	request.Release.Status = portainer.PlatformReleaseStatusRecoveryFailed
	request.Release.FailureReason = ReleaseFailureReasonRecoveryFailed
	request.Release.ManualActionRequired = true

	result, err := executor.RetryRecovery(context.Background(), request)
	require.NoError(t, err)

	require.Equal(t, portainer.PlatformReleaseStatusFailed, result.Release.Status)
	require.False(t, result.Release.ManualActionRequired)
	require.Empty(t, result.Release.LeaseOwner)
	require.Zero(t, result.Release.LeaseExpiresAt)
	require.NotNil(t, result.Deployment)
	require.Equal(t, portainer.PlatformDeploymentDriftNone, result.Deployment.DriftStatus)
	require.Contains(t, driver.calls, "recover")
	require.Equal(t, "retry-recovery", result.Release.Steps[len(result.Release.Steps)-1].Name)
	require.Equal(t, portainer.PlatformReleaseStepStatusSucceeded, result.Release.Steps[len(result.Release.Steps)-1].Status)
}

func TestSingleTargetExecutorCleanupRuntimeSkipsServingRuntime(t *testing.T) {
	driver := &fakeRuntimeDriver{}
	executor := NewSingleTargetExecutor(driver)
	request := sampleReleaseExecutionRequest()
	request.Deployment.CurrentRuntimeRef = portainer.RuntimeRef{ResourceID: "serving"}
	request.Release.RuntimeSnapshot = portainer.PlatformRuntimeSnapshot{
		CandidateRuntimeRef: portainer.RuntimeRef{ResourceID: "candidate"},
		CurrentRuntimeRef:   portainer.RuntimeRef{ResourceID: "serving"},
		RetainedRuntimeRefs: []portainer.RuntimeRef{
			{ResourceID: "retained"},
			{ResourceID: "candidate"},
		},
	}

	result, err := executor.CleanupRuntime(context.Background(), ReleaseCleanupRequest{
		Release:    request.Release,
		Deployment: request.Deployment,
	})
	require.NoError(t, err)

	require.Equal(t, []string{"delete-candidate", "delete-retained"}, driver.calls)
	require.Len(t, result.DeletedRuntimeRefs, 2)
	require.Empty(t, result.FailedRuntimeRefs)
	require.Equal(t, "cleanup-runtime", result.Release.Steps[len(result.Release.Steps)-1].Name)
	require.Equal(t, portainer.PlatformReleaseStepStatusSucceeded, result.Release.Steps[len(result.Release.Steps)-1].Status)
}

func sampleReleaseExecutionRequest() ReleaseExecutionRequest {
	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/orders-api:1.0.0"
	spec.Ports = []portainer.PlatformPortSpec{{Name: "http", ContainerPort: 80, HostPort: 18080, Protocol: portainer.PlatformPortProtocolTCP}}
	spec.HealthCheck.Path = "/health"
	spec.HealthCheck.Port = 80

	return ReleaseExecutionRequest{
		Project: portainer.PlatformProject{
			ID:   1,
			Name: "Customer A",
			Slug: "customer-a",
		},
		Environment: portainer.PlatformEnvironment{
			ID:            1,
			ProjectID:     1,
			Name:          "Dev",
			Slug:          "dev",
			Type:          portainer.PlatformEnvironmentTypeDev,
			TargetMode:    portainer.PlatformTargetModeSingle,
			Targets:       []portainer.PlatformDeploymentTarget{{EndpointID: 1, Role: portainer.PlatformDeploymentTargetRoleWorkload, HostAddress: "127.0.0.1", Enabled: true}},
			ReleasePolicy: portainer.NewPlatformReleasePolicy(),
		},
		Application: portainer.PlatformApplication{
			ID:        1,
			ProjectID: 1,
			Name:      "Orders",
			Slug:      "orders",
		},
		ServiceDefinition: portainer.PlatformServiceDefinition{
			ID:            1,
			ProjectID:     1,
			ApplicationID: 1,
			Name:          "Orders API",
			Slug:          "orders-api",
			Type:          portainer.PlatformServiceTypeBackend,
		},
		Deployment: portainer.PlatformServiceDeployment{
			ID:                  1,
			ProjectID:           1,
			EnvironmentID:       1,
			ApplicationID:       1,
			ServiceDefinitionID: 1,
			DesiredSpec:         spec,
			SpecRevision:        1,
			CurrentRuntimeRef:   portainer.RuntimeRef{ResourceID: "previous"},
			DriftStatus:         portainer.PlatformDeploymentDriftNone,
			PlatformLifecycle:   portainer.NewPlatformLifecycle(),
		},
		Artifact: portainer.PlatformArtifact{
			ID:                  1,
			ProjectID:           1,
			ApplicationID:       1,
			ServiceDefinitionID: 1,
			Name:                "orders-api",
			Version:             "1.0.0",
			Type:                portainer.PlatformArtifactTypeImage,
			SourceType:          portainer.PlatformArtifactSourceImageReference,
			ImageRef:            "registry.example.com/orders-api:1.0.0",
			Traceability:        portainer.PlatformTraceabilityWeak,
		},
		Release: portainer.PlatformRelease{
			ID:                   1,
			ProjectID:            1,
			EnvironmentID:        1,
			ApplicationID:        1,
			ServiceDefinitionID:  1,
			ServiceDeploymentID:  1,
			ArtifactID:           1,
			Version:              "1.0.0",
			TriggerType:          portainer.PlatformReleaseTriggerDeploy,
			Strategy:             portainer.NewPlatformReleaseStrategy(),
			Status:               portainer.PlatformReleaseStatusQueued,
			ExpectedSpecRevision: 1,
			Image:                "registry.example.com/orders-api:1.0.0",
			ArtifactSnapshot:     portainer.PlatformArtifactSnapshot{ArtifactID: 1, ImageRef: "registry.example.com/orders-api:1.0.0"},
			ConfigSnapshot:       portainer.PlatformServiceConfigSnapshot{SpecRevision: 1, DesiredSpecSnapshot: spec},
			ResolutionAction:     portainer.PlatformReleaseResolutionNone,
			OperatorUserID:       1,
			QueueExpiresAt:       time.Now().Unix() + 600,
		},
	}
}

func indexOfCall(calls []string, expected string) int {
	for i, call := range calls {
		if call == expected {
			return i
		}
	}

	return -1
}
