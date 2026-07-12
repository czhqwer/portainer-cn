package platform

import (
	"context"
	"errors"
	"sort"
	"time"

	portainer "github.com/portainer/portainer/api"
)

// MultiTargetExecutor 在阶段 5.4 将现有单目标受控动作复用于每台 workload 主机。
// 每次调用仍只把一个 target 交给 Docker driver，避免共享全局 RuntimeRef 导致不同主机互相切换容器。
type MultiTargetExecutor struct {
	driver         RuntimeDriver
	now            func() time.Time
	gatewayCutover GatewayCutover
}

func (executor *MultiTargetExecutor) WithGatewayCutover(cutover GatewayCutover) *MultiTargetExecutor {
	executor.gatewayCutover = cutover
	return executor
}

func NewMultiTargetExecutor(driver RuntimeDriver) *MultiTargetExecutor {
	return &MultiTargetExecutor{driver: driver, now: time.Now}
}

func (executor *MultiTargetExecutor) Execute(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error) {
	if request.Environment.TargetMode != portainer.PlatformTargetModeMulti {
		return NewSingleTargetExecutor(executor.driver).Execute(ctx, request)
	}
	if executor.driver == nil {
		release := request.Release
		failRelease(&release, ReleaseFailureReasonExecutorUnavailable, safeReleaseFailureMessage(ReleaseFailureReasonExecutorUnavailable), executor.now().Unix())
		return ReleaseExecutionResult{Release: release}, nil
	}
	targets, err := selectMultiWorkloadTargets(request.Environment)
	if err != nil {
		release := request.Release
		failRelease(&release, ReleaseFailureReasonTargetNotConfigured, safeReleaseFailureMessage(ReleaseFailureReasonTargetNotConfigured), executor.now().Unix())
		return ReleaseExecutionResult{Release: release}, nil
	}

	release := request.Release
	policy := request.Environment.BatchPolicy
	if policy.BatchSize == 0 {
		policy = portainer.NewPlatformBatchPolicy()
	}
	if err := portainer.ValidatePlatformBatchPolicy(policy); err != nil {
		failRelease(&release, ReleaseFailureReasonTargetNotConfigured, safeReleaseFailureMessage(ReleaseFailureReasonTargetNotConfigured), executor.now().Unix())
		return ReleaseExecutionResult{Release: release}, nil
	}
	release.TargetSnapshots = multiTargetSnapshots(targets, request.Deployment.DesiredSpec.Runtime.RuntimeDriver)
	release.BatchPolicySnapshot = policy
	release.BatchSnapshots = multiBatchSnapshots(len(targets), policy.BatchSize)
	if len(release.TargetSnapshots) > 0 {
		release.TargetSnapshot = release.TargetSnapshots[0]
	}
	results := make([]portainer.PlatformReleaseTargetResult, 0, len(targets))
	previous := targetResultByEndpoint(request.Deployment.CurrentTargetRuntimeRefs)
	for index, target := range targets {
		batchIndex := index / policy.BatchSize
		perTargetRequest := request
		perTargetRequest.Environment = request.Environment
		perTargetRequest.Environment.TargetMode = portainer.PlatformTargetModeSingle
		perTargetRequest.Environment.Targets = []portainer.PlatformDeploymentTarget{target}
		perTargetRequest.Progress = nil
		perTargetRequest.Deployment = request.Deployment
		if prior, found := previous[target.EndpointID]; found {
			perTargetRequest.Deployment.CurrentRuntimeRef = prior.RuntimeRef
		}
		result, err := NewSingleTargetExecutor(executor.driver).Execute(ctx, perTargetRequest)
		if err != nil {
			return ReleaseExecutionResult{}, err
		}
		targetResult := portainer.PlatformReleaseTargetResult{EndpointID: target.EndpointID, NodeName: target.NodeName, HostAddress: target.HostAddress, BatchIndex: batchIndex, Status: targetStatusFromRelease(result.Release)}
		if result.Release.Status == portainer.PlatformReleaseStatusSucceeded {
			targetResult.RuntimeRef = result.Release.RuntimeSnapshot.CurrentRuntimeRef
			targetResult.PublishedPorts = result.Release.RuntimeSnapshot.PublishedPorts
		}
		if result.Release.FailureReason != "" {
			targetResult.Reason = result.Release.FailureReason
		}
		results = append(results, targetResult)
		release.Steps = append(release.Steps, result.Release.Steps...)
		if targetResult.Status != portainer.PlatformReleaseTargetStatusSucceeded {
			for _, skippedTarget := range targets[index+1:] {
				results = append(results, portainer.PlatformReleaseTargetResult{EndpointID: skippedTarget.EndpointID, NodeName: skippedTarget.NodeName, HostAddress: skippedTarget.HostAddress, BatchIndex: (index + 1) / policy.BatchSize, Status: portainer.PlatformReleaseTargetStatusSkipped, Reason: "BATCH_PAUSED_AFTER_TARGET_FAILURE"})
			}
			release.TargetResults = results
			return executor.failAndRecover(ctx, request, release, results)
		}
		if (index+1)%policy.BatchSize == 0 && index+1 < len(targets) && policy.IntervalSeconds > 0 {
			if err := waitBatchInterval(ctx, time.Duration(policy.IntervalSeconds)*time.Second); err != nil {
				for _, skippedTarget := range targets[index+1:] {
					results = append(results, portainer.PlatformReleaseTargetResult{EndpointID: skippedTarget.EndpointID, NodeName: skippedTarget.NodeName, HostAddress: skippedTarget.HostAddress, BatchIndex: (index + 1) / policy.BatchSize, Status: portainer.PlatformReleaseTargetStatusSkipped, Reason: "BATCH_CANCELED"})
				}
				release.TargetResults = results
				failRelease(&release, ReleaseFailureReasonRuntimeOperationFailed, safeReleaseFailureMessage(ReleaseFailureReasonRuntimeOperationFailed), executor.now().Unix())
				return ReleaseExecutionResult{Release: release}, nil
			}
		}
	}

	if executor.gatewayCutover != nil {
		snapshot, err := executor.gatewayCutover.Cutover(ctx, request, release)
		if err != nil {
			release.TargetResults = results
			return executor.failAndRecover(ctx, request, release, results)
		}
		release.GatewaySnapshot = snapshot
	}
	now := executor.now().Unix()
	release.Status = portainer.PlatformReleaseStatusSucceeded
	release.FailureReason = ""
	release.TargetResults = results
	release.FinishedAt = now
	release.LeaseOwner, release.LeaseExpiresAt = "", 0
	deployment := request.Deployment
	deployment.CurrentServingReleaseID = release.ID
	deployment.CurrentTargetRuntimeRefs = append([]portainer.PlatformReleaseTargetResult(nil), results...)
	if len(results) > 0 {
		deployment.CurrentRuntimeRef = results[0].RuntimeRef // 保留旧 single 字段，供未迁移调用方读取首个稳定 target。
	}
	deployment.LastDeployedSpecRevision = deployment.SpecRevision
	deployment.LastDeployedAt = now
	deployment.DriftStatus = portainer.PlatformDeploymentDriftNone
	deployment.ResourceVersion++
	deployment.UpdatedAt = now
	return ReleaseExecutionResult{Release: release, Deployment: &deployment}, nil
}

