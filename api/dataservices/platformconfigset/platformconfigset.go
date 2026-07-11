package platformconfigset

import (
	"reflect"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_config_sets"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformConfigSet, portainer.PlatformConfigSetID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}

	return &Service{
		BaseDataService: dataservices.BaseDataService[portainer.PlatformConfigSet, portainer.PlatformConfigSetID]{
			Bucket:     BucketName,
			Connection: connection,
		},
	}, nil
}

func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{
		BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformConfigSet, portainer.PlatformConfigSetID]{
			Bucket:     BucketName,
			Connection: service.Connection,
			Tx:         tx,
		},
	}
}

// Create saves only validated configuration metadata. 敏感 plain 值在加密能力尚未接入前
// 会被模型校验拒绝，避免先建立 BoltDB bucket 后出现无法修复的明文历史数据。
func (service *Service) Create(configSet *portainer.PlatformConfigSet) error {
	prepareConfigSetForStorage(configSet)
	if err := portainer.ValidatePlatformConfigSet(*configSet); err != nil {
		return err
	}

	return service.Connection.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			configSet.ID = portainer.PlatformConfigSetID(id)
			return int(configSet.ID), configSet
		},
	)
}

func (service *Service) Update(id portainer.PlatformConfigSetID, configSet *portainer.PlatformConfigSet) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error {
		return service.Tx(tx).Update(id, configSet)
	})
}

func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformConfigSet, portainer.PlatformConfigSetID]
}

// Create provides the same metadata validation for callers that compose platform changes
// in a transaction, so direct and transactional persistence cannot diverge in revision
// defaults or sensitive-value safety.
func (service ServiceTx) Create(configSet *portainer.PlatformConfigSet) error {
	prepareConfigSetForStorage(configSet)
	if err := portainer.ValidatePlatformConfigSet(*configSet); err != nil {
		return err
	}

	return service.Tx.CreateObject(
		BucketName,
		func(id uint64) (int, any) {
			configSet.ID = portainer.PlatformConfigSetID(id)
			return int(configSet.ID), configSet
		},
	)
}

// Update increments Revision only when configuration metadata changes. 归档等生命周期操作
// 不会制造新的有效配置版本，避免服务误判为需要重新部署；实际条目变化才会让后续发布
// 快照与漂移检测观察到新的 revision。
func (service ServiceTx) Update(id portainer.PlatformConfigSetID, configSet *portainer.PlatformConfigSet) error {
	current, err := service.BaseDataServiceTx.Read(id)
	prepareConfigSetForStorage(configSet)
	if err := portainer.ValidatePlatformConfigSet(*configSet); err != nil {
		return err
	}
	if err != nil {
		// datastore Import 使用 Update 作为可重复导入的 upsert；新 bucket 中还没有
		// 旧记录时不能因为 revision 比较而拒绝恢复历史配置集。
		return service.BaseDataServiceTx.Update(id, configSet)
	}

	if configSetContentChanged(*current, *configSet) {
		currentRevision := current.Revision
		if currentRevision == 0 {
			currentRevision = 1
		}
		configSet.Revision = currentRevision + 1
	} else {
		configSet.Revision = current.Revision
	}

	return service.BaseDataServiceTx.Update(id, configSet)
}

func (service ServiceTx) GetNextIdentifier() int {
	return service.Tx.GetNextIdentifier(BucketName)
}

func prepareConfigSetForStorage(configSet *portainer.PlatformConfigSet) {
	portainer.NormalizePlatformConfigSet(configSet)
	if configSet.LifecycleStatus == "" {
		configSet.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if configSet.ResourceVersion == 0 {
		configSet.ResourceVersion = 1
	}
}

func configSetContentChanged(current, next portainer.PlatformConfigSet) bool {
	return !reflect.DeepEqual(
		struct {
			ProjectID portainer.PlatformProjectID
			ScopeType portainer.PlatformConfigScopeType
			ScopeID   int
			Name      string
			Entries   []portainer.PlatformConfigEntry
		}{
			ProjectID: current.ProjectID,
			ScopeType: current.ScopeType,
			ScopeID:   current.ScopeID,
			Name:      current.Name,
			Entries:   current.Entries,
		},
		struct {
			ProjectID portainer.PlatformProjectID
			ScopeType portainer.PlatformConfigScopeType
			ScopeID   int
			Name      string
			Entries   []portainer.PlatformConfigEntry
		}{
			ProjectID: next.ProjectID,
			ScopeType: next.ScopeType,
			ScopeID:   next.ScopeID,
			Name:      next.Name,
			Entries:   next.Entries,
		},
	)
}
