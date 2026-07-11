package platformauditlog

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_audit_logs"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformAuditLog, portainer.PlatformAuditLogID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformAuditLog, portainer.PlatformAuditLogID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformAuditLog, portainer.PlatformAuditLogID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

func (service *Service) Create(log *portainer.PlatformAuditLog) error {
	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			log.ID = portainer.PlatformAuditLogID(id)
			return int(log.ID), log
		},
	)
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformAuditLog, portainer.PlatformAuditLogID]
}

func (service ServiceTx) Create(log *portainer.PlatformAuditLog) error {
	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			log.ID = portainer.PlatformAuditLogID(id)
			return int(log.ID), log
		},
	)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}
