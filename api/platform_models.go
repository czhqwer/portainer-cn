package portainer

import (
	"fmt"
	"regexp"
	"strings"
)

type (
	PlatformProjectID           int
	PlatformEnvironmentID       int
	PlatformApplicationID       int
	PlatformServiceDefinitionID int
	PlatformServiceDeploymentID int
	PlatformConfigSetID         int
	PlatformArtifactID          int
	PlatformReleaseID           int
	PlatformAuditLogID          int

	PlatformProjectRole           string
	PlatformLifecycleStatus       string
	PlatformEnvironmentType       string
	PlatformTargetMode            string
	PlatformDeploymentTargetRole  string
	PlatformReleasePolicyType     string
	PlatformServiceType           string
	PlatformArtifactType          string
	PlatformArtifactSourceType    string
	PlatformTraceability          string
	PlatformStorageProvider       string
	PlatformImagePullPolicy       string
	PlatformPortProtocol          string
	PlatformPortExposeMode        string
	PlatformEnvVarSource          string
	PlatformConfigScopeType       string
	PlatformConfigValueType       string
	PlatformConfigEntrySource     string
	PlatformHealthVerification    string
	PlatformHealthCheckType       string
	PlatformHealthCheckStatus     string
	PlatformRuntimeDriver         string
	PlatformRuntimeResourceType   string
	PlatformRuntimeRestartPolicy  string
	PlatformVolumeType            string
	PlatformReleaseStrategyType   string
	PlatformReleaseTriggerType    string
	PlatformReleaseStatus         string
	PlatformReleaseStepStatus     string
	PlatformReleaseResolution     string
	PlatformReleaseTargetStatus   string
	PlatformExecutorMode          string
	PlatformDeploymentDriftStatus string
	PlatformAuditAction           string
	PlatformAuditResult           string
)

