package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	ObservabilityTemplateServiceAvailability = "service-availability"
	ObservabilityTemplateServiceLatency      = "service-latency"
	ObservabilityTemplateServiceLogs         = "service-logs"
	MaxObservabilitySeries                   = 100
	MaxObservabilityLogLines                 = 200
	maxObservabilityRange                    = 24 * time.Hour
	observabilityRequestTimeout              = 30 * time.Second
	maxObservabilityResponseBytes            = 1024 * 1024

	ObservabilityReasonInvalidRequest = "OBSERVABILITY_INVALID_REQUEST"
	ObservabilityReasonTimeout        = "OBSERVABILITY_TIMEOUT"
	ObservabilityReasonNetwork        = "OBSERVABILITY_NETWORK_ERROR"
	ObservabilityReasonUpstream       = "OBSERVABILITY_UPSTREAM_ERROR"
	ObservabilityReasonResponse       = "OBSERVABILITY_INVALID_RESPONSE"
)

type ObservabilityQuery struct {
	Template            string
	PrometheusURL       string
	LokiURL             string
	BearerToken         string
	ServiceDeploymentID int
	Start               int64
	End                 int64
}

type ObservabilityResult struct {
	Template string            `json:"Template"`
	Series   []json.RawMessage `json:"Series,omitempty"`
	LogLines []string          `json:"LogLines,omitempty"`
}

// ObservabilityAdapter 只提供后端固定模板查询，避免把表达式语言作为平台 API 的透传能力。
type ObservabilityAdapter interface {
	Query(context.Context, ObservabilityQuery) (ObservabilityResult, error)
}

type HTTPObservabilityAdapter struct {
	client *http.Client
}

func NewHTTPObservabilityAdapter(client *http.Client) *HTTPObservabilityAdapter {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPObservabilityAdapter{client: client}
}

