package platformrelease

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_releases"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformRelease, portainer.PlatformReleaseID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformRelease, portainer.PlatformReleaseID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformRelease, portainer.PlatformReleaseID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create records the asynchronous release fact in BoltDB only; workers and Docker
// execution are separate later batches so the status machine can be tested safely.
func (service *Service) Create(release *portainer.PlatformRelease) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			release.ID = portainer.PlatformReleaseID(id)
			return int(release.ID), release
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformRelease, portainer.PlatformReleaseID]
}

func (service ServiceTx) Create(release *portainer.PlatformRelease) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			release.ID = portainer.PlatformReleaseID(id)
			return int(release.ID), release
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}
