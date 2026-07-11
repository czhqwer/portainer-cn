package platform

import (
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type secretRevealResponse struct {
	Value string `json:"Value"`
}

// encryptSensitiveConfigEntries transforms request-only plain values before any datastore
// write. 已有密文在普通配置更新时被保留，用户必须显式提交新值或删除条目，避免脱敏响应
// 回写时意外清空密钥。
func (handler *Handler) encryptSensitiveConfigEntries(entries []portainer.PlatformConfigEntry, previous []portainer.PlatformConfigEntry) error {
	previousByKey := make(map[string]portainer.PlatformConfigEntry, len(previous))
	for _, entry := range previous {
		previousByKey[entry.Key] = entry
	}

	var cipher *platformservice.SecretCipher
	for i := range entries {
		entry := &entries[i]
		if !entry.Sensitive {
			entry.CipherText = ""
			entry.EncryptionVersion = ""
			entry.Hash = ""
			entry.HasValue = entry.Value != ""
			continue
		}
		if entry.ValueType != portainer.PlatformConfigValuePlain {
			entry.CipherText = ""
			entry.EncryptionVersion = ""
			if entry.Value == "" {
				if old, found := previousByKey[entry.Key]; found && old.Sensitive && old.ValueType == entry.ValueType {
					entry.Value = old.Value
					entry.Hash = old.Hash
					entry.HasValue = old.HasValue
					continue
				}
			}
			entry.HasValue = entry.Value != ""
			continue
		}

		if entry.Value == "" {
			if old, found := previousByKey[entry.Key]; found && old.Sensitive && old.ValueType == portainer.PlatformConfigValuePlain && old.CipherText != "" {
				entry.CipherText = old.CipherText
				entry.EncryptionVersion = old.EncryptionVersion
				entry.Hash = old.Hash
				entry.HasValue = old.HasValue
			}
			continue
		}

		if cipher == nil {
			var err error
			cipher, err = platformservice.NewSecretCipher(handler.DataStore.Connection())
			if err != nil {
				return err
			}
		}
		cipherText, hash, err := cipher.Encrypt(entry.Value)
		if err != nil {
			return err
		}
		entry.Value = ""
		entry.CipherText = cipherText
		entry.EncryptionVersion = platformservice.PlatformSecretEncryptionVersion
		entry.Hash = hash
		entry.HasValue = true
	}

	return nil
}

func (handler *Handler) configSecretReveal(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	return handler.configSecretRead(w, r, portainer.PlatformAuditActionSecretRevealed)
}

func (handler *Handler) configSecretCopy(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	return handler.configSecretRead(w, r, portainer.PlatformAuditActionSecretCopied)
}

func (handler *Handler) configSecretRead(w http.ResponseWriter, r *http.Request, action portainer.PlatformAuditAction) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "configSetId")
	if handlerErr != nil {
		return handlerErr
	}
	key, err := request.RetrieveRouteVariableValue(r, "key")
	if err != nil || key == "" {
		return validationFailedError("Config entry key is required")
	}
	if _, handlerErr := handler.requireConfigSetPermission(r, portainer.PlatformConfigSetID(id), platformPermissionSensitiveConfig); handlerErr != nil {
		return handlerErr
	}

	var value string
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		configSet, err := readActiveConfigSet(tx, portainer.PlatformConfigSetID(id))
		if err != nil {
			return err
		}
		entry, found := findConfigEntry(configSet.Entries, key)
		if !found || !entry.Sensitive || entry.ValueType != portainer.PlatformConfigValuePlain || entry.CipherText == "" {
			return notFoundError("Sensitive config entry is unavailable")
		}
		cipher, err := platformservice.NewSecretCipher(handler.DataStore.Connection())
		if err != nil {
			return validationFailedError("Encrypted platform secret storage is unavailable")
		}
		value, err = cipher.Decrypt(entry.CipherText)
		if err != nil {
			return validationFailedError("Sensitive config entry cannot be decrypted")
		}

		audit := &portainer.PlatformAuditLog{
			Timestamp:       time.Now().Unix(),
			Action:          action,
			Result:          portainer.PlatformAuditResultSuccess,
			ProjectID:       configSet.ProjectID,
			AfterSummary:    map[string]any{"configSetId": configSet.ID, "key": entry.Key, "hasValue": entry.HasValue},
			SensitiveFields: []string{entry.Key},
		}
		handler.fillPlatformAuditRequestFields(tx, r, audit)
		return tx.PlatformAuditLog().Create(audit)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, secretRevealResponse{Value: value})
}

func findConfigEntry(entries []portainer.PlatformConfigEntry, key string) (portainer.PlatformConfigEntry, bool) {
	for _, entry := range entries {
		if entry.Key == key {
			return entry, true
		}
	}
	return portainer.PlatformConfigEntry{}, false
}

func redactReleases(releases []portainer.PlatformRelease) []portainer.PlatformRelease {
	result := make([]portainer.PlatformRelease, len(releases))
	for i := range releases {
		result[i] = redactRelease(releases[i])
	}
	return result
}

// redactRelease removes release-snapshot ciphertext before any list or detail response.
// 密文只允许在服务端发布/回滚执行链路中使用；即使调用方有管理员权限，也不能通过普通
// Release API 获取可离线攻击的密文材料。
func redactRelease(release portainer.PlatformRelease) portainer.PlatformRelease {
	result := release
	result.ConfigSnapshot.SecretSnapshots = append([]portainer.PlatformSecretSnapshot(nil), release.ConfigSnapshot.SecretSnapshots...)
	for i := range result.ConfigSnapshot.SecretSnapshots {
		result.ConfigSnapshot.SecretSnapshots[i].CipherText = ""
	}
	return result
}