const (
	PlatformProjectRoleAdmin     PlatformProjectRole = "admin"
	PlatformProjectRoleDeveloper PlatformProjectRole = "developer"
	PlatformProjectRoleViewer    PlatformProjectRole = "viewer"

	PlatformLifecycleStatusActive   PlatformLifecycleStatus = "active"
	PlatformLifecycleStatusArchived PlatformLifecycleStatus = "archived"

	PlatformEnvironmentTypeDev    PlatformEnvironmentType = "dev"
	PlatformEnvironmentTypeTest   PlatformEnvironmentType = "test"
	PlatformEnvironmentTypeProd   PlatformEnvironmentType = "prod"
	PlatformEnvironmentTypeCustom PlatformEnvironmentType = "custom"

	PlatformTargetModeSingle PlatformTargetMode = "single"
	PlatformTargetModeMulti  PlatformTargetMode = "multi"

	PlatformDeploymentTargetRoleWorkload PlatformDeploymentTargetRole = "workload"
	PlatformDeploymentTargetRoleGateway  PlatformDeploymentTargetRole = "gateway"

	PlatformReleasePolicyReplace PlatformReleasePolicyType = "replace"

	PlatformServiceTypeFrontend      PlatformServiceType = "frontend"
	PlatformServiceTypeStaticSite    PlatformServiceType = "static-site"
	PlatformServiceTypeBackend       PlatformServiceType = "backend"
	PlatformServiceTypePythonService PlatformServiceType = "python-service"
	PlatformServiceTypeJavaService   PlatformServiceType = "java-service"
	PlatformServiceTypeWorker        PlatformServiceType = "worker"
	PlatformServiceTypeScheduler     PlatformServiceType = "scheduler"
	PlatformServiceTypePythonJob     PlatformServiceType = "python-job"
	PlatformServiceTypeDatabase      PlatformServiceType = "database"
	PlatformServiceTypeRedis         PlatformServiceType = "redis"

	PlatformArtifactTypeImage         PlatformArtifactType = "image"
	PlatformArtifactTypeDockerTar     PlatformArtifactType = "docker-image-tar"
	PlatformArtifactTypeOCIArchive    PlatformArtifactType = "oci-archive"
	PlatformArtifactTypeJavaJar       PlatformArtifactType = "java-jar"
	PlatformArtifactTypeFrontendDist  PlatformArtifactType = "frontend-dist"
	PlatformArtifactTypeStaticPackage PlatformArtifactType = "static-package"

	PlatformArtifactSourceImageReference PlatformArtifactSourceType = "image-reference"
	PlatformArtifactSourceUpload         PlatformArtifactSourceType = "upload"
	PlatformArtifactSourceObjectStorage  PlatformArtifactSourceType = "object-storage"

	PlatformTraceabilityStrong PlatformTraceability = "strong"
	PlatformTraceabilityWeak   PlatformTraceability = "weak"

	PlatformStorageProviderLocal     PlatformStorageProvider = "local"
	PlatformStorageProviderMinio     PlatformStorageProvider = "minio"
	PlatformStorageProviderS3        PlatformStorageProvider = "s3"
	PlatformStorageProviderAliyunOSS PlatformStorageProvider = "aliyun-oss"

	PlatformImagePullPolicyAlways       PlatformImagePullPolicy = "always"
	PlatformImagePullPolicyIfNotPresent PlatformImagePullPolicy = "if-not-present"

	PlatformPortProtocolTCP PlatformPortProtocol = "tcp"
	PlatformPortProtocolUDP PlatformPortProtocol = "udp"

	PlatformPortExposeModePublished PlatformPortExposeMode = "published"

	PlatformEnvVarSourceLiteral  PlatformEnvVarSource = "literal"
	PlatformEnvVarSourceConfig   PlatformEnvVarSource = "config"
	PlatformEnvVarSourceSecret   PlatformEnvVarSource = "secret"
	PlatformEnvVarSourceDatabase PlatformEnvVarSource = "database"

	PlatformConfigScopeProject           PlatformConfigScopeType = "project"
	PlatformConfigScopeEnvironment       PlatformConfigScopeType = "environment"
	PlatformConfigScopeServiceDeployment PlatformConfigScopeType = "service-deployment"

	PlatformConfigValuePlain       PlatformConfigValueType = "plain"
	PlatformConfigValueSecretRef   PlatformConfigValueType = "secret-ref"
	PlatformConfigValueDatabaseRef PlatformConfigValueType = "database-ref"
	PlatformConfigValueRedisRef    PlatformConfigValueType = "redis-ref"

	PlatformConfigEntrySourceProject           PlatformConfigEntrySource = "project"
	PlatformConfigEntrySourceEnvironment       PlatformConfigEntrySource = "environment"
	PlatformConfigEntrySourceServiceDeployment PlatformConfigEntrySource = "service-deployment"

	PlatformConfigSetDefaultName = "default"

	PlatformHealthVerificationVerified    PlatformHealthVerification = "verified"
	PlatformHealthVerificationStartupOnly PlatformHealthVerification = "startup-only"
	PlatformHealthVerificationUnverified  PlatformHealthVerification = "unverified"

	PlatformHealthCheckTypeHTTP    PlatformHealthCheckType = "http"
	PlatformHealthCheckTypeTCP     PlatformHealthCheckType = "tcp"
	PlatformHealthCheckTypeStartup PlatformHealthCheckType = "startup"
	PlatformHealthCheckTypeNone    PlatformHealthCheckType = "none"

	PlatformHealthCheckStatusPassed  PlatformHealthCheckStatus = "passed"
	PlatformHealthCheckStatusFailed  PlatformHealthCheckStatus = "failed"
	PlatformHealthCheckStatusSkipped PlatformHealthCheckStatus = "skipped"

	PlatformRuntimeDriverDockerContainer PlatformRuntimeDriver = "docker-container"
	PlatformRuntimeDriverDockerStack     PlatformRuntimeDriver = "docker-stack"
	PlatformRuntimeDriverKubernetes      PlatformRuntimeDriver = "kubernetes"

	PlatformRuntimeResourceContainer  PlatformRuntimeResourceType = "container"
	PlatformRuntimeResourceStack      PlatformRuntimeResourceType = "stack"
	PlatformRuntimeResourceKubernetes PlatformRuntimeResourceType = "kubernetes-workload"

	PlatformRuntimeRestartPolicyNo            PlatformRuntimeRestartPolicy = "no"
	PlatformRuntimeRestartPolicyAlways        PlatformRuntimeRestartPolicy = "always"
	PlatformRuntimeRestartPolicyUnlessStopped PlatformRuntimeRestartPolicy = "unless-stopped"
	PlatformRuntimeRestartPolicyOnFailure     PlatformRuntimeRestartPolicy = "on-failure"

	PlatformVolumeTypeBind   PlatformVolumeType = "bind"
	PlatformVolumeTypeVolume PlatformVolumeType = "volume"

	PlatformReleaseStrategyReplace PlatformReleaseStrategyType = "replace"

	PlatformReleaseTriggerDeploy   PlatformReleaseTriggerType = "deploy"
	PlatformReleaseTriggerRedeploy PlatformReleaseTriggerType = "redeploy"
	PlatformReleaseTriggerRollback PlatformReleaseTriggerType = "rollback"

	PlatformReleaseStatusQueued            PlatformReleaseStatus = "queued"
	PlatformReleaseStatusValidating        PlatformReleaseStatus = "validating"
	PlatformReleaseStatusPulling           PlatformReleaseStatus = "pulling"
	PlatformReleaseStatusPreparing         PlatformReleaseStatus = "preparing"
	PlatformReleaseStatusCandidateStarting PlatformReleaseStatus = "candidate-starting"
	PlatformReleaseStatusCandidateChecking PlatformReleaseStatus = "candidate-checking"
	PlatformReleaseStatusSwitching         PlatformReleaseStatus = "switching"
	PlatformReleaseStatusFinalChecking     PlatformReleaseStatus = "final-checking"
	PlatformReleaseStatusRecovering        PlatformReleaseStatus = "recovering"
	PlatformReleaseStatusInterrupted       PlatformReleaseStatus = "interrupted"
	PlatformReleaseStatusSucceeded         PlatformReleaseStatus = "succeeded"
	PlatformReleaseStatusFailed            PlatformReleaseStatus = "failed"
	PlatformReleaseStatusRecoveryFailed    PlatformReleaseStatus = "recovery-failed"
	PlatformReleaseStatusCanceled          PlatformReleaseStatus = "canceled"
	PlatformReleaseStatusResolved          PlatformReleaseStatus = "resolved"

	PlatformReleaseStepStatusPending   PlatformReleaseStepStatus = "pending"
	PlatformReleaseStepStatusRunning   PlatformReleaseStepStatus = "running"
	PlatformReleaseStepStatusSucceeded PlatformReleaseStepStatus = "succeeded"
	PlatformReleaseStepStatusFailed    PlatformReleaseStepStatus = "failed"
	PlatformReleaseStepStatusSkipped   PlatformReleaseStepStatus = "skipped"

	PlatformReleaseResolutionNone            PlatformReleaseResolution = "none"
	PlatformReleaseResolutionAcceptCurrent   PlatformReleaseResolution = "accept-current"
	PlatformReleaseResolutionMarkHandled     PlatformReleaseResolution = "mark-handled"
	PlatformReleaseResolutionReleaseLockOnly PlatformReleaseResolution = "release-lock-only"

	PlatformReleaseTargetStatusPending   PlatformReleaseTargetStatus = "pending"
	PlatformReleaseTargetStatusRunning   PlatformReleaseTargetStatus = "running"
	PlatformReleaseTargetStatusSucceeded PlatformReleaseTargetStatus = "succeeded"
	PlatformReleaseTargetStatusFailed    PlatformReleaseTargetStatus = "failed"
	PlatformReleaseTargetStatusSkipped   PlatformReleaseTargetStatus = "skipped"

	PlatformExecutorModeSingle PlatformExecutorMode = "single"
	PlatformExecutorModeMulti  PlatformExecutorMode = "multi"
	PlatformExecutorModeBatch  PlatformExecutorMode = "batch"

	PlatformDeploymentDriftNone           PlatformDeploymentDriftStatus = "none"
	PlatformDeploymentDriftConfigChanged  PlatformDeploymentDriftStatus = "config-changed"
	PlatformDeploymentDriftRuntimeMissing PlatformDeploymentDriftStatus = "runtime-missing"
	PlatformDeploymentDriftRuntimeDrifted PlatformDeploymentDriftStatus = "runtime-drifted"

	PlatformAuditActionReleaseCreated        PlatformAuditAction = "release.created"
	PlatformAuditActionReleaseSucceeded      PlatformAuditAction = "release.succeeded"
	PlatformAuditActionReleaseFailed         PlatformAuditAction = "release.failed"
	PlatformAuditActionReleaseRecoveryFailed PlatformAuditAction = "release.recovery_failed"
	PlatformAuditActionReleaseCanceled       PlatformAuditAction = "release.canceled"
	PlatformAuditActionReleaseResolved       PlatformAuditAction = "release.resolved"
	PlatformAuditActionReleaseRetryRecovery  PlatformAuditAction = "release.retry_recovery"
	PlatformAuditActionReleaseCleanupRuntime PlatformAuditAction = "release.cleanup_runtime"
	PlatformAuditActionReleaseDenied         PlatformAuditAction = "release.denied"
	PlatformAuditActionSecretRevealed        PlatformAuditAction = "secret.revealed"
	PlatformAuditActionSecretCopied          PlatformAuditAction = "secret.copied"

	PlatformAuditResultSuccess PlatformAuditResult = "success"
	PlatformAuditResultFailed  PlatformAuditResult = "failed"
	PlatformAuditResultDenied  PlatformAuditResult = "denied"
)

