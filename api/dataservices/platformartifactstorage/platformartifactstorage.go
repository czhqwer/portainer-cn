package platformartifactstorage

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_artifact_storages"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformArtifactStorage, portainer.PlatformArtifactStorageID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformArtifactStorage, portainer.PlatformArtifactStorageID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformArtifactStorage, portainer.PlatformArtifactStorageID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create 只保存已经由上层完成加密的 S3 Compatible 配置。
// dataservice 不接触凭据明文，因此即使后续 adapter 失败，BoltDB 中也不会留下可直接使用的 access key 或 secret key。
func (service *Service) Create(storage *portainer.PlatformArtifactStorage) error {
	prepareStorageForPersistence(storage)
	if err := portainer.ValidatePlatformArtifactStorage(*storage); err != nil {
		return err
	}

	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			storage.ID = portainer.PlatformArtifactStorageID(id)
			return int(storage.ID), storage
		},
	)
}

func (service *Service) Update(id portainer.PlatformArtifactStorageID, storage *portainer.PlatformArtifactStorage) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error {
		return service.Tx(tx).Update(id, storage)
	})
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformArtifactStorage, portainer.PlatformArtifactStorageID]
}

func (service ServiceTx) Create(storage *portainer.PlatformArtifactStorage) error {
	prepareStorageForPersistence(storage)
	if err := portainer.ValidatePlatformArtifactStorage(*storage); err != nil {
		return err
	}

	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			storage.ID = portainer.PlatformArtifactStorageID(id)
			return int(storage.ID), storage
		},
	)
}

// Update 使用 BaseDataServiceTx 的 upsert 语义，保证备份恢复可以在空 bucket 中重建配置。
// 不在这里解密或校验网络连通性，避免 Import 和 BoltDB 事务触发外部 I/O。
func (service ServiceTx) Update(id portainer.PlatformArtifactStorageID, storage *portainer.PlatformArtifactStorage) error {
	prepareStorageForPersistence(storage)
	if err := portainer.ValidatePlatformArtifactStorage(*storage); err != nil {
		return err
	}

	return service.BaseDataServiceTx.Update(id, storage)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}

func prepareStorageForPersistence(storage *portainer.PlatformArtifactStorage) {
	portainer.NormalizePlatformArtifactStorage(storage)
	if storage.LifecycleStatus == "" {
		storage.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if storage.ResourceVersion == 0 {
		storage.ResourceVersion = 1
	}
}
