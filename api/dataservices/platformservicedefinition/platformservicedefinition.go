package platformservicedefinition

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_service_definitions"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformServiceDefinition, portainer.PlatformServiceDefinitionID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformServiceDefinition, portainer.PlatformServiceDefinitionID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformServiceDefinition, portainer.PlatformServiceDefinitionID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create stores the cross-environment logical service only; per-environment
// deployment facts remain in PlatformServiceDeployment and Release records.
func (service *Service) Create(definition *portainer.PlatformServiceDefinition) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			definition.ID = portainer.PlatformServiceDefinitionID(id)
			return int(definition.ID), definition
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformServiceDefinition, portainer.PlatformServiceDefinitionID]
}

func (service ServiceTx) Create(definition *portainer.PlatformServiceDefinition) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			definition.ID = portainer.PlatformServiceDefinitionID(id)
			return int(definition.ID), definition
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}