// PlatformLifecycle keeps platform control-plane lifecycle separate from Docker runtime state;
// this lets Gate 0A safely persist and archive metadata before any real Docker executor exists.
type PlatformLifecycle struct {
	LifecycleStatus  PlatformLifecycleStatus `json:"LifecycleStatus" example:"active"`
	ArchivedAt       int64                   `json:"ArchivedAt" example:"0"`
	ArchivedByUserID UserID                  `json:"ArchivedByUserId" example:"0"`
	CreatedAt        int64                   `json:"CreatedAt" example:"1783740000"`
	UpdatedAt        int64                   `json:"UpdatedAt" example:"1783740000"`
	ResourceVersion  int                     `json:"ResourceVersion" example:"1"`
}

type PlatformProject struct {
	ID             PlatformProjectID              `json:"Id" example:"1"`
	Name           string                         `json:"Name" example:"Default project"`
	Slug           string                         `json:"Slug" example:"default-project"`
	Description    string                         `json:"Description,omitempty"`
	MemberPolicies map[UserID]PlatformProjectRole `json:"MemberPolicies,omitempty"`
	TeamPolicies   map[TeamID]PlatformProjectRole `json:"TeamPolicies,omitempty"`
	PlatformLifecycle
}

type PlatformEnvironment struct {
	ID                PlatformEnvironmentID      `json:"Id" example:"1"`
	ProjectID         PlatformProjectID          `json:"ProjectId" example:"1"`
	Name              string                     `json:"Name" example:"Production"`
	Slug              string                     `json:"Slug" example:"prod"`
	Type              PlatformEnvironmentType    `json:"Type" example:"prod"`
	IsProduction      bool                       `json:"IsProduction" example:"true"`
	TargetMode        PlatformTargetMode         `json:"TargetMode" example:"single"`
	Targets           []PlatformDeploymentTarget `json:"Targets,omitempty"`
	DefaultRegistryID RegistryID                 `json:"DefaultRegistryId" example:"1"`
	HealthCheckHost   string                     `json:"HealthCheckHost,omitempty" example:"127.0.0.1"`
	ReleasePolicy     PlatformReleasePolicy      `json:"ReleasePolicy"`
	PlatformLifecycle
}

type PlatformDeploymentTarget struct {
	EndpointID  EndpointID                   `json:"EndpointId" example:"1"`
	NodeName    string                       `json:"NodeName,omitempty"`
	Role        PlatformDeploymentTargetRole `json:"Role" example:"workload"`
	HostAddress string                       `json:"HostAddress,omitempty" example:"127.0.0.1"`
	Enabled     bool                         `json:"Enabled" example:"true"`
}

type PlatformReleasePolicy struct {
	Type PlatformReleasePolicyType `json:"Type" example:"replace"`
}

type PlatformApplication struct {
	ID           PlatformApplicationID `json:"Id" example:"1"`
	ProjectID    PlatformProjectID     `json:"ProjectId" example:"1"`
	Name         string                `json:"Name" example:"App"`
	Slug         string                `json:"Slug" example:"app"`
	Description  string                `json:"Description,omitempty"`
	OwnerUserIDs []UserID              `json:"OwnerUserIds,omitempty"`
	PlatformLifecycle
}

type PlatformServiceDefinition struct {
	ID            PlatformServiceDefinitionID `json:"Id" example:"1"`
	ProjectID     PlatformProjectID           `json:"ProjectId" example:"1"`
	ApplicationID PlatformApplicationID       `json:"ApplicationId" example:"1"`
	Name          string                      `json:"Name" example:"Order API"`
	Slug          string                      `json:"Slug" example:"order-api"`
	Type          PlatformServiceType         `json:"Type" example:"backend"`
	Description   string                      `json:"Description,omitempty"`
	PlatformLifecycle
}

type PlatformServiceDeployment struct {
	ID                       PlatformServiceDeploymentID   `json:"Id" example:"1"`
	ProjectID                PlatformProjectID             `json:"ProjectId" example:"1"`
	EnvironmentID            PlatformEnvironmentID         `json:"EnvironmentId" example:"1"`
	ApplicationID            PlatformApplicationID         `json:"ApplicationId" example:"1"`
	ServiceDefinitionID      PlatformServiceDefinitionID   `json:"ServiceDefinitionId" example:"1"`
	DesiredSpec              PlatformDeploymentDesiredSpec `json:"DesiredSpec"`
	SpecRevision             int                           `json:"SpecRevision" example:"1"`
	LastDeployedSpecRevision int                           `json:"LastDeployedSpecRevision" example:"0"`
	LastDeployedAt           int64                         `json:"LastDeployedAt" example:"0"`
	CurrentServingReleaseID  PlatformReleaseID             `json:"CurrentServingReleaseId" example:"0"`
	CurrentArtifactID        PlatformArtifactID            `json:"CurrentArtifactId" example:"0"`
	CurrentRuntimeRef        RuntimeRef                    `json:"CurrentRuntimeRef"`
	CurrentImage             string                        `json:"CurrentImage,omitempty"`
	DriftStatus              PlatformDeploymentDriftStatus `json:"DriftStatus" example:"none"`
	LastObservedAt           int64                         `json:"LastObservedAt" example:"0"`
	PlatformLifecycle
}

