package platform

import (
	"context"
	"errors"

	portainer "github.com/portainer/portainer/api"
)

const (
	GatewayConfigFailurePreflight = "GATEWAY_CONFIG_PREFLIGHT_FAILED"
	GatewayConfigFailureReload    = "GATEWAY_CONFIG_RELOAD_FAILED"
	GatewayConfigFailureRecovery  = "GATEWAY_CONFIG_RECOVERY_FAILED"
)

// GatewayConfigPublisher 把候选写入、Nginx 预检、活动切换和 reload 串成固定顺序。
// 运行时动作不放进 BoltDB 事务；持久化调用方可根据返回的稳定 reason 记录候选/失败/活动版本和审计。
type GatewayConfigPublisher struct {
	store   *GatewayConfigStore
	runtime GatewayRuntime
}

type GatewayConfigPublishResult struct {
	CandidateHash string
	PreviousHash  string
	Recovered     bool
}

type GatewayConfigPublishError struct{ Reason string }

func (err *GatewayConfigPublishError) Error() string { return err.Reason }

func NewGatewayConfigPublisher(store *GatewayConfigStore, runtime GatewayRuntime) (*GatewayConfigPublisher, error) {
	if store == nil || runtime == nil {
		return nil, errors.New("gateway config publisher dependencies are required")
	}
	return &GatewayConfigPublisher{store: store, runtime: runtime}, nil
}

func (publisher *GatewayConfigPublisher) Publish(ctx context.Context, gateway portainer.PlatformGateway, config []byte) (GatewayConfigPublishResult, error) {
	if publisher == nil || gateway.ID <= 0 {
		return GatewayConfigPublishResult{}, errors.New("gateway config publisher input is invalid")
	}
	candidateHash, err := publisher.store.WriteCandidate(gateway.ID, config)
	if err != nil {
		return GatewayConfigPublishResult{}, err
	}
	result := GatewayConfigPublishResult{CandidateHash: candidateHash}
	if err := publisher.runtime.Test(ctx, gateway, candidateHash); err != nil {
		return result, &GatewayConfigPublishError{Reason: GatewayConfigFailurePreflight}
	}
	previousHash, err := publisher.store.Activate(gateway.ID, candidateHash)
	if err != nil {
		return result, err
	}
	result.PreviousHash = previousHash
	if err := publisher.runtime.Reload(ctx, gateway); err == nil {
		return result, nil
	}

	// reload 失败时不能只恢复磁盘文件：运行中的 Nginx 仍可能持有失败配置。
	// 因此恢复旧版本后必须再 reload；任一恢复步骤失败都明确返回人工处置 reason。
	if previousHash == "" {
		return result, &GatewayConfigPublishError{Reason: GatewayConfigFailureReload}
	}
	if _, err := publisher.store.Activate(gateway.ID, previousHash); err != nil {
		return result, &GatewayConfigPublishError{Reason: GatewayConfigFailureRecovery}
	}
	if err := publisher.runtime.Reload(ctx, gateway); err != nil {
		return result, &GatewayConfigPublishError{Reason: GatewayConfigFailureRecovery}
	}
	result.Recovered = true
	return result, &GatewayConfigPublishError{Reason: GatewayConfigFailureReload}
}
