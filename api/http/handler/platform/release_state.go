package platform

import (
	"fmt"

	portainer "github.com/portainer/portainer/api"
)

var platformReleaseTransitions = map[portainer.PlatformReleaseStatus]map[portainer.PlatformReleaseStatus]bool{
	portainer.PlatformReleaseStatusQueued: {
		portainer.PlatformReleaseStatusValidating: true,
		portainer.PlatformReleaseStatusCanceled:   true,
	},
	portainer.PlatformReleaseStatusValidating: {
		portainer.PlatformReleaseStatusPulling:  true,
		portainer.PlatformReleaseStatusFailed:   true,
		portainer.PlatformReleaseStatusCanceled: true,
	},
	portainer.PlatformReleaseStatusPulling: {
		portainer.PlatformReleaseStatusPreparing: true,
		portainer.PlatformReleaseStatusFailed:    true,
		portainer.PlatformReleaseStatusCanceled:  true,
	},
	portainer.PlatformReleaseStatusPreparing: {
		portainer.PlatformReleaseStatusCandidateStarting: true,
		portainer.PlatformReleaseStatusFailed:            true,
		portainer.PlatformReleaseStatusCanceled:          true,
	},
	portainer.PlatformReleaseStatusCandidateStarting: {
		portainer.PlatformReleaseStatusCandidateChecking: true,
		portainer.PlatformReleaseStatusRecovering:        true,
		portainer.PlatformReleaseStatusFailed:            true,
	},
	portainer.PlatformReleaseStatusCandidateChecking: {
		portainer.PlatformReleaseStatusSwitching:  true,
		portainer.PlatformReleaseStatusRecovering: true,
		portainer.PlatformReleaseStatusFailed:     true,
	},
	portainer.PlatformReleaseStatusSwitching: {
		portainer.PlatformReleaseStatusFinalChecking: true,
		portainer.PlatformReleaseStatusRecovering:    true,
	},
	portainer.PlatformReleaseStatusFinalChecking: {
		portainer.PlatformReleaseStatusSucceeded:  true,
		portainer.PlatformReleaseStatusRecovering: true,
	},
	portainer.PlatformReleaseStatusRecovering: {
		portainer.PlatformReleaseStatusFailed:         true,
		portainer.PlatformReleaseStatusRecoveryFailed: true,
	},
	portainer.PlatformReleaseStatusInterrupted: {
		portainer.PlatformReleaseStatusFailed:         true,
		portainer.PlatformReleaseStatusRecoveryFailed: true,
		portainer.PlatformReleaseStatusResolved:       true,
	},
	portainer.PlatformReleaseStatusRecoveryFailed: {
		portainer.PlatformReleaseStatusRecovering: true,
		portainer.PlatformReleaseStatusResolved:   true,
	},
}

func validateReleaseTransition(from portainer.PlatformReleaseStatus, to portainer.PlatformReleaseStatus) error {
	if platformReleaseTransitions[from][to] {
		return nil
	}

	return fmt.Errorf("release status transition %q -> %q is not allowed", from, to)
}

func releaseStatusIsTerminal(status portainer.PlatformReleaseStatus) bool {
	switch status {
	case portainer.PlatformReleaseStatusSucceeded,
		portainer.PlatformReleaseStatusFailed,
		portainer.PlatformReleaseStatusCanceled,
		portainer.PlatformReleaseStatusResolved:
		return true
	default:
		return false
	}
}

func releaseStatusKeepsLock(status portainer.PlatformReleaseStatus) bool {
	return status == portainer.PlatformReleaseStatusRecoveryFailed || status == portainer.PlatformReleaseStatusInterrupted
}

func releaseStatusReleasesLock(status portainer.PlatformReleaseStatus) bool {
	return releaseStatusIsTerminal(status) && !releaseStatusKeepsLock(status)
}
