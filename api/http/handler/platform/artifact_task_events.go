package platform

import (
	"regexp"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const (
	artifactTaskEventLimit    = 16
	artifactTaskLogLimit      = 200
	artifactTaskLogBytesLimit = 128 * 1024
	artifactTaskLogLineLimit  = 4 * 1024
)

var (
	artifactTaskLogURLUserInfoPattern         = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s@]+@`)
	artifactTaskLogSensitiveQueryPattern      = regexp.MustCompile(`(?i)([?&](?:x-amz-signature|x-amz-credential|token|signature|sig|access[_-]?key|secret|password)=)[^&#\s]+`)
	artifactTaskLogSensitiveAssignmentPattern = regexp.MustCompile(`(?i)\b([a-z0-9_.-]*(?:password|secret|token|api[_-]?key|access[_-]?key)[a-z0-9_.-]*\s*[:=]\s*)([^\s,;]+)`)
	artifactTaskLogAuthHeaderPattern          = regexp.MustCompile(`(?i)\b(authorization\s*:\s*).*$`)
	artifactTaskLogAuthSchemePattern          = regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+([a-z0-9._~+/=-]+)`)
	artifactTaskLogWindowsPathPattern         = regexp.MustCompile(`(?i)\b[a-z]:\\(?:[^\\/:*?"<>|\r\n]+\\)*[^\\/:*?"<>|\r\n]*`)
	artifactTaskLogUnixPathPattern            = regexp.MustCompile(`/(?:home|root|var|tmp|users|workspace|data)(?:/[^\s'"]+)*`)
)

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

// resetArtifactTaskLogs 在新的受控制品任务开始时丢弃上一次操作的终端输出，
// 防止操作者把历史失败日志误认为当前构建结果，也防止日志在 BoltDB 中无限增长。
func resetArtifactTaskLogs(artifact *portainer.PlatformArtifact) {
	artifact.TaskLogs = nil
}

// appendArtifactTaskLog 只保留经过服务端脱敏后的 Docker 文本输出；日志必须能帮助定位构建问题，
// 但不能把认证值、签名 URL 或服务端真实路径同步到浏览器和非加密制品记录中。
func appendArtifactTaskLog(artifact *portainer.PlatformArtifact, output string) {
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		artifact.TaskLogs = append(artifact.TaskLogs, limitArtifactTaskLogLine(redactArtifactTaskLog(line)))
	}
	for len(artifact.TaskLogs) > artifactTaskLogLimit || artifactTaskLogSize(artifact.TaskLogs) > artifactTaskLogBytesLimit {
		artifact.TaskLogs = artifact.TaskLogs[1:]
	}
}

func redactArtifactTaskLog(line string) string {
	line = artifactTaskLogURLUserInfoPattern.ReplaceAllString(line, "$1***@")
	line = artifactTaskLogSensitiveQueryPattern.ReplaceAllString(line, "$1***")
	line = artifactTaskLogSensitiveAssignmentPattern.ReplaceAllString(line, "$1***")
	line = artifactTaskLogAuthHeaderPattern.ReplaceAllString(line, "$1***")
	line = artifactTaskLogAuthSchemePattern.ReplaceAllString(line, "$1 ***")
	line = artifactTaskLogWindowsPathPattern.ReplaceAllString(line, "[redacted-path]")
	return artifactTaskLogUnixPathPattern.ReplaceAllString(line, "[redacted-path]")
}

func limitArtifactTaskLogLine(line string) string {
	if len(line) <= artifactTaskLogLineLimit {
		return line
	}
	limit := artifactTaskLogLineLimit - len("...")
	for index := range line {
		if index > limit {
			return line[:index] + "..."
		}
	}
	return line
}

func artifactTaskLogSize(lines []string) int {
	size := 0
	for _, line := range lines {
		size += len(line) + 1
	}
	return size
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

// recordArtifactTaskLog 用任务租约将 Docker I/O 与 BoltDB 写入隔离开来；
// 旧任务超时或被替换后不再能够向新任务追加终端日志。
func (handler *Handler) recordArtifactTaskLog(artifactID portainer.PlatformArtifactID, leaseID, output string) {
	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		artifact, err := tx.PlatformArtifact().Read(artifactID)
		if err != nil || artifact.TaskID != leaseID {
			return err
		}
		appendArtifactTaskLog(artifact, output)
		touchLifecycle(&artifact.PlatformLifecycle, time.Now().Unix())
		return tx.PlatformArtifact().Update(artifact.ID, artifact)
	})
}
