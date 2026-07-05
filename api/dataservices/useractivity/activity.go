package useractivity

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const ActivityLogBucketName = "user_activity_logs"

type ActivityLogService struct {
	dataservices.BaseDataService[portainer.UserActivityLog, portainer.UserActivityLogID]
}

func NewActivityLogService(connection portainer.Connection) (*ActivityLogService, error) {
	if err := connection.SetServiceName(ActivityLogBucketName); err != nil {
		return nil, err
	}

	return &ActivityLogService{
		BaseDataService: dataservices.BaseDataService[portainer.UserActivityLog, portainer.UserActivityLogID]{
			Bucket:     ActivityLogBucketName,
			Connection: connection,
		},
	}, nil
}

func (service *ActivityLogService) Create(log *portainer.UserActivityLog) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error {
		return service.Tx(tx).Create(log)
	})
}

func (service *ActivityLogService) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(ActivityLogBucketName)
}

