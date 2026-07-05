package useractivity

import (
	"net/http"

	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/http/security"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"

	"github.com/gorilla/mux"
)

type Handler struct {
	*mux.Router
	DataStore dataservices.DataStore
}

func NewHandler(bouncer security.BouncerService) *Handler {
	h := &Handler{
		Router: mux.NewRouter(),
	}

	h.Handle("/useractivity/authlogs",
		bouncer.AdminAccess(httperror.LoggerHandler(h.authLogs))).Methods(http.MethodGet)
	h.Handle("/useractivity/authlogs.csv",
		bouncer.AdminAccess(httperror.LoggerHandler(h.authLogsCSV))).Methods(http.MethodGet)
	h.Handle("/useractivity/logs",
		bouncer.AdminAccess(httperror.LoggerHandler(h.activityLogs))).Methods(http.MethodGet)
	h.Handle("/useractivity/logs.csv",
		bouncer.AdminAccess(httperror.LoggerHandler(h.activityLogsCSV))).Methods(http.MethodGet)

	return h
}

