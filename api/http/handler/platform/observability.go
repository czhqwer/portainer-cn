package platform

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type observabilityConfigPayload struct {
	ResourceVersion int     `json:"ResourceVersion"`
	PrometheusURL   string  `json:"PrometheusUrl"`
	LokiURL         string  `json:"LokiUrl"`
	GrafanaURL      string  `json:"GrafanaUrl"`
	BearerToken     *string `json:"BearerToken"`
	Enabled         bool    `json:"Enabled"`
}

type observabilityConfigResponse struct {
	ID                    portainer.PlatformObservabilityConfigID `json:"Id"`
	PrometheusURL         string                                  `json:"PrometheusUrl,omitempty"`
	LokiURL               string                                  `json:"LokiUrl,omitempty"`
	GrafanaURL            string                                  `json:"GrafanaUrl,omitempty"`
	CredentialsConfigured bool                                    `json:"CredentialsConfigured"`
	Enabled               bool                                    `json:"Enabled"`
	Revision              int                                     `json:"Revision"`
	portainer.PlatformLifecycle
}

func (payload *observabilityConfigPayload) Validate(*http.Request) error {
	payload.PrometheusURL = strings.TrimSpace(payload.PrometheusURL)
	payload.LokiURL = strings.TrimSpace(payload.LokiURL)
	payload.GrafanaURL = strings.TrimSpace(payload.GrafanaURL)
	if payload.PrometheusURL == "" && payload.LokiURL == "" && payload.GrafanaURL == "" {
		return errors.New("at least one observability endpoint is required")
	}
	return nil
}

// observabilityConfigInspect 将凭据状态而非密文返回给全局管理员，避免诊断配置本身成为令牌读取接口。
func (handler *Handler) observabilityConfigInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	if handlerErr := handler.requireGlobalAdmin(r); handlerErr != nil {
		return handlerErr
	}
	config, err := handler.currentObservabilityConfig()
	if err != nil {
		return handler.convertError(err)
	}
	if config == nil {
		return response.JSON(w, nil)
	}
	return response.JSON(w, redactObservabilityConfig(*config))
}

func (handler *Handler) observabilityConfigUpsert(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	if handlerErr := handler.requireGlobalAdmin(r); handlerErr != nil {
		return handlerErr
	}
	var payload observabilityConfigPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	var saved *portainer.PlatformObservabilityConfig
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		configs, err := tx.PlatformObservabilityConfig().ReadAll(func(config portainer.PlatformObservabilityConfig) bool { return isActive(config.PlatformLifecycle) })
		if err != nil && !handler.DataStore.IsErrObjectNotFound(err) {
			return err
		}
		if len(configs) > 1 {
			return conflictError("Multiple active observability configurations exist")
		}
		var config portainer.PlatformObservabilityConfig
		if len(configs) == 0 {
			config = portainer.NewPlatformObservabilityConfig()
			config.PlatformLifecycle = newLifecycle(time.Now().Unix())
		} else {
			config = configs[0]
			if payload.ResourceVersion > 0 {
				if err := requireResourceVersion(config.ResourceVersion, payload.ResourceVersion); err != nil {
					return err
				}
			}
		}
		before := config
		config.PrometheusURL = payload.PrometheusURL
		config.LokiURL = payload.LokiURL
		config.GrafanaURL = payload.GrafanaURL
		config.Enabled = payload.Enabled
		if err := handler.setObservabilityToken(&config, payload.BearerToken); err != nil {
			return validationFailedError("Encrypted observability credential storage is unavailable")
		}
		touchLifecycle(&config.PlatformLifecycle, time.Now().Unix())
		if len(configs) == 0 {
			if err := tx.PlatformObservabilityConfig().Create(&config); err != nil {
				return err
			}
		} else if err := tx.PlatformObservabilityConfig().Update(config.ID, &config); err != nil {
			return err
		}
		action := portainer.PlatformAuditActionObservabilityConfigured
		if err := handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: action, Result: portainer.PlatformAuditResultSuccess, BeforeSummary: observabilityAuditSummary(before), AfterSummary: observabilityAuditSummary(config), SensitiveFields: []string{"bearerToken"}}); err != nil {
			return err
		}
		saved = &config
		return nil
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, redactObservabilityConfig(*saved))
}

