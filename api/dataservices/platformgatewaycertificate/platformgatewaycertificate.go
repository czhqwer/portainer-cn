package platformgatewaycertificate

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

const BucketName = "platform_gateway_certificates"

type Service struct {
	dataservices.BaseDataService[portainer.PlatformGatewayCertificate, portainer.PlatformGatewayCertificateID]
}

func NewService(connection portainer.Connection) (*Service, error) {
	if err := connection.SetServiceName(BucketName); err != nil {
		return nil, err
	}
	return &Service{BaseDataService: dataservices.BaseDataService[portainer.PlatformGatewayCertificate, portainer.PlatformGatewayCertificateID]{Bucket: BucketName, Connection: connection}}, nil
}
func (service *Service) Tx(tx portainer.Transaction) ServiceTx {
	return ServiceTx{BaseDataServiceTx: dataservices.BaseDataServiceTx[portainer.PlatformGatewayCertificate, portainer.PlatformGatewayCertificateID]{Bucket: BucketName, Connection: service.Connection, Tx: tx}}
}
func (service *Service) Create(certificate *portainer.PlatformGatewayCertificate) error {
	prepareForPersistence(certificate)
	if err := portainer.ValidatePlatformGatewayCertificate(*certificate); err != nil {
		return err
	}
	return service.Connection.CreateObject(BucketName, func(id uint64) (int, any) {
		certificate.ID = portainer.PlatformGatewayCertificateID(id)
		return int(certificate.ID), certificate
	})
}
func (service *Service) Update(id portainer.PlatformGatewayCertificateID, certificate *portainer.PlatformGatewayCertificate) error {
	return service.Connection.UpdateTx(func(tx portainer.Transaction) error { return service.Tx(tx).Update(id, certificate) })
}
func (service *Service) GetNextIdentifier() int {
	return service.Connection.GetNextIdentifier(BucketName)
}

type ServiceTx struct {
	dataservices.BaseDataServiceTx[portainer.PlatformGatewayCertificate, portainer.PlatformGatewayCertificateID]
}

func (service ServiceTx) Create(certificate *portainer.PlatformGatewayCertificate) error {
	prepareForPersistence(certificate)
	if err := portainer.ValidatePlatformGatewayCertificate(*certificate); err != nil {
		return err
	}
	return service.Tx.CreateObject(BucketName, func(id uint64) (int, any) {
		certificate.ID = portainer.PlatformGatewayCertificateID(id)
		return int(certificate.ID), certificate
	})
}
func (service ServiceTx) Update(id portainer.PlatformGatewayCertificateID, certificate *portainer.PlatformGatewayCertificate) error {
	prepareForPersistence(certificate)
	if err := portainer.ValidatePlatformGatewayCertificate(*certificate); err != nil {
		return err
	}
	return service.BaseDataServiceTx.Update(id, certificate)
}
func (service ServiceTx) GetNextIdentifier() int { return service.Tx.GetNextIdentifier(BucketName) }
func prepareForPersistence(certificate *portainer.PlatformGatewayCertificate) {
	portainer.NormalizePlatformGatewayCertificate(certificate)
	if certificate.LifecycleStatus == "" {
		certificate.LifecycleStatus = portainer.PlatformLifecycleStatusActive
	}
	if certificate.ResourceVersion == 0 {
		certificate.ResourceVersion = 1
	}
}
