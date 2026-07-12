package platform

import (
	"context"

	portainer "github.com/portainer/portainer/api"
)

// AdaptiveReleaseExecutor 保留 single 的阶段 1 行为，并仅在明确 multi 环境时分派到多目标执行器。
type AdaptiveReleaseExecutor struct {
	single *SingleTargetExecutor
	multi  *MultiTargetExecutor
}

func NewAdaptiveReleaseExecutor(driver RuntimeDriver) *AdaptiveReleaseExecutor {
	return &AdaptiveReleaseExecutor{single: NewSingleTargetExecutor(driver), multi: NewMultiTargetExecutor(driver)}
}

func (executor *AdaptiveReleaseExecutor) WithGatewayCutover(cutover GatewayCutover) *AdaptiveReleaseExecutor {
	executor.single.WithGatewayCutover(cutover)
	executor.multi.WithGatewayCutover(cutover)
	return executor
}

func (executor *AdaptiveReleaseExecutor) Execute(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error) {
	if request.Environment.TargetMode == portainer.PlatformTargetModeMulti {
		return executor.multi.Execute(ctx, request)
	}
	return executor.single.Execute(ctx, request)
}

func (executor *AdaptiveReleaseExecutor) RetryRecovery(ctx context.Context, request ReleaseExecutionRequest) (ReleaseExecutionResult, error) {
	if request.Environment.TargetMode == portainer.PlatformTargetModeMulti {
		return executor.multi.RetryRecovery(ctx, request)
	}
	return executor.single.RetryRecovery(ctx, request)
}

func (executor *AdaptiveReleaseExecutor) CleanupRuntime(ctx context.Context, request ReleaseCleanupRequest) (ReleaseCleanupResult, error) {
	if request.Release.TargetSnapshot.TargetMode == portainer.PlatformTargetModeMulti {
		return executor.multi.CleanupRuntime(ctx, request)
	}
	return executor.single.CleanupRuntime(ctx, request)
}

var _ ReleaseExecutor = (*AdaptiveReleaseExecutor)(nil)
var _ ReleaseRecoveryExecutor = (*AdaptiveReleaseExecutor)(nil)