func multiTargetSnapshots(targets []portainer.PlatformDeploymentTarget, driver portainer.PlatformRuntimeDriver) []portainer.PlatformTargetSnapshot {
	snapshots := make([]portainer.PlatformTargetSnapshot, 0, len(targets))
	for _, target := range targets {
		snapshots = append(snapshots, portainer.PlatformTargetSnapshot{TargetMode: portainer.PlatformTargetModeMulti, EndpointID: target.EndpointID, NodeName: target.NodeName, HostAddress: target.HostAddress, RuntimeDriver: driver, ExecutorMode: portainer.PlatformExecutorModeBatch})
	}
	return snapshots
}

func multiBatchSnapshots(targetCount, batchSize int) []portainer.PlatformReleaseBatchSnapshot {
	batches := make([]portainer.PlatformReleaseBatchSnapshot, 0, (targetCount+batchSize-1)/batchSize)
	for start, index := 0, 0; start < targetCount; start, index = start+batchSize, index+1 {
		end := start + batchSize
		if end > targetCount {
			end = targetCount
		}
		indices := make([]int, 0, end-start)
		for current := start; current < end; current++ {
			indices = append(indices, current)
		}
		batches = append(batches, portainer.PlatformReleaseBatchSnapshot{Index: index, TargetIndices: indices})
	}
	return batches
}

func waitBatchInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (executor *MultiTargetExecutor) failAndRecover(ctx context.Context, request ReleaseExecutionRequest, release portainer.PlatformRelease, results []portainer.PlatformReleaseTargetResult) (ReleaseExecutionResult, error) {
	for i := range results {
		if results[i].Status != portainer.PlatformReleaseTargetStatusSucceeded {
			continue
		}
		perTargetRequest := request
		perTargetRequest.Deployment = request.Deployment
		if prior, found := targetResultByEndpoint(request.Deployment.CurrentTargetRuntimeRefs)[results[i].EndpointID]; found {
			perTargetRequest.Deployment.CurrentRuntimeRef = prior.RuntimeRef
		}
		target := portainer.PlatformDeploymentTarget{EndpointID: results[i].EndpointID, NodeName: results[i].NodeName, HostAddress: results[i].HostAddress, Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true}
		if err := executor.driver.Recover(ctx, perTargetRequest, target); err != nil {
			results[i].Status, results[i].Reason = portainer.PlatformReleaseTargetStatusRecoveryFailed, ReleaseFailureReasonRecoveryFailed
			release.Status, release.FailureReason, release.ManualActionRequired = portainer.PlatformReleaseStatusRecoveryFailed, ReleaseFailureReasonRecoveryFailed, true
			release.TargetResults, release.FinishedAt = results, executor.now().Unix()
			return ReleaseExecutionResult{Release: release}, nil
		}
		results[i].Status, results[i].Reason = portainer.PlatformReleaseTargetStatusRecovered, ""
	}
	release.TargetResults = results
	failRelease(&release, ReleaseFailureReasonFinalHealthFailed, safeReleaseFailureMessage(ReleaseFailureReasonFinalHealthFailed), executor.now().Unix())
	return ReleaseExecutionResult{Release: release}, nil
}

func selectMultiWorkloadTargets(environment portainer.PlatformEnvironment) ([]portainer.PlatformDeploymentTarget, error) {
	if environment.TargetMode != portainer.PlatformTargetModeMulti {
		return nil, errors.New("multi target mode is required")
	}
	targets := make([]portainer.PlatformDeploymentTarget, 0)
	for _, target := range environment.Targets {
		if target.Enabled && target.Role == portainer.PlatformDeploymentTargetRoleWorkload {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return nil, errors.New("workload target is required")
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].EndpointID != targets[j].EndpointID {
			return targets[i].EndpointID < targets[j].EndpointID
		}
		return targets[i].NodeName < targets[j].NodeName
	})
	return targets, nil
}

func targetResultByEndpoint(results []portainer.PlatformReleaseTargetResult) map[portainer.EndpointID]portainer.PlatformReleaseTargetResult {
	byEndpoint := make(map[portainer.EndpointID]portainer.PlatformReleaseTargetResult, len(results))
	for _, result := range results {
		byEndpoint[result.EndpointID] = result
	}
	return byEndpoint
}

func targetStatusFromRelease(release portainer.PlatformRelease) portainer.PlatformReleaseTargetStatus {
	if release.Status == portainer.PlatformReleaseStatusSucceeded {
		return portainer.PlatformReleaseTargetStatusSucceeded
	}
	return portainer.PlatformReleaseTargetStatusFailed
}

// RetryRecovery 按 Release 保存的目标结果逐台恢复上一版运行时；任一失败保留人工处置状态。
func (executor *MultiTargetExecutor) RetryRecovery(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error) {
	if request.Environment.TargetMode != portainer.PlatformTargetModeMulti {
		return NewSingleTargetExecutor(executor.driver).RetryRecovery(ctx, request)
	}
	release := request.Release
	now := executor.now().Unix()
	if executor.driver == nil {
		markRecoveryRetryFailed(&release, ReleaseFailureReasonExecutorUnavailable, safeReleaseFailureMessage(ReleaseFailureReasonExecutorUnavailable), now)
		return ReleaseExecutionResult{Release: release}, nil
	}
	previous := targetResultByEndpoint(request.Deployment.CurrentTargetRuntimeRefs)
	for i := range release.TargetResults {
		result := &release.TargetResults[i]
		if result.Status != portainer.PlatformReleaseTargetStatusSucceeded && result.Status != portainer.PlatformReleaseTargetStatusFailed {
			continue
		}
		perTarget := request
		if prior, found := previous[result.EndpointID]; found {
			perTarget.Deployment.CurrentRuntimeRef = prior.RuntimeRef
		}
		target := portainer.PlatformDeploymentTarget{EndpointID: result.EndpointID, NodeName: result.NodeName, HostAddress: result.HostAddress, Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true}
		if err := executor.driver.Recover(ctx, perTarget, target); err != nil {
			result.Status, result.Reason = portainer.PlatformReleaseTargetStatusRecoveryFailed, ReleaseFailureReasonRecoveryFailed
			markRecoveryRetryFailed(&release, ReleaseFailureReasonRecoveryFailed, safeReleaseFailureMessage(ReleaseFailureReasonRecoveryFailed), executor.now().Unix())
			return ReleaseExecutionResult{Release: release}, nil
		}
		result.Status, result.Reason = portainer.PlatformReleaseTargetStatusRecovered, ""
	}
	release.Status, release.ManualActionRequired, release.FinishedAt, release.LeaseOwner, release.LeaseExpiresAt = portainer.PlatformReleaseStatusFailed, false, executor.now().Unix(), "", 0
	deployment := request.Deployment
	deployment.DriftStatus = portainer.PlatformDeploymentDriftNone
	deployment.ResourceVersion++
	deployment.UpdatedAt = executor.now().Unix()
	return ReleaseExecutionResult{Release: release, Deployment: &deployment}, nil
}

func (executor *MultiTargetExecutor) CleanupRuntime(ctx context.Context, request ReleaseCleanupRequest) (ReleaseCleanupResult, error) {
	if request.Release.TargetSnapshot.TargetMode != portainer.PlatformTargetModeMulti {
		return NewSingleTargetExecutor(executor.driver).CleanupRuntime(ctx, request)
	}
	if executor.driver == nil {
		return ReleaseCleanupResult{Release: request.Release}, nil
	}
	deleted, failed := make([]portainer.RuntimeRef, 0), make([]portainer.RuntimeRef, 0)
	for _, result := range request.Release.TargetResults {
		if result.RuntimeRef.ResourceID == "" {
			continue
		}
		if err := executor.driver.DeleteRuntime(ctx, result.RuntimeRef); err != nil {
			failed = append(failed, result.RuntimeRef)
		} else {
			deleted = append(deleted, result.RuntimeRef)
		}
	}
	return ReleaseCleanupResult{Release: request.Release, DeletedRuntimeRefs: deleted, FailedRuntimeRefs: failed}, nil
}
