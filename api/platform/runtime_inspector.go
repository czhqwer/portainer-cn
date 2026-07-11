package platform

import (
	"context"

	portainer "github.com/portainer/portainer/api"
)

type RuntimeInspector interface {
	InspectRuntime(ctx context.Context, runtimeRef portainer.RuntimeRef) (RuntimeInspection, error)
	RuntimeLogs(ctx context.Context, runtimeRef portainer.RuntimeRef, options RuntimeLogOptions) (RuntimeLogResult, error)
}

type RuntimeInspection struct {
	RuntimeRef     portainer.RuntimeRef
	Found          bool
	Running        bool
	State          string
	Status         string
	Image          string
	RestartCount   int
	StartedAt      string
	FinishedAt     string
	PublishedPorts []portainer.PlatformPublishedPort
	Message        string
}

type RuntimeLogOptions struct {
	Tail int
}

type RuntimeLogResult struct {
	RuntimeRef portainer.RuntimeRef
	Available  bool
	Tail       int
	Logs       string
	Reason     string
}
