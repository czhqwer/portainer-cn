package platformservicedatabasebinding

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_service_database_bindings"

// Service 持久化服务与项目数据库资源的无密文引用；Release 自身只保存可追溯快照。
type Service struct {
	dataservices.BaseDataService[portainer.PlatformServiceDatabaseBinding, portainer.PlatformServiceDatabaseBindingID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}
	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformServiceDatabaseBinding, portainer.PlatformServiceDatabaseBindingID]{Bucket: BucketName, Connection: connection}}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformServiceDatabaseBinding, portainer.PlatformServiceDatabaseBindingID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}

func (service *Service) Create(binding *portainer.PlatformServiceDatabaseBinding) error {
	prepare(binding)
	if err := portainer.ValidatePlatformServiceDatabaseBinding(*binding); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		binding.ID = portainer.PlatformServiceDatabaseBindingID(id)
		return int(binding.ID), binding
	})
}

func (service *Service) Update(id portainer.PlatformServiceDatabaseBindingID, binding *portainer.PlatformServiceDatabaseBinding) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, binding) })
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformServiceDatabaseBinding, portainer.PlatformServiceDatabaseBindingID]
}

func (service ServiceTx) Create(binding *portainer.PlatformServiceDatabaseBinding) error {
	prepare(binding)
	if err := portainer.ValidatePlatformServiceDatabaseBinding(*binding); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		binding.ID = portainer.PlatformServiceDatabaseBindingID(id)
		return int(binding.ID), binding
	})
}

func (service ServiceTx) Update(id portainer.PlatformServiceDatabaseBindingID, binding *portainer.PlatformServiceDatabaseBinding) error {
	previous, err := service.BaseDataServiceTx.Read(id)
	prepare(binding)
	if err := portainer.ValidatePlatformServiceDatabaseBinding(*binding); err != nil {
		return err
	}
	if err == nil && (previous.ProjectID != binding.ProjectID || previous.EnvironmentID != binding.EnvironmentID || previous.ServiceDeploymentID != binding.ServiceDeploymentID || previous.DatabaseResourceID != binding.DatabaseResourceID) {
		binding.Revision = previous.Revision + 1
	}
	return service.BaseDataServiceTx.Update(id, binding)
}

func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }

func prepare(binding *portainer.PlatformServiceDatabaseBinding) {
	portainer.NormalizePlatformServiceDatabaseBinding(binding)
	if binding.LifecycleStatus == "" {
		binding.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if binding.ResourceVersion == 0 {
		binding.ResourceVersion = 1
	}
}
