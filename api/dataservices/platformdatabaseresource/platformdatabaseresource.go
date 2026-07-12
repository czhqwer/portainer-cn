package platformdatabaseresource

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_database_resources"

// Service 仅保存已加密的项目数据库资源；密码明文只能在 handler 请求内存中短暂存在。
type Service struct {
	dataservices.BaseDataService[portainer.PlatformDatabaseResource, portainer.PlatformDatabaseResourceID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}
	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformDatabaseResource, portainer.PlatformDatabaseResourceID]{Bucket: BucketName, Connection: connection}}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformDatabaseResource, portainer.PlatformDatabaseResourceID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}

func (service *Service) Create(resource *portainer.PlatformDatabaseResource) error {
	prepare(resource)
	if err := portainer.ValidatePlatformDatabaseResource(*resource); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		resource.ID = portainer.PlatformDatabaseResourceID(id)
		return int(resource.ID), resource
	})
}

func (service *Service) Update(id portainer.PlatformDatabaseResourceID, resource *portainer.PlatformDatabaseResource) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, resource) })
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformDatabaseResource, portainer.PlatformDatabaseResourceID]
}

func (service ServiceTx) Create(resource *portainer.PlatformDatabaseResource) error {
	prepare(resource)
	if err := portainer.ValidatePlatformDatabaseResource(*resource); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		resource.ID = portainer.PlatformDatabaseResourceID(id)
		return int(resource.ID), resource
	})
}

func (service ServiceTx) Update(id portainer.PlatformDatabaseResourceID, resource *portainer.PlatformDatabaseResource) error {
	previous, err := service.BaseDataServiceTx.Read(id)
	prepare(resource)
	if err := portainer.ValidatePlatformDatabaseResource(*resource); err != nil {
		return err
	}
	if err == nil && databaseResourceContentChanged(*previous, *resource) {
		resource.Revision = previous.Revision + 1
	}
	return service.BaseDataServiceTx.Update(id, resource)
}

func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }

func prepare(resource *portainer.PlatformDatabaseResource) {
	portainer.NormalizePlatformDatabaseResource(resource)
	if resource.LifecycleStatus == "" {
		resource.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if resource.ResourceVersion == 0 {
		resource.ResourceVersion = 1
	}
}

func databaseResourceContentChanged(previous, next portainer.PlatformDatabaseResource) bool {
	return previous.ProjectID != next.ProjectID || previous.EnvironmentID != next.EnvironmentID || previous.EndpointID != next.EndpointID || previous.Name != next.Name || previous.Type != next.Type || previous.Host != next.Host || previous.Port != next.Port || previous.Database != next.Database || previous.Username != next.Username || previous.PasswordCipherText != next.PasswordCipherText || previous.CredentialEncryptionVersion != next.CredentialEncryptionVersion || previous.CredentialHash != next.CredentialHash || previous.HasPassword != next.HasPassword || previous.ConnectionTimeoutSeconds != next.ConnectionTimeoutSeconds
}