type PlatformDeploymentDesiredSpec struct {
	Image        PlatformImageSpec       `json:"Image"`
	Ports        []PlatformPortSpec      `json:"Ports,omitempty"`
	EnvOverrides []PlatformEnvVar        `json:"EnvOverrides,omitempty"`
	SecretRefs   []PlatformSecretRef     `json:"SecretRefs,omitempty"`
	ConfigRefs   []PlatformConfigRef     `json:"ConfigRefs,omitempty"`
	Volumes      []PlatformVolumeSpec    `json:"Volumes,omitempty"`
	HealthCheck  PlatformHealthCheckSpec `json:"HealthCheck"`
	Runtime      PlatformRuntimeSpec     `json:"Runtime"`
	Strategy     PlatformReleaseStrategy `json:"Strategy"`
}

type PlatformImageSpec struct {
	Image          string                  `json:"Image" example:"registry.example.com/app/order-api:1.0.0"`
	RegistryID     RegistryID              `json:"RegistryId" example:"1"`
	PullPolicy     PlatformImagePullPolicy `json:"PullPolicy" example:"if-not-present"`
	ResolvedDigest string                  `json:"ResolvedDigest,omitempty"`
	Traceability   PlatformTraceability    `json:"Traceability" example:"weak"`
}

type PlatformPortSpec struct {
	Name          string                 `json:"Name,omitempty" example:"http"`
	ContainerPort int                    `json:"ContainerPort" example:"8080"`
	HostPort      int                    `json:"HostPort,omitempty" example:"18080"`
	Protocol      PlatformPortProtocol   `json:"Protocol" example:"tcp"`
	ExposeMode    PlatformPortExposeMode `json:"ExposeMode" example:"published"`
}

type PlatformEnvVar struct {
	Name     string               `json:"Name" example:"APP_ENV"`
	Value    string               `json:"Value,omitempty"`
	Source   PlatformEnvVarSource `json:"Source" example:"literal"`
	IsSecret bool                 `json:"IsSecret" example:"false"`
	HasValue bool                 `json:"HasValue,omitempty"`
	Hash     string               `json:"Hash,omitempty"`
}

type PlatformSecretRef struct {
	Name    string `json:"Name" example:"DATABASE_PASSWORD"`
	RefType string `json:"RefType,omitempty" example:"secret"`
	RefID   string `json:"RefId,omitempty" example:"secret-1"`
}

type PlatformConfigRef struct {
	ConfigSetID PlatformConfigSetID `json:"ConfigSetId" example:"1"`
	Key         string              `json:"Key,omitempty" example:"APP_ENV"`
	Required    bool                `json:"Required" example:"false"`
}

type PlatformVolumeSpec struct {
	Type     PlatformVolumeType `json:"Type" example:"bind"`
	Source   string             `json:"Source" example:"/data/app"`
	Target   string             `json:"Target" example:"/app/data"`
	ReadOnly bool               `json:"ReadOnly" example:"false"`
}

type PlatformHealthCheckSpec struct {
	VerificationLevel  PlatformHealthVerification `json:"VerificationLevel" example:"verified"`
	Type               PlatformHealthCheckType    `json:"Type" example:"http"`
	Path               string                     `json:"Path,omitempty" example:"/health"`
	Port               int                        `json:"Port" example:"8080"`
	IntervalSeconds    int                        `json:"IntervalSeconds" example:"10"`
	TimeoutSeconds     int                        `json:"TimeoutSeconds" example:"3"`
	Retries            int                        `json:"Retries" example:"3"`
	StartPeriodSeconds int                        `json:"StartPeriodSeconds" example:"30"`
}

type PlatformRuntimeSpec struct {
	RuntimeDriver           PlatformRuntimeDriver        `json:"RuntimeDriver" example:"docker-container"`
	Replicas                int                          `json:"Replicas" example:"1"`
	RestartPolicy           PlatformRuntimeRestartPolicy `json:"RestartPolicy" example:"unless-stopped"`
	StopTimeoutSeconds      int                          `json:"StopTimeoutSeconds" example:"10"`
	ContainerRetentionCount int                          `json:"ContainerRetentionCount" example:"1"`
	ContainerRetentionDays  int                          `json:"ContainerRetentionDays" example:"7"`
}

type PlatformConfigSet struct {
	ID        PlatformConfigSetID     `json:"Id" example:"1"`
	ProjectID PlatformProjectID       `json:"ProjectId" example:"1"`
	ScopeType PlatformConfigScopeType `json:"ScopeType" example:"service-deployment"`
	ScopeID   int                     `json:"ScopeId" example:"1"`
	Name      string                  `json:"Name" example:"default"`
	Entries   []PlatformConfigEntry   `json:"Entries,omitempty"`
	Revision  int                     `json:"Revision" example:"1"`
	PlatformLifecycle
}

type PlatformConfigEntry struct {
	Key               string                    `json:"Key" example:"APP_ENV"`
	ValueType         PlatformConfigValueType   `json:"ValueType" example:"plain"`
	Value             string                    `json:"Value,omitempty"`
	CipherText        string                    `json:"CipherText,omitempty" swaggerignore:"true"`
	EncryptionVersion string                    `json:"EncryptionVersion,omitempty"`
	Hash              string                    `json:"Hash,omitempty"`
	HasValue          bool                      `json:"HasValue,omitempty"`
	Sensitive         bool                      `json:"Sensitive" example:"false"`
	Required          bool                      `json:"Required" example:"false"`
	Source            PlatformConfigEntrySource `json:"Source" example:"service-deployment"`
}

