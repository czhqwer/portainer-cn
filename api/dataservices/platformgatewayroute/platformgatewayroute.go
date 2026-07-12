package platformgatewayroute

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_gateway_routes"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformGatewayRoute, portainer.PlatformGatewayRouteID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}
	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformGatewayRoute, portainer.PlatformGatewayRouteID]{Bucket: BucketName, Connection: connection}}, nil
}
func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformGatewayRoute, portainer.PlatformGatewayRouteID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}
func (service *Service) Create(route *portainer.PlatformGatewayRoute) error {
	prepareForPersistence(route)
	if err := portainer.ValidatePlatformGatewayRoute(*route); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		route.ID = portainer.PlatformGatewayRouteID(id)
		return int(route.ID), route
	})
}
func (service *Service) Update(id portainer.PlatformGatewayRouteID, route *portainer.PlatformGatewayRoute) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, route) })
}
func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformGatewayRoute, portainer.PlatformGatewayRouteID]
}

func (service ServiceTx) Create(route *portainer.PlatformGatewayRoute) error {
	prepareForPersistence(route)
	if err := portainer.ValidatePlatformGatewayRoute(*route); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		route.ID = portainer.PlatformGatewayRouteID(id)
		return int(route.ID), route
	})
}
func (service ServiceTx) Update(id portainer.PlatformGatewayRouteID, route *portainer.PlatformGatewayRoute) error {
	prepareForPersistence(route)
	if err := portainer.ValidatePlatformGatewayRoute(*route); err != nil {
		return err
	}
	return service.BaseDataServiceTx.Update(id, route)
}
func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }
func prepareForPersistence(route *portainer.PlatformGatewayRoute) {
	portainer.NormalizePlatformGatewayRoute(route)
	if route.LifecycleStatus == "" {
		route.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if route.ResourceVersion == 0 {
		route.ResourceVersion = 1
	}
}
