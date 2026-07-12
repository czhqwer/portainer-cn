package platform

import (
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const artifactTaskEventLimit = 16

const (
	artifactTaskEventRunning   = "running"
	artifactTaskEventSucceeded = "succeeded"
	artifactTaskEventFailed    = "failed"
)

// resetArtifactTaskEvents 在每次新任务开始时只保留当前任务的安全阶段记录，
// 避免历史执行细节不断写大 BoltDB，也避免把一次失败误解为当前任务的状态。
func resetArtifactTaskEvents(artifact *portainer.PlatformArtifact, stage string, now int64) {
	artifact.TaskEvents = []portainer.PlatformArtifactTaskEvent{
		{Stage: stage, Status: artifactTaskEventRunning, OccurredAt: now},
	}
}

// appendArtifactTaskEvent 只接受调用方提供的固定阶段和 reason code；禁止透传 Docker、registry
// 或对象存储原始错误，从而使前端进度日志可追踪但不泄露凭据、签名 URL 或服务器路径。
func appendArtifactTaskEvent(artifact *portainer.PlatformArtifact, stage, status, reason string, now int64) {
	artifact.TaskEvents = append(artifact.TaskEvents, portainer.PlatformArtifactTaskEvent{
		Stage:      stage,
		Status:     status,
		Reason:     reason,
		OccurredAt: now,
	})
	if len(artifact.TaskEvents) > artifactTaskEventLimit {
		artifact.TaskEvents = artifact.TaskEvents[len(artifact.TaskEvents)-artifactTaskEventLimit:]
	}
}

// recordArtifactTaskEvent 让耗时 I/O 在事务外继续执行时仍能安全地更新可见进度。
// 任务租约必须匹配，避免旧请求或超时请求覆盖新任务的执行记录。
func (handler *Handler) recordArtifactTaskEvent(artifactID portainer.PlatformArtifactID, leaseID, stage, status, reason string) {
	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		artifact, err := tx.PlatformArtifact().Read(artifactID)
		if err != nil || artifact.TaskID != leaseID {
			return err
		}
		appendArtifactTaskEvent(artifact, stage, status, reason, time.Now().Unix())
		touchLifecycle(&artifact.PlatformLifecycle, time.Now().Unix())
		return tx.PlatformArtifact().Update(artifact.ID, artifact)
	})
}
