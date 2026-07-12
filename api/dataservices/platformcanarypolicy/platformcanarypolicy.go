package platformcanarypolicy

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_canary_policies"

// Service 只持久化发布引用与受限权重；实际 upstream 必须由 handler 从成功 Release 重新解析。
type Service struct {
	dataservices.BaseDataService[portainer.PlatformCanaryPolicy, portainer.PlatformCanaryPolicyID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}
	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformCanaryPolicy, portainer.PlatformCanaryPolicyID]{Bucket: BucketName, Connection: connection}}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformCanaryPolicy, portainer.PlatformCanaryPolicyID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}

func (service *Service) Create(policy *portainer.PlatformCanaryPolicy) error {
	prepare(policy)
	if err := portainer.ValidatePlatformCanaryPolicy(*policy); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		policy.ID = portainer.PlatformCanaryPolicyID(id)
		return int(policy.ID), policy
	})
}

func (service *Service) Update(id portainer.PlatformCanaryPolicyID, policy *portainer.PlatformCanaryPolicy) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, policy) })
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformCanaryPolicy, portainer.PlatformCanaryPolicyID]
}

func (service ServiceTx) Create(policy *portainer.PlatformCanaryPolicy) error {
	prepare(policy)
	if err := portainer.ValidatePlatformCanaryPolicy(*policy); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		policy.ID = portainer.PlatformCanaryPolicyID(id)
		return int(policy.ID), policy
	})
}

func (service ServiceTx) Update(id portainer.PlatformCanaryPolicyID, policy *portainer.PlatformCanaryPolicy) error {
	prepare(policy)
	if err := portainer.ValidatePlatformCanaryPolicy(*policy); err != nil {
		return err
	}
	return service.BaseDataServiceTx.Update(id, policy)
}

func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }

func prepare(policy *portainer.PlatformCanaryPolicy) {
	portainer.NormalizePlatformCanaryPolicy(policy)
	if policy.LifecycleStatus == "" {
		policy.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if policy.ResourceVersion == 0 {
		policy.ResourceVersion = 1
	}
}
