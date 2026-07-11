package platformservicedeployment

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_service_deployments"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformServiceDeployment, portainer.PlatformServiceDeploymentID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformServiceDeployment, portainer.PlatformServiceDeploymentID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformServiceDeployment, portainer.PlatformServiceDeploymentID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create persists desired deployment state and current release references only;
// actual container creation is forbidden until the Gate 0B executor batch.
func (service *Service) Create(deployment *portainer.PlatformServiceDeployment) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			deployment.ID = portainer.PlatformServiceDeploymentID(id)
			return int(deployment.ID), deployment
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformServiceDeployment, portainer.PlatformServiceDeploymentID]
}

func (service ServiceTx) Create(deployment *portainer.PlatformServiceDeployment) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			deployment.ID = portainer.PlatformServiceDeploymentID(id)
			return int(deployment.ID), deployment
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}
