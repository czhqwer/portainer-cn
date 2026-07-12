package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type buildJavaArtifactPayload struct {
	EndpointID portainer.EndpointID `json:"EndpointId"`
	JVMArgs    []string             `json:"JvmArgs,omitempty"`
	AppArgs    []string             `json:"AppArgs,omitempty"`
	Port       int                  `json:"Port"`
}

func (p *buildJavaArtifactPayload) Validate(_ *http.Request) error {
	return platformservice.ValidateJavaBuildOptions(platformservice.JavaBuildOptions{JVMArgs: p.JVMArgs, AppArgs: p.AppArgs, Port: p.Port})
}

// artifactJavaBuild 使用固定 Java 8 context，用户仅能传递已校验的 token；Docker build 在事务外运行并受 Artifact 租约保护。
func (h *Handler) artifactJavaBuild(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, e := h.routeID(r, "artifactId")
	if e != nil {
		return e
	}
	a, e := h.requireArtifactPermission(r, portainer.PlatformArtifactID(id), platformPermissionArtifactRelease)
	if e != nil {
		return e
	}
	var p buildJavaArtifactPayload
	if err := request.DecodeAndValidateJSONPayload(r, &p); err != nil {
		return validationFailed(err)
	}
	if e := h.requireEndpointAccess(r, p.EndpointID); e != nil {
		return e
	}
	if h.JavaImageBuilder == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Java build is unavailable", "JAVA_BUILD_UNAVAILABLE", nil)
	}
	lease := uuid.NewString()
	now := time.Now().Unix()
	var artifact *portainer.PlatformArtifact
	err := h.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		artifact, err = readActiveArtifact(tx, a.ID)
		if err != nil {
			return err
		}
		if artifact.Type != portainer.PlatformArtifactTypeJavaJar || artifact.StorageProvider != portainer.PlatformStorageProviderLocal {
			return validationFailedError("Artifact cannot use Java template")
		}
		if artifactTaskActive(*artifact, now) {
			return conflictError("Artifact task is already running")
		}
		artifact.Status = portainer.PlatformArtifactStatusBuilding
		artifact.TaskID = lease
		artifact.TaskLeaseExpiresAt = now + 600
		artifact.BuildTemplate = platformservice.Java8BuildTemplate
		artifact.FailureReason = ""
		touchLifecycle(&artifact.PlatformLifecycle, now)
		return tx.PlatformArtifact().Update(artifact.ID, artifact)
	})
	if err != nil {
		return h.convertError(err)
	}
	path, err := h.artifactLocalFilePath(*artifact)
	if err != nil {
		return h.finishJavaBuild(r, *artifact, lease, p.EndpointID, "CONTROLLED_BUILD_FAILED")
	}
	file, err := os.Open(path)
	if err != nil {
		return h.finishJavaBuild(r, *artifact, lease, p.EndpointID, "CONTROLLED_BUILD_FAILED")
	}
	if err := platformservice.ValidateJavaJar(path); err != nil {
		_ = file.Close()
		return h.finishJavaBuild(r, *artifact, lease, p.EndpointID, "ARTIFACT_TYPE_INVALID")
	}
	contextTar, err := platformservice.NewJava8BuildContext(file, platformservice.JavaBuildOptions{JVMArgs: p.JVMArgs, AppArgs: p.AppArgs, Port: p.Port})
	_ = file.Close()
	if err != nil {
		return h.finishJavaBuild(r, *artifact, lease, p.EndpointID, "CONTROLLED_BUILD_FAILED")
	}
	candidate := fmt.Sprintf("portainer-platform-java/%d:%s", artifact.ID, lease[:12])
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	result, err := h.JavaImageBuilder.Build(ctx, platformservice.JavaImageBuildRequest{EndpointID: int(p.EndpointID), Context: bytes.NewReader(contextTar), CandidateRef: candidate})
	if err != nil {
		_ = h.JavaImageBuilder.Cleanup(context.Background(), int(p.EndpointID), candidate)
		return h.finishJavaBuild(r, *artifact, lease, p.EndpointID, "CONTROLLED_BUILD_FAILED")
	}
	err = h.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformArtifact().Read(artifact.ID)
		if err != nil || current.TaskID != lease {
			return errors.New("java build lease lost")
		}
		current.Status = portainer.PlatformArtifactStatusBuilt
		current.CandidateImageRef = result.CandidateRef
		current.CandidateImageID = result.ImageID
		current.TaskLeaseExpiresAt = 0
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		return tx.PlatformArtifact().Update(current.ID, current)
	})
	if err != nil {
		_ = h.JavaImageBuilder.Cleanup(context.Background(), int(p.EndpointID), result.CandidateRef)
		return h.finishJavaBuild(r, *artifact, lease, p.EndpointID, "CONTROLLED_BUILD_FAILED")
	}
	updated, _ := h.DataStore.PlatformArtifact().Read(artifact.ID)
	return response.JSON(w, artifactResponse(*updated))
}
func (h *Handler) finishJavaBuild(r *http.Request, a portainer.PlatformArtifact, lease string, endpoint portainer.EndpointID, reason string) *httperror.HandlerError {
	_ = h.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformArtifact().Read(a.ID)
		if err != nil || current.TaskID != lease {
			return err
		}
		current.Status = portainer.PlatformArtifactStatusFailed
		current.TaskLeaseExpiresAt = 0
		current.FailureReason = reason
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		return tx.PlatformArtifact().Update(current.ID, current)
	})
	return validationFailedError("Java build failed")
}
