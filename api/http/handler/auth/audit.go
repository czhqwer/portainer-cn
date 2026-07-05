package auth

import (
	"net"
	"net/http"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/http/security"

	"github.com/rs/zerolog/log"
)

func (handler *Handler) logAuthentication(r *http.Request, username string, context portainer.AuthenticationMethod, logType portainer.UserAuthenticationLogType) {
	origin := requestOrigin(r)
	logEntry := &portainer.UserAuthenticationLog{
		Timestamp: time.Now().Unix(),
		Context:   context,
		Type:      logType,
		Username:  username,
		Origin:    origin,
	}

	if err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if err := security.CleanExpiredAuditLogs(tx); err != nil {
			return err
		}

		return tx.UserAuthenticationLog().Create(logEntry)
	}); err != nil {
		log.Warn().Err(err).Msg("failed to write user authentication log")
	}
}

func requestOrigin(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := r.Header.Get(header)
		if value == "" {
			continue
		}

		origin := strings.TrimSpace(strings.Split(value, ",")[0])
		if origin != "" {
			return origin
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}

