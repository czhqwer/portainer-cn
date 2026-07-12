package platformgateway

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_gateways"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformGateway, portainer.PlatformGatewayID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformGateway, portainer.PlatformGatewayID]{Bucket: BucketName, Connection: connection}}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformGateway, portainer.PlatformGatewayID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}

// Create 只持久化经过模型校验的网关定位信息；实际容器发现和配置发布属于事务外运行时操作。
func (service *Service) Create(gateway *portainer.PlatformGateway) error {
	prepareForPersistence(gateway)
	if err := portainer.ValidatePlatformGateway(*gateway); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		gateway.ID = portainer.PlatformGatewayID(id)
		return int(gateway.ID), gateway
	})
}

func (service *Service) Update(id portainer.PlatformGatewayID, gateway *portainer.PlatformGateway) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, gateway) })
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformGateway, portainer.PlatformGatewayID]
}

func (service ServiceTx) Create(gateway *portainer.PlatformGateway) error {
	prepareForPersistence(gateway)
	if err := portainer.ValidatePlatformGateway(*gateway); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		gateway.ID = portainer.PlatformGatewayID(id)
		return int(gateway.ID), gateway
	})
}

func (service ServiceTx) Update(id portainer.PlatformGatewayID, gateway *portainer.PlatformGateway) error {
	prepareForPersistence(gateway)
	if err := portainer.ValidatePlatformGateway(*gateway); err != nil {
		return err
	}
	return service.BaseDataServiceTx.Update(id, gateway)
}

func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }

func prepareForPersistence(gateway *portainer.PlatformGateway) {
	portainer.NormalizePlatformGateway(gateway)
	if gateway.LifecycleStatus == "" {
		gateway.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if gateway.ResourceVersion == 0 {
		gateway.ResourceVersion = 1
	}
}
