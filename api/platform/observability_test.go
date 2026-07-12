package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObservabilityAdapterUsesFixedTemplateAndCapsSeries(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/query_range", r.URL.Path)
		query := r.URL.Query().Get("query")
		require.Contains(t, query, "platform_service_deployment_id")
		require.NotContains(t, query, "user supplied query")
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{}, {}, {}]}}`))
	}))
	defer server.Close()

	adapter := NewHTTPObservabilityAdapter(server.Client())
	result, err := adapter.Query(context.Background(), ObservabilityQuery{
		Template:            ObservabilityTemplateServiceAvailability,
		PrometheusURL:       server.URL,
		ServiceDeploymentID: 1,
		Start:               100,
		End:                 160,
	})

	require.NoError(t, err)
	require.LessOrEqual(t, len(result.Series), MaxObservabilitySeries)
}