type PlatformEffectiveConfigSnapshot struct {
	SpecRevision       int                           `json:"SpecRevision" example:"1"`
	ConfigSetRevisions map[string]int                `json:"ConfigSetRevisions,omitempty"`
	Entries            []PlatformConfigEntrySnapshot `json:"Entries,omitempty"`
	Hash               string                        `json:"Hash,omitempty"`
}

type PlatformConfigEntrySnapshot struct {
	Key       string                    `json:"Key" example:"APP_ENV"`
	ValueType PlatformConfigValueType   `json:"ValueType" example:"plain"`
	Value     string                    `json:"Value,omitempty"`
	Sensitive bool                      `json:"Sensitive" example:"false"`
	Required  bool                      `json:"Required" example:"false"`
	Source    PlatformConfigEntrySource `json:"Source" example:"service-deployment"`
	Hash      string                    `json:"Hash,omitempty"`
	HasValue  bool                      `json:"HasValue,omitempty"`
}

type PlatformSecretSnapshot struct {
	Name              string `json:"Name" example:"DATABASE_PASSWORD"`
	CipherText        string `json:"CipherText,omitempty" swaggerignore:"true"`
	EncryptionVersion string `json:"EncryptionVersion,omitempty"`
	Hash              string `json:"Hash,omitempty"`
	HasValue          bool   `json:"HasValue,omitempty"`
}

type PlatformArtifact struct {
	ID                  PlatformArtifactID          `json:"Id" example:"1"`
	ProjectID           PlatformProjectID           `json:"ProjectId" example:"1"`
	ApplicationID       PlatformApplicationID       `json:"ApplicationId,omitempty" example:"1"`
	ServiceDefinitionID PlatformServiceDefinitionID `json:"ServiceDefinitionId,omitempty" example:"1"`
	Name                string                      `json:"Name" example:"order-api"`
	Version             string                      `json:"Version" example:"20260711-001"`
	Type                PlatformArtifactType        `json:"Type" example:"image"`
	SourceType          PlatformArtifactSourceType  `json:"SourceType" example:"image-reference"`
	ImageRef            string                      `json:"ImageRef,omitempty"`
	ImageDigest         string                      `json:"ImageDigest,omitempty"`
	Traceability        PlatformTraceability        `json:"Traceability" example:"weak"`
	RegistryID          RegistryID                  `json:"RegistryId,omitempty" example:"1"`
	FileName            string                      `json:"FileName,omitempty"`
	Size                int64                       `json:"Size,omitempty"`
	SHA256              string                      `json:"SHA256,omitempty"`
	StorageProvider     PlatformStorageProvider     `json:"StorageProvider,omitempty"`
	StoragePath         string                      `json:"StoragePath,omitempty"`
	Retained            bool                        `json:"Retained" example:"false"`
	Cleanable           bool                        `json:"Cleanable" example:"false"`
	PlatformLifecycle
}

type PlatformRelease struct {
	ID                      PlatformReleaseID             `json:"Id" example:"1"`
	ProjectID               PlatformProjectID             `json:"ProjectId" example:"1"`
	EnvironmentID           PlatformEnvironmentID         `json:"EnvironmentId" example:"1"`
	ApplicationID           PlatformApplicationID         `json:"ApplicationId" example:"1"`
	ServiceDefinitionID     PlatformServiceDefinitionID   `json:"ServiceDefinitionId" example:"1"`
	ServiceDeploymentID     PlatformServiceDeploymentID   `json:"ServiceDeploymentId" example:"1"`
	ArtifactID              PlatformArtifactID            `json:"ArtifactId" example:"1"`
	Version                 string                        `json:"Version" example:"20260711-001"`
	TriggerType             PlatformReleaseTriggerType    `json:"TriggerType" example:"deploy"`
	Strategy                PlatformReleaseStrategy       `json:"Strategy"`
	Status                  PlatformReleaseStatus         `json:"Status" example:"queued"`
	OperatorUserID          UserID                        `json:"OperatorUserId" example:"1"`
	IdempotencyKeyHash      string                        `json:"IdempotencyKeyHash,omitempty"`
	PayloadHash             string                        `json:"PayloadHash,omitempty"`
	ExpectedSpecRevision    int                           `json:"ExpectedSpecRevision" example:"1"`
	Image                   string                        `json:"Image,omitempty"`
	ImageDigest             string                        `json:"ImageDigest,omitempty"`
	Traceability            PlatformTraceability          `json:"Traceability" example:"weak"`
	ArtifactSnapshot        PlatformArtifactSnapshot      `json:"ArtifactSnapshot"`
	ConfigSnapshot          PlatformServiceConfigSnapshot `json:"ConfigSnapshot"`
	TargetSnapshot          PlatformTargetSnapshot        `json:"TargetSnapshot"`
	RuntimeSnapshot         PlatformRuntimeSnapshot       `json:"RuntimeSnapshot"`
	GatewaySnapshot         PlatformGatewaySnapshot       `json:"GatewaySnapshot"`
	HealthCheckResult       PlatformHealthCheckResult     `json:"HealthCheckResult"`
	Steps                   []PlatformReleaseStep         `json:"Steps,omitempty"`
	TargetResults           []PlatformReleaseTargetResult `json:"TargetResults,omitempty"`
	PreviousReleaseID       PlatformReleaseID             `json:"PreviousReleaseId" example:"0"`
	RollbackSourceReleaseID PlatformReleaseID             `json:"RollbackSourceReleaseId" example:"0"`
	CanRollback             bool                          `json:"CanRollback" example:"false"`
	FailureReason           string                        `json:"FailureReason,omitempty"`
	ManualActionRequired    bool                          `json:"ManualActionRequired" example:"false"`
	ResolutionAction        PlatformReleaseResolution     `json:"ResolutionAction" example:"none"`
	ResolvedByUserID        UserID                        `json:"ResolvedByUserId" example:"0"`
	ResolvedAt              int64                         `json:"ResolvedAt" example:"0"`
	ResolutionComment       string                        `json:"ResolutionComment,omitempty"`
	LeaseOwner              string                        `json:"LeaseOwner,omitempty"`
	LeaseExpiresAt          int64                         `json:"LeaseExpiresAt" example:"0"`
	StartedAt               int64                         `json:"StartedAt" example:"0"`
	FinishedAt              int64                         `json:"FinishedAt" example:"0"`
	CreatedAt               int64                         `json:"CreatedAt" example:"1783740000"`
	QueueExpiresAt          int64                         `json:"QueueExpiresAt" example:"1783740600"`
}

