package platformenvironment

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_environments"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformEnvironment, portainer.PlatformEnvironmentID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformEnvironment, portainer.PlatformEnvironmentID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformEnvironment, portainer.PlatformEnvironmentID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create only persists the environment binding metadata; it never probes Endpoint
// runtime state so Gate 0A remains independent from Docker/Agent Spike results.
func (service *Service) Create(environment *portainer.PlatformEnvironment) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			environment.ID = portainer.PlatformEnvironmentID(id)
			return int(environment.ID), environment
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformEnvironment, portainer.PlatformEnvironmentID]
}

func (service ServiceTx) Create(environment *portainer.PlatformEnvironment) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			environment.ID = portainer.PlatformEnvironmentID(id)
			return int(environment.ID), environment
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}
