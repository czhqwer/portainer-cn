package databaseconnection

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "database_connection"

type Service struct {
	dataservices.BaseDataService[portainer.DatabaseConnection, portainer.DatabaseConnectionID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.DatabaseConnection, portainer.DatabaseConnectionID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.DatabaseConnection, portainer.DatabaseConnectionID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

func (service *Service) Create(connection *portainer.DatabaseConnection) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			connection.ID = portainer.DatabaseConnectionID(id)
			return int(connection.ID), connection
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

func (service *Service) ConnectionsByContainer(userID portainer.UserID, environmentID portainer.EndpointID, containerID string) ([]portainer.DatabaseConnection, error) {
	return service.ReadAll(func(connection portainer.DatabaseConnection) bool {
		return connection.CreatedByUserID == userID &&
			connection.EnvironmentID == environmentID &&
			connection.ContainerID == containerID
	})
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.DatabaseConnection, portainer.DatabaseConnectionID]
}

func (service ServiceTx) Create(connection *portainer.DatabaseConnection) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			connection.ID = portainer.DatabaseConnectionID(id)
			return int(connection.ID), connection
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}

func (service ServiceTx) ConnectionsByContainer(userID portainer.UserID, environmentID portainer.EndpointID, containerID string) ([]portainer.DatabaseConnection, error) {
	return service.ReadAll(func(connection portainer.DatabaseConnection) bool {
		return connection.CreatedByUserID == userID &&
			connection.EnvironmentID == environmentID &&
			connection.ContainerID == containerID
	})
}