type PlatformReleaseStrategy struct {
	Type PlatformReleaseStrategyType `json:"Type" example:"replace"`
}

type PlatformArtifactSnapshot struct {
	ArtifactID      PlatformArtifactID         `json:"ArtifactId" example:"1"`
	Name            string                     `json:"Name,omitempty"`
	Version         string                     `json:"Version,omitempty"`
	Type            PlatformArtifactType       `json:"Type,omitempty"`
	SourceType      PlatformArtifactSourceType `json:"SourceType,omitempty"`
	ImageRef        string                     `json:"ImageRef,omitempty"`
	ImageDigest     string                     `json:"ImageDigest,omitempty"`
	Traceability    PlatformTraceability       `json:"Traceability,omitempty"`
	RegistryID      RegistryID                 `json:"RegistryId,omitempty"`
	SHA256          string                     `json:"SHA256,omitempty"`
	Size            int64                      `json:"Size,omitempty"`
	StorageProvider PlatformStorageProvider    `json:"StorageProvider,omitempty"`
	StoragePath     string                     `json:"StoragePath,omitempty"`
	Retained        bool                       `json:"Retained" example:"false"`
}

type PlatformServiceConfigSnapshot struct {
	SpecRevision            int                             `json:"SpecRevision" example:"1"`
	DesiredSpecSnapshot     PlatformDeploymentDesiredSpec   `json:"DesiredSpecSnapshot"`
	EffectiveConfigSnapshot PlatformEffectiveConfigSnapshot `json:"EffectiveConfigSnapshot"`
	SecretSnapshots         []PlatformSecretSnapshot        `json:"SecretSnapshots,omitempty"`
	ConfigHash              string                          `json:"ConfigHash,omitempty"`
}

type PlatformTargetSnapshot struct {
	TargetMode    PlatformTargetMode    `json:"TargetMode" example:"single"`
	EndpointID    EndpointID            `json:"EndpointId" example:"1"`
	NodeName      string                `json:"NodeName,omitempty"`
	HostAddress   string                `json:"HostAddress,omitempty"`
	RuntimeDriver PlatformRuntimeDriver `json:"RuntimeDriver" example:"docker-container"`
	ExecutorMode  PlatformExecutorMode  `json:"ExecutorMode" example:"single"`
}

type PlatformRuntimeSnapshot struct {
	PreviousRuntimeRef  RuntimeRef              `json:"PreviousRuntimeRef"`
	CandidateRuntimeRef RuntimeRef              `json:"CandidateRuntimeRef"`
	CurrentRuntimeRef   RuntimeRef              `json:"CurrentRuntimeRef"`
	ContainerConfigHash string                  `json:"ContainerConfigHash,omitempty"`
	PublishedPorts      []PlatformPublishedPort `json:"PublishedPorts,omitempty"`
	RetainedRuntimeRefs []RuntimeRef            `json:"RetainedRuntimeRefs,omitempty"`
}

type PlatformGatewaySnapshot struct {
	Domain              string `json:"Domain,omitempty"`
	Path                string `json:"Path,omitempty"`
	Upstream            string `json:"Upstream,omitempty"`
	ConfigHash          string `json:"ConfigHash,omitempty"`
	ReloadBeforeVersion string `json:"ReloadBeforeVersion,omitempty"`
	ReloadAfterVersion  string `json:"ReloadAfterVersion,omitempty"`
	CertificateRef      string `json:"CertificateRef,omitempty"`
}

type PlatformHealthCheckResult struct {
	Level        PlatformHealthVerification `json:"Level" example:"verified"`
	Type         PlatformHealthCheckType    `json:"Type" example:"http"`
	Status       PlatformHealthCheckStatus  `json:"Status" example:"passed"`
	Target       string                     `json:"Target,omitempty"`
	StatusCode   int                        `json:"StatusCode,omitempty"`
	ErrorMessage string                     `json:"ErrorMessage,omitempty"`
	StartedAt    int64                      `json:"StartedAt" example:"0"`
	FinishedAt   int64                      `json:"FinishedAt" example:"0"`
	LogSummary   string                     `json:"LogSummary,omitempty"`
}

type PlatformReleaseStep struct {
	Name       string                    `json:"Name" example:"validate"`
	Status     PlatformReleaseStepStatus `json:"Status" example:"pending"`
	Reason     string                    `json:"Reason,omitempty"`
	Message    string                    `json:"Message,omitempty"`
	RuntimeRef RuntimeRef                `json:"RuntimeRef"`
	StartedAt  int64                     `json:"StartedAt" example:"0"`
	FinishedAt int64                     `json:"FinishedAt" example:"0"`
	Retryable  bool                      `json:"Retryable" example:"false"`
}

type PlatformReleaseTargetResult struct {
	EndpointID     EndpointID                  `json:"EndpointId" example:"1"`
	NodeName       string                      `json:"NodeName,omitempty"`
	RuntimeRef     RuntimeRef                  `json:"RuntimeRef"`
	PublishedPorts []PlatformPublishedPort     `json:"PublishedPorts,omitempty"`
	Status         PlatformReleaseTargetStatus `json:"Status" example:"pending"`
	Reason         string                      `json:"Reason,omitempty"`
}

type PlatformPublishedPort struct {
	Name          string               `json:"Name,omitempty" example:"http"`
	ContainerPort int                  `json:"ContainerPort" example:"8080"`
	HostPort      int                  `json:"HostPort" example:"18080"`
	Protocol      PlatformPortProtocol `json:"Protocol" example:"tcp"`
	HostIP        string               `json:"HostIp,omitempty"`
}

type RuntimeRef struct {
	DriverID     PlatformRuntimeDriver       `json:"DriverId" example:"docker-container"`
	EndpointID   EndpointID                  `json:"EndpointId" example:"1"`
	ResourceType PlatformRuntimeResourceType `json:"ResourceType" example:"container"`
	ResourceID   string                      `json:"ResourceId,omitempty"`
	Name         string                      `json:"Name,omitempty"`
	Labels       map[string]string           `json:"Labels,omitempty"`
}

type PlatformReleaseLock struct {
	ID                  string                      `json:"Id" example:"1"`
	ServiceDeploymentID PlatformServiceDeploymentID `json:"ServiceDeploymentId" example:"1"`
	ReleaseID           PlatformReleaseID           `json:"ReleaseId" example:"1"`
	IdempotencyKeyHash  string                      `json:"IdempotencyKeyHash,omitempty"`
	PayloadHash         string                      `json:"PayloadHash,omitempty"`
	LeaseOwner          string                      `json:"LeaseOwner,omitempty"`
	LeaseExpiresAt      int64                       `json:"LeaseExpiresAt" example:"0"`
	CreatedAt           int64                       `json:"CreatedAt" example:"1783740000"`
	UpdatedAt           int64                       `json:"UpdatedAt" example:"1783740000"`
}

type PlatformAuditLog struct {
	ID                  PlatformAuditLogID          `json:"Id" example:"1"`
	Timestamp           int64                       `json:"Timestamp" example:"1783740000"`
	RequestID           string                      `json:"RequestId,omitempty"`
	OperatorUserID      UserID                      `json:"OperatorUserId" example:"1"`
	OperatorUsername    string                      `json:"OperatorUsername,omitempty"`
	IPAddress           string                      `json:"IpAddress,omitempty"`
	UserAgent           string                      `json:"UserAgent,omitempty"`
	Action              PlatformAuditAction         `json:"Action" example:"release.created"`
	Result              PlatformAuditResult         `json:"Result" example:"success"`
	ProjectID           PlatformProjectID           `json:"ProjectId" example:"1"`
	EnvironmentID       PlatformEnvironmentID       `json:"EnvironmentId" example:"1"`
	ApplicationID       PlatformApplicationID       `json:"ApplicationId" example:"1"`
	ServiceDefinitionID PlatformServiceDefinitionID `json:"ServiceDefinitionId" example:"1"`
	ServiceDeploymentID PlatformServiceDeploymentID `json:"ServiceDeploymentId" example:"1"`
	ReleaseID           PlatformReleaseID           `json:"ReleaseId" example:"1"`
	ArtifactID          PlatformArtifactID          `json:"ArtifactId" example:"1"`
	BeforeSummary       map[string]any              `json:"BeforeSummary,omitempty"`
	AfterSummary        map[string]any              `json:"AfterSummary,omitempty"`
	FailureReason       string                      `json:"FailureReason,omitempty"`
	SensitiveFields     []string                    `json:"SensitiveFields,omitempty"`
}

func NewPlatformLifecycle() PlatformLifecycle {
	return PlatformLifecycle{
		LifecycleStatus: PlatformLifecycleStatusActive,
		ResourceVersion: 1,
	}
}

var platformConfigEntryKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// NewPlatformConfigSet creates the safe metadata-only baseline for a configuration set.
// 敏感值的加密保存将在阶段 2 批次 4 接入；这里先固定默认配置集和版本，避免
// 后续配置 CRUD 因缺少版本基线而无法判断发布快照是否已经过期。
func NewPlatformConfigSet() PlatformConfigSet {
	return PlatformConfigSet{
		Name:              PlatformConfigSetDefaultName,
		Revision:          1,
		PlatformLifecycle: NewPlatformLifecycle(),
	}
}

// NormalizePlatformConfigSet 为直接 datastore 调用和后续 HTTP handler 应用一致的确定性默认值。
// 配置项来源必须由所属配置集的作用域推导，避免低层级条目伪装成更高优先级并破坏三级合并顺序。
func NormalizePlatformConfigSet(configSet *PlatformConfigSet) {
	if configSet == nil {
		return
	}

	configSet.Name = strings.TrimSpace(configSet.Name)
	if configSet.Name == "" {
		configSet.Name = PlatformConfigSetDefaultName
	}
	if configSet.Revision == 0 {
		configSet.Revision = 1
	}

	source := platformConfigEntrySourceForScope(configSet.ScopeType)
	for i := range configSet.Entries {
		if configSet.Entries[i].ValueType == "" {
			configSet.Entries[i].ValueType = PlatformConfigValuePlain
		}
		configSet.Entries[i].Source = source
	}
}

// ValidatePlatformConfigSet 在配置 API 尚未开放时守住阶段 2 的存储边界。
// 在敏感变量专属批次接入加密字段和密钥版本服务前，必须拒绝敏感 plain 值，避免明文进入 BoltDB。
func ValidatePlatformConfigSet(configSet PlatformConfigSet) error {
	return validatePlatformConfigSet(configSet, false)
}

// ValidatePlatformConfigSetInput 只用于 handler 在加密前校验用户输入结构。
// 它允许敏感 plain 值暂存于请求内存，调用方必须在进入 dataservice 前加密并清空 Value。
func ValidatePlatformConfigSetInput(configSet PlatformConfigSet) error {
	return validatePlatformConfigSet(configSet, true)
}

