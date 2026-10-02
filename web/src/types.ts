export interface Page<T> {
  items: T[];
  nextCursor?: string;
}
export interface Session {
  actor: string;
  role: "viewer" | "operator";
  csrf?: string;
  expiresAt?: string;
}
export interface RuntimeConfig {
  securityEnabled: boolean;
  loginURL: string;
  grafanaURL?: string;
}
export interface Context {
  id: string;
  owner: string;
  name: string;
  version: string;
  state: string;
  condition?: string;
  createdAt: string;
  observedAt: string;
  metricContext: string;
  liveClients: number;
  queuedJobs: number;
  runningJobs: number;
  readyWorkers: number;
  desiredInstances: number;
  spec: {
    image: string;
    engine: { type: string; version: string; settings: unknown };
    results: unknown;
  };
}
export interface Client {
  hostname?: string;
  id: string;
  clientId: string;
  state: string;
  lastRenewedAt?: string;
  expiresAt?: string;
}
export interface Instance {
  id: string;
  contextId: string;
  incarnation: string;
  resourceKind: string;
  resourceUid: string;
  state: string;
  observedAt?: string;
  stale: boolean;
  available: boolean;
  detail?: {
    application?: string;
    applicationState?: string;
    driverPod?: string;
    driverPhase?: string;
    executorStates?: Record<string, number>;
    resources?: Record<string, number>;
    links?: { name: string; url: string }[];
    uiService?: string;
    uiPort?: number;
    condition?: string;
  };
}
export interface Worker {
  id: string;
  incarnation: string;
  state: string;
  capacity: number;
  occupiedSlots: number;
  heartbeatAt: string;
  capabilities: {
    handler: string;
    version: number;
    result: { kind: string; schema: string; version: number };
  }[];
}
export interface Failure {
  code: string;
  message: string;
  exceptionType?: string;
  stackTrace?: string;
  stackTraceTruncated?: boolean;
  phase?: string;
}
export interface RequestInput {
  handler: string;
  version: number;
  payload: unknown;
  idempotencyKey: string;
  retry: { maxAttempts: number; backoffSeconds: number };
  deadline?: string;
}
export interface Job {
  id: string;
  owner: string;
  contextId: string;
  contextName: string;
  contextVersion: string;
  metricContext: string;
  state: string;
  submittedAt: string;
  updatedAt: string;
  attemptCount: number;
  request: RequestInput;
  progress?: { fraction: number; message: string };
  failure?: Failure;
  result?: { descriptor: { kind: string }; expired: boolean };
  submittedBy?: string;
  replayedFrom?: string;
}
export interface Attempt {
  id: string;
  number: number;
  state: string;
  workerId: string;
  startedAt: string;
  finishedAt?: string;
  failure?: Failure;
}
export interface Audit {
  id: string;
  actor: string;
  action: string;
  acceptedAt: string;
  outcome: string;
  replayedFrom?: string;
}
export interface ResultFile {
  allocationId: string;
  name: string;
  size: number;
  contentType: string;
  sha256: string;
}
export interface Result {
  descriptor: { kind: string; schema: string; version: number };
  expired: boolean;
  expiresAt: string;
  files: { file: ResultFile; location: string; unavailable?: string }[];
  dataset?: {
    location: string;
    available: boolean;
    metadata: { format: string; partCount: number; totalBytes: number };
  };
}
export interface Part {
  path: string;
  size: number;
  sha256: string;
}
export interface Preview {
  text: string;
  truncated: boolean;
  validJSON: boolean;
  maxBytes: number;
}
export interface Server {
  id: string;
  pod: string;
  version: string;
  startedAt: string;
  heartbeatAt: string;
  state: string;
  detail: {
    ready?: boolean;
    database?: boolean;
    localStorage?: boolean;
    observedAt?: string;
    [key: string]: unknown;
  };
}

export interface OverviewData {
  observedAt: string;
  since: string;
  runningJobs: number;
  queuedJobs: number;
  retryingJobs: number;
  cancellingJobs: number;
  succeededJobs24h: number;
  failedJobs24h: number;
  cancelledJobs24h: number;
  oldestQueuedSeconds: number;
  meanProcessingSeconds24h: number | null;
  activeContexts: number;
  liveClients: number;
  readyWorkers: number;
  workerSlots: number;
  occupiedSlots: number;
  serversReady: number;
  serversUnready: number;
  serversStale: number;
  activeContextDetails: {
    id: string;
    name: string;
    owner: string;
    state: string;
    condition?: string;
    runningJobs: number;
    queuedJobs: number;
  }[];
  recentFailures: {
    id: string;
    contextId: string;
    contextName: string;
    owner: string;
    handler: string;
    message: string;
    updatedAt: string;
  }[];
}
