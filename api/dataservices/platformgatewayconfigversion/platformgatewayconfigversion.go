package platformgatewayconfigversion

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_gateway_config_versions"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformGatewayConfigVersion, portainer.PlatformGatewayConfigVersionID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}
	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformGatewayConfigVersion, portainer.PlatformGatewayConfigVersionID]{Bucket: BucketName, Connection: connection}}, nil
}
func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformGatewayConfigVersion, portainer.PlatformGatewayConfigVersionID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}
func (service *Service) Create(version *portainer.PlatformGatewayConfigVersion) error {
	portainer.NormalizePlatformGatewayConfigVersion(version)
	if err := portainer.ValidatePlatformGatewayConfigVersion(*version); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		version.ID = portainer.PlatformGatewayConfigVersionID(id)
		return int(version.ID), version
	})
}
func (service *Service) Update(id portainer.PlatformGatewayConfigVersionID, version *portainer.PlatformGatewayConfigVersion) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, version) })
}
func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformGatewayConfigVersion, portainer.PlatformGatewayConfigVersionID]
}

func (service ServiceTx) Create(version *portainer.PlatformGatewayConfigVersion) error {
	portainer.NormalizePlatformGatewayConfigVersion(version)
	if err := portainer.ValidatePlatformGatewayConfigVersion(*version); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		version.ID = portainer.PlatformGatewayConfigVersionID(id)
		return int(version.ID), version
	})
}
func (service ServiceTx) Update(id portainer.PlatformGatewayConfigVersionID, version *portainer.PlatformGatewayConfigVersion) error {
	portainer.NormalizePlatformGatewayConfigVersion(version)
	if err := portainer.ValidatePlatformGatewayConfigVersion(*version); err != nil {
		return err
	}
	return service.BaseDataServiceTx.Update(id, version)
}
func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }
