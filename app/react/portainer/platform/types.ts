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
  StartedAt?: number;
  FinishedAt?: number;
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
  Steps?: PlatformReleaseStep[];
}