// observabilityQuery 将项目、环境和 Endpoint 三层权限求交后才调用外部观测系统；网络 I/O
// 必须位于 BoltDB 事务之外，防止慢查询占用控制面写锁。
func (handler *Handler) observabilityQuery(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionView); handlerErr != nil {
		return handlerErr
	}
	environmentID, deploymentID, start, end, template, err := parseObservabilityQuery(r)
	if err != nil {
		return validationFailed(err)
	}
	environment, handlerErr := handler.requireEnvironmentPermission(r, environmentID, platformPermissionView)
	if handlerErr != nil || environment.ProjectID != portainer.PlatformProjectID(projectID) {
		return platformAccessDenied()
	}
	if handlerErr := handler.requireEnvironmentEndpointAccess(r, environment); handlerErr != nil {
		return handlerErr
	}
	deployment, err := handler.DataStore.PlatformServiceDeployment().Read(deploymentID)
	if err != nil || deployment.ProjectID != portainer.PlatformProjectID(projectID) || deployment.EnvironmentID != environment.ID {
		return platformAccessDenied()
	}
	config, err := handler.currentObservabilityConfig()
	if err != nil || config == nil || !config.Enabled {
		return validationFailedError("Observability integration is unavailable")
	}
	token, err := handler.observabilityToken(*config)
	if err != nil {
		return validationFailedError("Observability credentials are unavailable")
	}
	if handler.ObservabilityAdapter == nil {
		return validationFailedError("Observability integration is unavailable")
	}
	result, queryErr := handler.ObservabilityAdapter.Query(r.Context(), platformservice.ObservabilityQuery{Template: template, PrometheusURL: config.PrometheusURL, LokiURL: config.LokiURL, BearerToken: token, ServiceDeploymentID: int(deployment.ID), Start: start, End: end})
	reason := ""
	resultStatus := portainer.PlatformAuditResultSuccess
	if queryErr != nil {
		reason = platformservice.ObservabilityErrorReason(queryErr)
		resultStatus = portainer.PlatformAuditResultFailed
	}
	_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionObservabilityQueried, Result: resultStatus, FailureReason: reason, ProjectID: portainer.PlatformProjectID(projectID), EnvironmentID: environment.ID, ServiceDeploymentID: deployment.ID, AfterSummary: map[string]any{"template": template, "seriesCount": len(result.Series), "logLineCount": len(result.LogLines), "status": resultStatus}})
	})
	if queryErr != nil {
		return validationFailedError("Observability query failed")
	}
	return response.JSON(w, result)
}

func (handler *Handler) currentObservabilityConfig() (*portainer.PlatformObservabilityConfig, error) {
	configs, err := handler.DataStore.PlatformObservabilityConfig().ReadAll(func(config portainer.PlatformObservabilityConfig) bool { return isActive(config.PlatformLifecycle) })
	if err != nil && !handler.DataStore.IsErrObjectNotFound(err) {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}
	if len(configs) != 1 {
		return nil, conflictError("Multiple active observability configurations exist")
	}
	return &configs[0], nil
}

func (handler *Handler) setObservabilityToken(config *portainer.PlatformObservabilityConfig, token *string) error {
	if token == nil {
		return nil
	}
	if *token == "" {
		config.BearerTokenCipherText, config.CredentialEncryptionVersion, config.CredentialHash, config.HasCredentials = "", "", "", false
		return nil
	}
	cipher, err := platformservice.NewSecretCipher(handler.DataStore.Connection())
	if err != nil {
		return err
	}
	cipherText, hash, err := cipher.Encrypt(*token)
	if err != nil {
		return err
	}
	config.BearerTokenCipherText = cipherText
	config.CredentialEncryptionVersion = portainer.PlatformObservabilityCredentialEncryptionVersion
	config.CredentialHash = hash
	config.HasCredentials = true
	return nil
}

func (handler *Handler) observabilityToken(config portainer.PlatformObservabilityConfig) (string, error) {
	if !config.HasCredentials {
		return "", nil
	}
	cipher, err := platformservice.NewSecretCipher(handler.DataStore.Connection())
	if err != nil {
		return "", err
	}
	return cipher.Decrypt(config.BearerTokenCipherText)
}

func redactObservabilityConfig(config portainer.PlatformObservabilityConfig) observabilityConfigResponse {
	return observabilityConfigResponse{ID: config.ID, PrometheusURL: config.PrometheusURL, LokiURL: config.LokiURL, GrafanaURL: config.GrafanaURL, CredentialsConfigured: config.HasCredentials, Enabled: config.Enabled, Revision: config.Revision, PlatformLifecycle: config.PlatformLifecycle}
}

func observabilityAuditSummary(config portainer.PlatformObservabilityConfig) map[string]any {
	return map[string]any{"observabilityConfigId": config.ID, "enabled": config.Enabled, "prometheusConfigured": config.PrometheusURL != "", "lokiConfigured": config.LokiURL != "", "grafanaConfigured": config.GrafanaURL != "", "credentialsConfigured": config.HasCredentials}
}

func parseObservabilityQuery(r *http.Request) (portainer.PlatformEnvironmentID, portainer.PlatformServiceDeploymentID, int64, int64, string, error) {
	values := r.URL.Query()
	environmentID, err := strconv.Atoi(values.Get("environmentId"))
	if err != nil || environmentID <= 0 {
		return 0, 0, 0, 0, "", errors.New("environment ID is required")
	}
	deploymentID, err := strconv.Atoi(values.Get("serviceDeploymentId"))
	if err != nil || deploymentID <= 0 {
		return 0, 0, 0, 0, "", errors.New("service deployment ID is required")
	}
	start, err := strconv.ParseInt(values.Get("start"), 10, 64)
	if err != nil {
		return 0, 0, 0, 0, "", errors.New("start is required")
	}
	end, err := strconv.ParseInt(values.Get("end"), 10, 64)
	if err != nil {
		return 0, 0, 0, 0, "", errors.New("end is required")
	}
	template := values.Get("template")
	if template != platformservice.ObservabilityTemplateServiceAvailability && template != platformservice.ObservabilityTemplateServiceLatency && template != platformservice.ObservabilityTemplateServiceLogs {
		return 0, 0, 0, 0, "", errors.New("observability template is invalid")
	}
	return portainer.PlatformEnvironmentID(environmentID), portainer.PlatformServiceDeploymentID(deploymentID), start, end, template, nil
}
