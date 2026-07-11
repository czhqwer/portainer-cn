package platformreleaselock

import (
	"fmt"
	"strconv"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_release_locks"

type Service struct {
	connection portainer.Connection
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{connection: connection}, nil
}

func (service *Service) BucketName() string {
	return BucketName
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		service: service,
		tx:      tx,
	}
}

// LockIDForServiceDeployment keeps the lock key stable per ServiceDeployment,
// matching the one-active-release-per-deployment rule in the platform design.
func LockIDForServiceDeployment(id portainer.PlatformServiceDeploymentID) string {
	return strconv.Itoa(int(id))
}

func (service *Service) Create(lock *portainer.PlatformReleaseLock) error {
	return service.connection.UpdateTx(func(tx portainer.Transaction) error {
		return service.Tx(tx).Create(lock)
	})
}

func (service *Service) Read(id string) (*portainer.PlatformReleaseLock, error) {
	var lock *portainer.PlatformReleaseLock

	return lock, service.connection.ViewTx(func(tx portainer.Transaction) error {
		var err error
		lock, err = service.Tx(tx).Read(id)
		return err
	})
}

func (service *Service) Exists(id string) (bool, error) {
	var exists bool

	err := service.connection.ViewTx(func(tx portainer.Transaction) error {
		var err error
		exists, err = service.Tx(tx).Exists(id)
		return err
	})

	return exists, err
}

func (service *Service) ReadAll(predicates ...func(portainer.PlatformReleaseLock) bool) ([]portainer.PlatformReleaseLock, error) {
	locks := make([]portainer.PlatformReleaseLock, 0)

	return locks, service.connection.ViewTx(func(tx portainer.Transaction) error {
		var err error
		locks, err = service.Tx(tx).ReadAll(predicates...)
		return err
	})
}

func (service *Service) Update(id string, lock *portainer.PlatformReleaseLock) error {
	return service.connection.UpdateTx(func(tx portainer.Transaction) error {
		return service.Tx(tx).Update(id, lock)
	})
}

func (service *Service) Delete(id string) error {
	return service.connection.UpdateTx(func(tx portainer.Transaction) error {
		return service.Tx(tx).Delete(id)
	})
}

type ServiceTx struct {
	service *Service
	tx      portainer.Transaction
}

func (service ServiceTx) BucketName() string {
	return BucketName
}

func (service ServiceTx) Create(lock *portainer.PlatformReleaseLock) error {
	if err := ensureLockID(lock); err != nil {
		return err
	}

	return service.tx.CreateObjectWithStringId(BucketName, []byte(lock.ID), lock)
}

func (service ServiceTx) Read(id string) (*portainer.PlatformReleaseLock, error) {
	var lock portainer.PlatformReleaseLock

	if err := service.tx.GetObject(BucketName, []byte(id), &lock); err != nil {
		return nil, err
	}

	return &lock, nil
}

func (service ServiceTx) Exists(id string) (bool, error) {
	return service.tx.KeyExists(BucketName, []byte(id))
}

func (service ServiceTx) ReadAll(predicates ...func(portainer.PlatformReleaseLock) bool) ([]portainer.PlatformReleaseLock, error) {
	locks := make([]portainer.PlatformReleaseLock, 0)

	if len(predicates) == 0 {
		return locks, service.tx.GetAll(BucketName, &portainer.PlatformReleaseLock{}, dataservices.AppendFn(&locks))
	}

	filterFn := func(lock portainer.PlatformReleaseLock) bool {
		for _, predicate := range predicates {
			if !predicate(lock) {
				return false
			}
		}

		return true
	}

	return locks, service.tx.GetAll(BucketName, &portainer.PlatformReleaseLock{}, dataservices.FilterFn(&locks, filterFn))
}

func (service ServiceTx) Update(id string, lock *portainer.PlatformReleaseLock) error {
	if lock.ID == "" {
		lock.ID = id
	}

	if err := ensureLockID(lock); err != nil {
		return err
	}

	return service.tx.UpdateObject(BucketName, []byte(id), lock)
}

func (service ServiceTx) Delete(id string) error {
	return service.tx.DeleteObject(BucketName, []byte(id))
}

func ensureLockID(lock *portainer.PlatformReleaseLock) error {
	if lock == nil {
		return fmt.Errorf("platform release lock is nil")
	}

	if lock.ID == "" {
		if lock.ServiceDeploymentID == 0 {
			return fmt.Errorf("platform release lock requires an ID or ServiceDeploymentID")
		}
		lock.ID = LockIDForServiceDeployment(lock.ServiceDeploymentID)
	}

	return nil
}
