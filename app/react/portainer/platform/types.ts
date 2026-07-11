export type PlatformLifecycleStatus = 'active' | 'archived';
export type PlatformReleaseStatus =
  | 'queued'
  | 'validating'
  | 'pulling'
  | 'preparing'
  | 'candidate-starting'
  | 'candidate-checking'
  | 'switching'
  | 'final-checking'
  | 'recovering'
  | 'interrupted'
  | 'succeeded'
  | 'failed'
  | 'recovery-failed'
  | 'canceled'
  | 'resolved';

export interface PlatformLifecycle {
  LifecycleStatus: PlatformLifecycleStatus;
  ArchivedAt?: number;
  CreatedAt?: number;
  UpdatedAt?: number;
  ResourceVersion: number;
}

export interface PlatformProject extends PlatformLifecycle {
  Id: number;
  Name: string;
  Slug: string;
  Description?: string;
}

export interface PlatformApplication extends PlatformLifecycle {
  Id: number;
  ProjectId: number;
  Name: string;
  Slug: string;
  Description?: string;
}

export interface PlatformServiceDefinition extends PlatformLifecycle {
  Id: number;
  ProjectId: number;
  ApplicationId: number;
  Name: string;
  Slug: string;
  Type: string;
  Description?: string;
}

export interface PlatformRuntimeRef {
  DriverId?: string;
  EndpointId?: number;
  NodeName?: string;
  ResourceType?: string;
  ResourceId?: string;
  Name?: string;
}

export interface PlatformPublishedPort {
  Name?: string;
  ContainerPort: number;
  HostPort?: number;
  Protocol?: string;
  HostIP?: string;
}

export interface PlatformDeploymentDesiredSpec {
  Image?: {
    Image?: string;
    RegistryId?: number;
    PullPolicy?: string;
    ResolvedDigest?: string;
    Traceability?: string;
  };
  Ports?: PlatformPublishedPort[];
  HealthCheck?: {
    VerificationLevel?: string;
    Type?: string;
    Path?: string;
    Port?: number;
    Retries?: number;
    TimeoutSeconds?: number;
  };
  Runtime?: {
    RuntimeDriver?: string;
    Replicas?: number;
    RestartPolicy?: string;
  };
  Strategy?: {
    Type?: string;
  };
}

export interface PlatformServiceDeployment extends PlatformLifecycle {
  Id: number;
  ProjectId: number;
  EnvironmentId: number;
  ApplicationId: number;
  ServiceDefinitionId: number;
  DesiredSpec: PlatformDeploymentDesiredSpec;
  SpecRevision: number;
  LastDeployedSpecRevision: number;
  LastDeployedAt?: number;
  CurrentServingReleaseId?: number;
  CurrentArtifactId?: number;
  CurrentRuntimeRef?: PlatformRuntimeRef;
  CurrentImage?: string;
  DriftStatus?: string;
  LastObservedAt?: number;
}

export interface PlatformArtifact extends PlatformLifecycle {
  Id: number;
  ProjectId: number;
  ApplicationId?: number;
  ServiceDefinitionId?: number;
  Name: string;
  Version: string;
  Type: string;
  SourceType: string;
  ImageRef?: string;
  ImageDigest?: string;
  SHA256?: string;
  Traceability?: string;
}

export interface PlatformReleaseStep {
  Name: string;
  Status: string;
  Reason?: string;
  Message?: string;
  RuntimeRef?: PlatformRuntimeRef;
  StartedAt?: number;
  FinishedAt?: number;
  Retryable?: boolean;
}

export interface PlatformRuntimeSnapshot {
  PreviousRuntimeRef?: PlatformRuntimeRef;
  CandidateRuntimeRef?: PlatformRuntimeRef;
  CurrentRuntimeRef?: PlatformRuntimeRef;
  ContainerConfigHash?: string;
  PublishedPorts?: PlatformPublishedPort[];
  RetainedRuntimeRefs?: PlatformRuntimeRef[];
}

export interface PlatformHealthCheckResult {
  Level?: string;
  Type?: string;
  Status?: string;
  Target?: string;
  StatusCode?: number;
  ErrorMessage?: string;
  StartedAt?: number;
  FinishedAt?: number;
  LogSummary?: string;
}

export interface PlatformRelease {
  Id: number;
  ProjectId: number;
  EnvironmentId: number;
  ApplicationId: number;
  ServiceDefinitionId: number;
  ServiceDeploymentId: number;
  ArtifactId: number;
  Version: string;
  Status: PlatformReleaseStatus;
  Image?: string;
  ImageDigest?: string;
  FailureReason?: string;
  ManualActionRequired?: boolean;
  CreatedAt?: number;
  StartedAt?: number;
  FinishedAt?: number;
  RuntimeSnapshot?: PlatformRuntimeSnapshot;
  HealthCheckResult?: PlatformHealthCheckResult;
  ResolutionAction?: string;
  ResolvedAt?: number;
  ResolutionComment?: string;
  Steps?: PlatformReleaseStep[];
}

export interface PlatformServiceDeploymentStatus {
  ServiceDeploymentId: number;
  CurrentServingReleaseId?: number;
  CurrentArtifactId?: number;
  CurrentImage?: string;
  SpecRevision: number;
  LastDeployedSpecRevision: number;
  LastDeployedAt?: number;
  DriftStatus?: string;
  RuntimeRef?: PlatformRuntimeRef;
  RuntimeFound: boolean;
  RuntimeRunning: boolean;
  RuntimeState?: string;
  RuntimeStatus?: string;
  RuntimeMessage?: string;
  RestartCount: number;
  PublishedPorts?: PlatformPublishedPort[];
  Reason?: string;
}

export interface PlatformServiceDeploymentLogs {
  ServiceDeploymentId: number;
  RuntimeRef?: PlatformRuntimeRef;
  Available: boolean;
  Tail: number;
  Logs?: string;
  Reason?: string;
}

export type PlatformReleaseResolutionAction =
  | 'accept-current'
  | 'mark-handled'
  | 'release-lock-only';
