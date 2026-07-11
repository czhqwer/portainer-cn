package platformartifact

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_artifacts"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformArtifact, portainer.PlatformArtifactID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformArtifact, portainer.PlatformArtifactID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformArtifact, portainer.PlatformArtifactID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create records deployment input metadata such as image-reference; it does not
// pull, inspect, or mutate images in Gate 0A.
func (service *Service) Create(artifact *portainer.PlatformArtifact) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			artifact.ID = portainer.PlatformArtifactID(id)
			return int(artifact.ID), artifact
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformArtifact, portainer.PlatformArtifactID]
}

func (service ServiceTx) Create(artifact *portainer.PlatformArtifact) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			artifact.ID = portainer.PlatformArtifactID(id)
			return int(artifact.ID), artifact
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}