func (adapter *HTTPObservabilityAdapter) Query(ctx context.Context, query ObservabilityQuery) (ObservabilityResult, error) {
	if err := validateObservabilityQuery(query); err != nil {
		return ObservabilityResult{}, newObservabilityError(ObservabilityReasonInvalidRequest, err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, observabilityRequestTimeout)
	defer cancel()

	switch query.Template {
	case ObservabilityTemplateServiceAvailability, ObservabilityTemplateServiceLatency:
		return adapter.queryPrometheus(requestCtx, query)
	case ObservabilityTemplateServiceLogs:
		return adapter.queryLoki(requestCtx, query)
	default:
		return ObservabilityResult{}, newObservabilityError(ObservabilityReasonInvalidRequest, fmt.Errorf("observability template is invalid"))
	}
}

func validateObservabilityQuery(query ObservabilityQuery) error {
	if query.ServiceDeploymentID <= 0 || query.Start < 0 || query.End <= query.Start || time.Duration(query.End-query.Start)*time.Second > maxObservabilityRange {
		return fmt.Errorf("observability query range is invalid")
	}
	endpoint := query.PrometheusURL
	if query.Template == ObservabilityTemplateServiceLogs {
		endpoint = query.LokiURL
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("observability endpoint is invalid")
	}
	return nil
}

func (adapter *HTTPObservabilityAdapter) queryPrometheus(ctx context.Context, query ObservabilityQuery) (ObservabilityResult, error) {
	endpoint, _ := url.Parse(query.PrometheusURL)
	endpoint.Path = joinObservabilityAPIPath(endpoint.Path, "/api/v1/query_range")
	parameters := endpoint.Query()
	parameters.Set("query", prometheusTemplate(query.Template, query.ServiceDeploymentID))
	parameters.Set("start", strconv.FormatInt(query.Start, 10))
	parameters.Set("end", strconv.FormatInt(query.End, 10))
	parameters.Set("step", "60")
	endpoint.RawQuery = parameters.Encode()

	body, err := adapter.doRequest(ctx, endpoint.String(), query.BearerToken)
	if err != nil {
		return ObservabilityResult{}, err
	}
	var response struct {
		Status string `json:"status"`
		Data   struct {
			Result []json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Status != "success" {
		return ObservabilityResult{}, newObservabilityError(ObservabilityReasonResponse, fmt.Errorf("prometheus response is invalid"))
	}
	if len(response.Data.Result) > MaxObservabilitySeries {
		response.Data.Result = response.Data.Result[:MaxObservabilitySeries]
	}
	return ObservabilityResult{Template: query.Template, Series: response.Data.Result}, nil
}

func (adapter *HTTPObservabilityAdapter) queryLoki(ctx context.Context, query ObservabilityQuery) (ObservabilityResult, error) {
	endpoint, _ := url.Parse(query.LokiURL)
	endpoint.Path = joinObservabilityAPIPath(endpoint.Path, "/loki/api/v1/query_range")
	parameters := endpoint.Query()
	parameters.Set("query", fmt.Sprintf(`{platform_service_deployment_id="%d"}`, query.ServiceDeploymentID))
	parameters.Set("start", strconv.FormatInt(query.Start*int64(time.Second), 10))
	parameters.Set("end", strconv.FormatInt(query.End*int64(time.Second), 10))
	parameters.Set("limit", strconv.Itoa(MaxObservabilityLogLines))
	parameters.Set("direction", "BACKWARD")
	endpoint.RawQuery = parameters.Encode()

	body, err := adapter.doRequest(ctx, endpoint.String(), query.BearerToken)
	if err != nil {
		return ObservabilityResult{}, err
	}
	var response struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Values [][]json.RawMessage `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Status != "success" {
		return ObservabilityResult{}, newObservabilityError(ObservabilityReasonResponse, fmt.Errorf("loki response is invalid"))
	}
	lines := make([]string, 0, MaxObservabilityLogLines)
	for _, stream := range response.Data.Result {
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}
			var line string
			if json.Unmarshal(value[1], &line) == nil {
				lines = append(lines, line)
			}
			if len(lines) == MaxObservabilityLogLines {
				return ObservabilityResult{Template: query.Template, LogLines: lines}, nil
			}
		}
	}
	return ObservabilityResult{Template: query.Template, LogLines: lines}, nil
}

func (adapter *HTTPObservabilityAdapter) doRequest(ctx context.Context, endpoint, bearerToken string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, newObservabilityError(ObservabilityReasonInvalidRequest, err)
	}
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	response, err := adapter.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, newObservabilityError(ObservabilityReasonTimeout, ctx.Err())
		}
		return nil, newObservabilityError(ObservabilityReasonNetwork, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, newObservabilityError(ObservabilityReasonUpstream, fmt.Errorf("observability response status is invalid"))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxObservabilityResponseBytes+1))
	if err != nil || len(body) > maxObservabilityResponseBytes {
		return nil, newObservabilityError(ObservabilityReasonResponse, fmt.Errorf("observability response is too large or unreadable"))
	}
	return body, nil
}

func prometheusTemplate(template string, deploymentID int) string {
	switch template {
	case ObservabilityTemplateServiceAvailability:
		return fmt.Sprintf(`avg_over_time(up{platform_service_deployment_id="%d"}[5m])`, deploymentID)
	case ObservabilityTemplateServiceLatency:
		return fmt.Sprintf(`histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{platform_service_deployment_id="%d"}[5m])) by (le))`, deploymentID)
	default:
		return ""
	}
}

func joinObservabilityAPIPath(base, apiPath string) string {
	if base == "" || base == "/" {
		return apiPath
	}
	return base + apiPath
}

type observabilityError struct {
	reason string
	err    error
}

func (err *observabilityError) Error() string { return err.err.Error() }
func (err *observabilityError) Unwrap() error { return err.err }

func ObservabilityErrorReason(err error) string {
	if typed, ok := err.(*observabilityError); ok {
		return typed.reason
	}
	return ObservabilityReasonResponse
}

func newObservabilityError(reason string, err error) error {
	return &observabilityError{reason: reason, err: err}
}
