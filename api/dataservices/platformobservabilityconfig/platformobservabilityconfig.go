package platformobservabilityconfig

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_observability_configs"

// Service 只保存已加密的观测系统凭据；连接探测和实际查询必须在 handler 的事务外执行。
type Service struct {
	dataservices.BaseDataService[portainer.PlatformObservabilityConfig, portainer.PlatformObservabilityConfigID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}
	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformObservabilityConfig, portainer.PlatformObservabilityConfigID]{Bucket: BucketName, Connection: connection}}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformObservabilityConfig, portainer.PlatformObservabilityConfigID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}

func (service *Service) Create(config *portainer.PlatformObservabilityConfig) error {
	prepare(config)
	if err := portainer.ValidatePlatformObservabilityConfig(*config); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		config.ID = portainer.PlatformObservabilityConfigID(id)
		return int(config.ID), config
	})
}

func (service *Service) Update(id portainer.PlatformObservabilityConfigID, config *portainer.PlatformObservabilityConfig) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, config) })
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformObservabilityConfig, portainer.PlatformObservabilityConfigID]
}

func (service ServiceTx) Create(config *portainer.PlatformObservabilityConfig) error {
	prepare(config)
	if err := portainer.ValidatePlatformObservabilityConfig(*config); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		config.ID = portainer.PlatformObservabilityConfigID(id)
		return int(config.ID), config
	})
}

func (service ServiceTx) Update(id portainer.PlatformObservabilityConfigID, config *portainer.PlatformObservabilityConfig) error {
	previous, err := service.BaseDataServiceTx.Read(id)
	prepare(config)
	if err := portainer.ValidatePlatformObservabilityConfig(*config); err != nil {
		return err
	}
	if err == nil && contentChanged(*previous, *config) {
		config.Revision = previous.Revision + 1
	}
	return service.BaseDataServiceTx.Update(id, config)
}

func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }

func prepare(config *portainer.PlatformObservabilityConfig) {
	portainer.NormalizePlatformObservabilityConfig(config)
	if config.LifecycleStatus == "" {
		config.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if config.ResourceVersion == 0 {
		config.ResourceVersion = 1
	}
}

func contentChanged(previous, next portainer.PlatformObservabilityConfig) bool {
	return previous.PrometheusURL != next.PrometheusURL || previous.LokiURL != next.LokiURL || previous.GrafanaURL != next.GrafanaURL || previous.BearerTokenCipherText != next.BearerTokenCipherText || previous.CredentialEncryptionVersion != next.CredentialEncryptionVersion || previous.CredentialHash != next.CredentialHash || previous.HasCredentials != next.HasCredentials || previous.Enabled != next.Enabled
}
