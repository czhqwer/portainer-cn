package platformhostgroup

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_host_groups"

// Service 持久化未来发布使用的可变主机组；既有 Release 必须持有自己的目标快照。
type Service struct {
	dataservices.BaseDataService[portainer.PlatformHostGroup, portainer.PlatformHostGroupID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}
	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformHostGroup, portainer.PlatformHostGroupID]{Bucket: BucketName, Connection: connection}}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformHostGroup, portainer.PlatformHostGroupID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}

func (service *Service) Create(group *portainer.PlatformHostGroup) error {
	prepare(group)
	if err := portainer.ValidatePlatformHostGroup(*group); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		group.ID = portainer.PlatformHostGroupID(id)
		return int(group.ID), group
	})
}

func (service *Service) Update(id portainer.PlatformHostGroupID, group *portainer.PlatformHostGroup) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, group) })
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformHostGroup, portainer.PlatformHostGroupID]
}

func (service ServiceTx) Create(group *portainer.PlatformHostGroup) error {
	prepare(group)
	if err := portainer.ValidatePlatformHostGroup(*group); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		group.ID = portainer.PlatformHostGroupID(id)
		return int(group.ID), group
	})
}

func (service ServiceTx) Update(id portainer.PlatformHostGroupID, group *portainer.PlatformHostGroup) error {
	prepare(group)
	if err := portainer.ValidatePlatformHostGroup(*group); err != nil {
		return err
	}
	return service.BaseDataServiceTx.Update(id, group)
}

func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }

func prepare(group *portainer.PlatformHostGroup) {
	portainer.NormalizePlatformHostGroup(group)
	if group.LifecycleStatus == "" {
		group.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if group.ResourceVersion == 0 {
		group.ResourceVersion = 1
	}
}
