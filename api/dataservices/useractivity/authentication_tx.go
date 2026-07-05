package useractivity

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

type AuthenticationLogServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.UserAuthenticationLog, portainer.UserAuthenticationLogID]
}

func (service *AuthenticationLogService) Tx(tx portainer.Transaction) AuthenticationLogServiceTx {
	return AuthenticationLogServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.UserAuthenticationLog, portainer.UserAuthenticationLogID]{
			Bucket:     AuthenticationLogBucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

func (service AuthenticationLogServiceTx) Create(log *portainer.UserAuthenticationLog) error {
	log.ID = portainer.UserAuthenticationLogID(service.Tx.GetNextIdentifier(AuthenticationLogBucketName))
	return service.Tx.CreateObjectWithId(AuthenticationLogBucketName, int(log.ID), log)
}

func (service AuthenticationLogServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(AuthenticationLogBucketName)
}
