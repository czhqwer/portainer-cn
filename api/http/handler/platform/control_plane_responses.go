package platform

import (
	"net/http"

	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type platformErrorDetails struct {
	Reason string `json:"reason,omitempty"`
	Field  string `json:"field,omitempty"`
}

type platformErrorResponse struct {
	Code    string               `json:"code"`
	Message string               `json:"message"`
	Details platformErrorDetails `json:"details"`
	Data    any                  `json:"data,omitempty"`
}

func writePlatformError(w http.ResponseWriter, status int, code string, message string, reason string, data any) *httperror.HandlerError {
	return response.JSONWithStatus(w, platformErrorResponse{
		Code:    code,
		Message: message,
		Details: platformErrorDetails{Reason: reason},
		Data:    data,
	}, status)
}
