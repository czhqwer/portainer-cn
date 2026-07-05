package useractivity

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

type ActivityLogServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.UserActivityLog, portainer.UserActivityLogID]
}

func (service *ActivityLogService) Tx(tx portainer.Transaction) ActivityLogServiceTx {
	return ActivityLogServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.UserActivityLog, portainer.UserActivityLogID]{
			Bucket:     ActivityLogBucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

func (service ActivityLogServiceTx) Create(log *portainer.UserActivityLog) error {
	log.ID = portainer.UserActivityLogID(service.Tx.GetNextIdentifier(ActivityLogBucketName))
	return service.Tx.CreateObjectWithId(ActivityLogBucketName, int(log.ID), log)
}

func (service ActivityLogServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(ActivityLogBucketName)
}
