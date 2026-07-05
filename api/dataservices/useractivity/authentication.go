package useractivity

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const AuthenticationLogBucketName = "user_authentication_logs"

type AuthenticationLogService struct {
	dataservices.BaseDataService[portainer.UserAuthenticationLog, portainer.UserAuthenticationLogID]
}

func NewAuthenticationLogService(connection portainer.Connection) (*AuthenticationLogService, error) {
	if err := connection.SetServiceName(AuthenticationLogBucketName); err != nil {
		return nil, err
	}

	return &AuthenticationLogService{
		BaseDataService: dataservices.BaseDataService[portainer.UserAuthenticationLog, portainer.UserAuthenticationLogID]{
			Bucket:     AuthenticationLogBucketName,
			Connection: connection,
		},
	}, nil
}

func (service *AuthenticationLogService) Create(log *portainer.UserAuthenticationLog) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error {
		return service.Tx(tx).Create(log)
	})
}

func (service *AuthenticationLogService) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(AuthenticationLogBucketName)
}

