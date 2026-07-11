package platform

import (
	"net/http"
	"strconv"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

const (
	runtimeReasonNotConfigured        = "RUNTIME_NOT_CONFIGURED"
	runtimeReasonInspectorUnavailable = "RUNTIME_INSPECTOR_UNAVAILABLE"
	runtimeReasonMissing              = "RUNTIME_MISSING"
)

type serviceDeploymentStatusResponse struct {
	ServiceDeploymentID      portainer.PlatformServiceDeploymentID   `json:"ServiceDeploymentId"`
	CurrentServingReleaseID  portainer.PlatformReleaseID             `json:"CurrentServingReleaseId"`
	CurrentArtifactID        portainer.PlatformArtifactID            `json:"CurrentArtifactId"`
	CurrentImage             string                                  `json:"CurrentImage,omitempty"`
	SpecRevision             int                                     `json:"SpecRevision"`
	LastDeployedSpecRevision int                                     `json:"LastDeployedSpecRevision"`
	LastDeployedAt           int64                                   `json:"LastDeployedAt"`
	DriftStatus              portainer.PlatformDeploymentDriftStatus `json:"DriftStatus"`
	RuntimeRef               portainer.RuntimeRef                    `json:"RuntimeRef"`
	RuntimeFound             bool                                    `json:"RuntimeFound"`
	RuntimeRunning           bool                                    `json:"RuntimeRunning"`
	RuntimeState             string                                  `json:"RuntimeState,omitempty"`
	RuntimeStatus            string                                  `json:"RuntimeStatus,omitempty"`
	RuntimeMessage           string                                  `json:"RuntimeMessage,omitempty"`
	RestartCount             int                                     `json:"RestartCount"`
	PublishedPorts           []portainer.PlatformPublishedPort       `json:"PublishedPorts,omitempty"`
	Reason                   string                                  `json:"Reason,omitempty"`
}

type serviceDeploymentLogsResponse struct {
	ServiceDeploymentID portainer.PlatformServiceDeploymentID `json:"ServiceDeploymentId"`
	RuntimeRef          portainer.RuntimeRef                  `json:"RuntimeRef"`
	Available           bool                                  `json:"Available"`
	Tail                int                                   `json:"Tail"`
	Logs                string                                `json:"Logs,omitempty"`
	Reason              string                                `json:"Reason,omitempty"`
}

func (handler *Handler) serviceDeploymentStatus(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "deploymentId")
	if handlerErr != nil {
		return handlerErr
	}

	deployment, err := handler.DataStore.PlatformServiceDeployment().Read(portainer.PlatformServiceDeploymentID(id))
	if err != nil {
		return handler.convertError(err)
	}

	result := serviceDeploymentStatusFromDeployment(*deployment)
	if deployment.CurrentRuntimeRef.ResourceID == "" {
		result.Reason = runtimeReasonNotConfigured
		return response.JSON(w, result)
	}
	if handler.RuntimeInspector == nil {
		result.Reason = runtimeReasonInspectorUnavailable
		return response.JSON(w, result)
	}

	inspection, err := handler.RuntimeInspector.InspectRuntime(r.Context(), deployment.CurrentRuntimeRef)
	if err != nil {
		return handler.convertError(err)
	}
	applyRuntimeInspectionToStatus(&result, inspection)
	if !inspection.Found {
		result.Reason = runtimeReasonMissing
		if err := handler.updateDeploymentRuntimeDrift(deployment.ID, portainer.PlatformDeploymentDriftRuntimeMissing); err != nil {
			return handler.convertError(err)
		}
		result.DriftStatus = portainer.PlatformDeploymentDriftRuntimeMissing
	}

	return response.JSON(w, result)
}

