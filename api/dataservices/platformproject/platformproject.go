package platformproject

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_projects"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformProject, portainer.PlatformProjectID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformProject, portainer.PlatformProjectID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformProject, portainer.PlatformProjectID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create only allocates a control-plane ID and writes metadata;
// Docker runtime side effects are intentionally outside Gate 0A dataservices.
func (service *Service) Create(project *portainer.PlatformProject) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			project.ID = portainer.PlatformProjectID(id)
			return int(project.ID), project
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformProject, portainer.PlatformProjectID]
}

func (service ServiceTx) Create(project *portainer.PlatformProject) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			project.ID = portainer.PlatformProjectID(id)
			return int(project.ID), project
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}