func validatePlatformConfigSet(configSet PlatformConfigSet, allowSensitivePlainValue bool) error {
	NormalizePlatformConfigSet(&configSet)

	if configSet.ProjectID <= 0 {
		return fmt.Errorf("config set project ID is required")
	}
	if configSet.ScopeID <= 0 {
		return fmt.Errorf("config set scope ID is required")
	}
	if configSet.ScopeType != PlatformConfigScopeProject &&
		configSet.ScopeType != PlatformConfigScopeEnvironment &&
		configSet.ScopeType != PlatformConfigScopeServiceDeployment {
		return fmt.Errorf("config set scope type %q is invalid", configSet.ScopeType)
	}
	if configSet.Name == "" {
		return fmt.Errorf("config set name is required")
	}

	keys := make(map[string]struct{}, len(configSet.Entries))
	for _, entry := range configSet.Entries {
		if !platformConfigEntryKeyPattern.MatchString(entry.Key) {
			return fmt.Errorf("config entry key %q is invalid", entry.Key)
		}
		if _, exists := keys[entry.Key]; exists {
			return fmt.Errorf("config entry key %q is duplicated", entry.Key)
		}
		keys[entry.Key] = struct{}{}

		if entry.ValueType != PlatformConfigValuePlain &&
			entry.ValueType != PlatformConfigValueSecretRef &&
			entry.ValueType != PlatformConfigValueDatabaseRef &&
			entry.ValueType != PlatformConfigValueRedisRef {
			return fmt.Errorf("config entry %q value type %q is invalid", entry.Key, entry.ValueType)
		}
		if !allowSensitivePlainValue && entry.Sensitive && entry.ValueType == PlatformConfigValuePlain && entry.Value != "" {
			return fmt.Errorf("sensitive config entry %q cannot store a plain value before encrypted storage is enabled", entry.Key)
		}
	}

	return nil
}

func platformConfigEntrySourceForScope(scope PlatformConfigScopeType) PlatformConfigEntrySource {
	switch scope {
	case PlatformConfigScopeProject:
		return PlatformConfigEntrySourceProject
	case PlatformConfigScopeEnvironment:
		return PlatformConfigEntrySourceEnvironment
	case PlatformConfigScopeServiceDeployment:
		return PlatformConfigEntrySourceServiceDeployment
	default:
		return ""
	}
}

// NewPlatformDeploymentDesiredSpec applies V0.1 defaults before any API layer exists;
// later handlers can normalize user payloads without knowing Docker executor details.
func NewPlatformDeploymentDesiredSpec() PlatformDeploymentDesiredSpec {
	spec := PlatformDeploymentDesiredSpec{}
	NormalizePlatformDeploymentDesiredSpec(&spec)
	return spec
}

func NormalizePlatformDeploymentDesiredSpec(spec *PlatformDeploymentDesiredSpec) {
	if spec == nil {
		return
	}

	if spec.Image.PullPolicy == "" {
		spec.Image.PullPolicy = PlatformImagePullPolicyIfNotPresent
	}
	if spec.Image.Traceability == "" {
		spec.Image.Traceability = PlatformTraceabilityWeak
	}

	for i := range spec.Ports {
		if spec.Ports[i].Protocol == "" {
			spec.Ports[i].Protocol = PlatformPortProtocolTCP
		}
		if spec.Ports[i].ExposeMode == "" {
			spec.Ports[i].ExposeMode = PlatformPortExposeModePublished
		}
	}

	for i := range spec.EnvOverrides {
		if spec.EnvOverrides[i].Source == "" {
			spec.EnvOverrides[i].Source = PlatformEnvVarSourceLiteral
		}
	}

	if spec.HealthCheck.VerificationLevel == "" {
		spec.HealthCheck.VerificationLevel = PlatformHealthVerificationVerified
	}
	if spec.HealthCheck.Type == "" {
		spec.HealthCheck.Type = PlatformHealthCheckTypeHTTP
	}
	if spec.HealthCheck.Path == "" && spec.HealthCheck.Type == PlatformHealthCheckTypeHTTP {
		spec.HealthCheck.Path = "/health"
	}
	if spec.HealthCheck.IntervalSeconds == 0 {
		spec.HealthCheck.IntervalSeconds = 10
	}
	if spec.HealthCheck.TimeoutSeconds == 0 {
		spec.HealthCheck.TimeoutSeconds = 3
	}
	if spec.HealthCheck.Retries == 0 {
		spec.HealthCheck.Retries = 3
	}
	if spec.HealthCheck.StartPeriodSeconds == 0 {
		spec.HealthCheck.StartPeriodSeconds = 30
	}

	if spec.Runtime.RuntimeDriver == "" {
		spec.Runtime.RuntimeDriver = PlatformRuntimeDriverDockerContainer
	}
	if spec.Runtime.Replicas == 0 {
		spec.Runtime.Replicas = 1
	}
	if spec.Runtime.RestartPolicy == "" {
		spec.Runtime.RestartPolicy = PlatformRuntimeRestartPolicyUnlessStopped
	}
	if spec.Runtime.StopTimeoutSeconds == 0 {
		spec.Runtime.StopTimeoutSeconds = 10
	}
	if spec.Runtime.ContainerRetentionCount == 0 {
		spec.Runtime.ContainerRetentionCount = 1
	}
	if spec.Runtime.ContainerRetentionDays == 0 {
		spec.Runtime.ContainerRetentionDays = 7
	}

	if spec.Strategy.Type == "" {
		spec.Strategy.Type = PlatformReleaseStrategyReplace
	}
}

func NewPlatformReleasePolicy() PlatformReleasePolicy {
	return PlatformReleasePolicy{Type: PlatformReleasePolicyReplace}
}

func NewPlatformReleaseStrategy() PlatformReleaseStrategy {
	return PlatformReleaseStrategy{Type: PlatformReleaseStrategyReplace}
}

// ValidatePlatformDeploymentDesiredSpecV01 captures only Gate 0A-safe control-plane checks;
// it intentionally does not inspect Docker, pull images, allocate ports, or create containers.
func ValidatePlatformDeploymentDesiredSpecV01(spec PlatformDeploymentDesiredSpec) error {
	NormalizePlatformDeploymentDesiredSpec(&spec)

	if spec.Runtime.RuntimeDriver != PlatformRuntimeDriverDockerContainer {
		return fmt.Errorf("runtime driver %q is not supported in V0.1", spec.Runtime.RuntimeDriver)
	}
	if spec.Runtime.Replicas != 1 {
		return fmt.Errorf("replicas must be 1 in V0.1")
	}
	if spec.Strategy.Type != PlatformReleaseStrategyReplace {
		return fmt.Errorf("release strategy %q is not supported in V0.1", spec.Strategy.Type)
	}

	return nil
}