func (handler *Handler) serviceDeploymentLogs(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "deploymentId")
	if handlerErr != nil {
		return handlerErr
	}

	deployment, err := handler.DataStore.PlatformServiceDeployment().Read(portainer.PlatformServiceDeploymentID(id))
	if err != nil {
		return handler.convertError(err)
	}

	tail := parseRuntimeLogTail(r)
	result := serviceDeploymentLogsResponse{
		ServiceDeploymentID: deployment.ID,
		RuntimeRef:          deployment.CurrentRuntimeRef,
		Tail:                tail,
	}
	if deployment.CurrentRuntimeRef.ResourceID == "" {
		result.Reason = runtimeReasonNotConfigured
		return response.JSON(w, result)
	}
	if handler.RuntimeInspector == nil {
		result.Reason = runtimeReasonInspectorUnavailable
		return response.JSON(w, result)
	}

	logs, err := handler.RuntimeInspector.RuntimeLogs(r.Context(), deployment.CurrentRuntimeRef, platformservice.RuntimeLogOptions{Tail: tail})
	if err != nil {
		return handler.convertError(err)
	}
	result.Available = logs.Available
	result.Logs = logs.Logs
	result.Reason = logs.Reason
	result.Tail = logs.Tail
	if result.Reason == runtimeReasonMissing {
		if err := handler.updateDeploymentRuntimeDrift(deployment.ID, portainer.PlatformDeploymentDriftRuntimeMissing); err != nil {
			return handler.convertError(err)
		}
	}

	return response.JSON(w, result)
}

func serviceDeploymentStatusFromDeployment(deployment portainer.PlatformServiceDeployment) serviceDeploymentStatusResponse {
	return serviceDeploymentStatusResponse{
		ServiceDeploymentID:      deployment.ID,
		CurrentServingReleaseID:  deployment.CurrentServingReleaseID,
		CurrentArtifactID:        deployment.CurrentArtifactID,
		CurrentImage:             deployment.CurrentImage,
		SpecRevision:             deployment.SpecRevision,
		LastDeployedSpecRevision: deployment.LastDeployedSpecRevision,
		LastDeployedAt:           deployment.LastDeployedAt,
		DriftStatus:              deployment.DriftStatus,
		RuntimeRef:               deployment.CurrentRuntimeRef,
		PublishedPorts:           publishedPortsFromDesiredSpec(deployment.DesiredSpec.Ports),
	}
}

func applyRuntimeInspectionToStatus(result *serviceDeploymentStatusResponse, inspection platformservice.RuntimeInspection) {
	if result == nil {
		return
	}

	result.RuntimeRef = inspection.RuntimeRef
	result.RuntimeFound = inspection.Found
	result.RuntimeRunning = inspection.Running
	result.RuntimeState = inspection.State
	result.RuntimeStatus = inspection.Status
	result.RuntimeMessage = inspection.Message
	result.RestartCount = inspection.RestartCount
	if len(inspection.PublishedPorts) > 0 {
		result.PublishedPorts = inspection.PublishedPorts
	}
}

func (handler *Handler) updateDeploymentRuntimeDrift(deploymentID portainer.PlatformServiceDeploymentID, drift portainer.PlatformDeploymentDriftStatus) error {
	return handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		deployment, err := tx.PlatformServiceDeployment().Read(deploymentID)
		if err != nil {
			return err
		}
		if deployment.DriftStatus == drift {
			return nil
		}

		deployment.DriftStatus = drift
		deployment.UpdatedAt = time.Now().Unix()
		deployment.ResourceVersion++

		return tx.PlatformServiceDeployment().Update(deployment.ID, deployment)
	})
}

func parseRuntimeLogTail(r *http.Request) int {
	tail := 100
	if value := r.URL.Query().Get("tail"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			tail = parsed
		}
	}
	if tail <= 0 {
		return 100
	}
	if tail > 1000 {
		return 1000
	}

	return tail
}

func publishedPortsFromDesiredSpec(specs []portainer.PlatformPortSpec) []portainer.PlatformPublishedPort {
	ports := make([]portainer.PlatformPublishedPort, 0, len(specs))
	for _, spec := range specs {
		if spec.HostPort <= 0 {
			continue
		}
		ports = append(ports, portainer.PlatformPublishedPort{
			Name:          spec.Name,
			ContainerPort: spec.ContainerPort,
			HostPort:      spec.HostPort,
			Protocol:      spec.Protocol,
		})
	}

	return ports
}
