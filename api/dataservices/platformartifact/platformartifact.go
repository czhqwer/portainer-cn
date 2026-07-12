package platformartifact

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_artifacts"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformArtifact, portainer.PlatformArtifactID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformArtifact, portainer.PlatformArtifactID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformArtifact, portainer.PlatformArtifactID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create 只记录经过模型校验的制品事实，不拉取、构建或修改镜像。
// 把状态默认值和路径安全检查放在 dataservice 中，可避免未来上传/S3 handler 绕过校验直接写入不安全元数据。
func (service *Service) Create(artifact *portainer.PlatformArtifact) error {
	prepareArtifactForPersistence(artifact)
	if err := portainer.ValidatePlatformArtifact(*artifact); err != nil {
		return err
	}

	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			artifact.ID = portainer.PlatformArtifactID(id)
			return int(artifact.ID), artifact
		},
	)
}

func (service *Service) Update(id portainer.PlatformArtifactID, artifact *portainer.PlatformArtifact) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error {
		return service.Tx(tx).Update(id, artifact)
	})
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformArtifact, portainer.PlatformArtifactID]
}

func (service ServiceTx) Create(artifact *portainer.PlatformArtifact) error {
	prepareArtifactForPersistence(artifact)
	if err := portainer.ValidatePlatformArtifact(*artifact); err != nil {
		return err
	}

	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			artifact.ID = portainer.PlatformArtifactID(id)
			return int(artifact.ID), artifact
		},
	)
}

// Update 保持 Import 的 upsert 行为，同时让直接更新也经过制品状态和存储路径校验。
// 这能确保后续任务状态机不会因旧数据或手工调用写入不可识别状态。
func (service ServiceTx) Update(id portainer.PlatformArtifactID, artifact *portainer.PlatformArtifact) error {
	prepareArtifactForPersistence(artifact)
	if err := portainer.ValidatePlatformArtifact(*artifact); err != nil {
		return err
	}

	return service.BaseDataServiceTx.Update(id, artifact)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}

func prepareArtifactForPersistence(artifact *portainer.PlatformArtifact) {
	portainer.NormalizePlatformArtifact(artifact)
	if artifact.LifecycleStatus == "" {
		artifact.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if artifact.ResourceVersion == 0 {
		artifact.ResourceVersion = 1
	}
}
