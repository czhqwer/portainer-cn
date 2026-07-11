package platformapplication

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_applications"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformApplication, portainer.PlatformApplicationID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformApplication, portainer.PlatformApplicationID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformApplication, portainer.PlatformApplicationID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create writes only the application grouping record; service/runtime aggregation
// is handled by later platform services instead of this bucket layer.
func (service *Service) Create(application *portainer.PlatformApplication) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			application.ID = portainer.PlatformApplicationID(id)
			return int(application.ID), application
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformApplication, portainer.PlatformApplicationID]
}

func (service ServiceTx) Create(application *portainer.PlatformApplication) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			application.ID = portainer.PlatformApplicationID(id)
			return int(application.ID), application
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}
