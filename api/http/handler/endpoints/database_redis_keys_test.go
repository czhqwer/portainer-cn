package endpoints

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portainer/portainer/api/internal/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestDatabaseConnectionRedisKeyDetailsRouteIsRemoved(t *testing.T) {
	t.Parallel()

	handler := NewHandler(testhelpers.NewTestRequestBouncer())
	request := httptest.NewRequest(http.MethodGet, "/endpoints/1/database-connections/1/redis-key-details", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
}
