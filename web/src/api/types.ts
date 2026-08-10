export type ComponentStatus = 'healthy' | 'degraded' | 'unhealthy' | 'unknown'

export interface ComponentState {
  status: ComponentStatus
  code: string
  message: string
  checkedAt: string | null
}

export type NamespaceScopeMode = 'single' | 'list' | 'all'
export type NamespaceScopeSource = 'saved' | 'cli' | 'none'

export interface SelectionSummary {
  clusterProfileId: number
  context: string
  cluster: string
  scopeId: number | null
  scopeName: string | null
  scopeMode: NamespaceScopeMode | null
  scopeSource: NamespaceScopeSource
  defaultNamespace: string | null
  namespaceCount: number
  generation: string
}

export interface SelectionData extends SelectionSummary {
  components?: Partial<Record<'cluster', ComponentState>>
}

export interface StatusData {
  version: string
  commit: string
  buildDate: string
  port: number
  components: Record<'application' | 'sqlite' | 'kubeconfig' | 'context' | 'cluster' | 'metrics', ComponentState>
  selection: SelectionSummary | null
}

export interface SessionData {
  csrfToken: string
  origin: string
  generation: string
  expiresAt: string
}

export interface KubeconfigFileDisplay {
  position: number
  displayPath: string
}

export interface ClusterProfile {
  id: number
  name: string
  context: string | null
  isDefault: boolean
  kubeconfigFiles: KubeconfigFileDisplay[]
}

export interface KubernetesContext {
  clusterProfileId: number
  name: string
  cluster: string
  selected: boolean
}

export interface SelectContextRequest {
  clusterProfileId: number
  context: string
  setDefault: boolean
  expectedGeneration: string
}

export interface Namespace {
  name: string
  phase: string
  selected: boolean
}

export interface NamespaceScope {
  id: number
  clusterProfileId: number
  name: string
  context: string
  mode: NamespaceScopeMode
  namespaces: string[]
  defaultNamespace: string | null
  version: number
  createdAt: string
  updatedAt: string
}

export interface InvalidNamespace {
  input: string
  code: 'INVALID_NAMESPACE_NAME' | string
}

export interface NamespaceExistence {
  checked: boolean
  reasonCode?: string
}

export interface NamespaceScopeValidation {
  valid: string[]
  validCount: number
  duplicateCount: number
  discardedEmptyCount: number
  invalid: InvalidNamespace[]
  invalidCount: number
  existence: NamespaceExistence
}

export interface NamespaceScopeInput {
  clusterProfileId: number
  name?: string
  context: string
  mode: NamespaceScopeMode
  namespaces?: string[]
  rawInput?: string
  defaultNamespace?: string | null
}

export interface NamespaceScopeWriteRequest extends NamespaceScopeInput {
  name: string
}

export interface NamespaceScopeUpdateRequest extends NamespaceScopeWriteRequest {
  version: number
  expectedGeneration: string
}

export interface NamespaceScopeDeleteRequest {
  confirmed: true
  version: number
  replacementScopeId?: number
  expectedGeneration: string
}

export interface SelectNamespaceScopeRequest {
  expectedGeneration: string
}

export type CapabilityDecision = 'allowed' | 'denied' | 'unknown'

export interface Capability {
  capabilityId: string
  namespace: string
  apiGroup: string
  resource: string
  subresource: string
  verb: string
  resourceName: string
  decision: CapabilityDecision
  reasonCode: string
  expiresAt: string | null
}

export interface CapabilityError {
  namespace?: string
  code: string
  message: string
}

export interface CapabilityMatrix {
  generation: string
  decisions: Capability[]
  complete: boolean
  truncated: boolean
  errors: CapabilityError[]
}

export interface PermissionQuery {
  namespaces?: string[]
  capabilityIds?: string[]
  resourceNames?: string[]
  refresh?: boolean
}

export type CounterState = 'available' | 'denied' | 'unavailable' | 'notCollected' | 'collecting' | 'truncated'

export interface DashboardCounter {
  state: CounterState
  value: number | null
}

export interface DashboardSummary {
  namespaces: DashboardCounter
  podsTotal: DashboardCounter
  podsHealthy: DashboardCounter
  podsProblematic: DashboardCounter
  workloadsDegraded: DashboardCounter
  restarts: DashboardCounter
  warningEvents: DashboardCounter
  possibleLogMatches: DashboardCounter
}

export interface DashboardPartialError {
  namespace?: string
  code: string
  message: string
}

export interface DashboardCoverage {
  requestedNamespaces: number
  completedNamespaces: number
  deniedNamespaces: string[]
  failed: DashboardPartialError[]
}

export interface DashboardBlock<T> {
  value: T
  complete: boolean
  truncated: boolean
  coverage: DashboardCoverage | null
  errors: DashboardPartialError[]
}

export interface DashboardResponse<T> {
  block: DashboardBlock<T>
  meta?: EnvelopeMeta
}

export interface ResourceRef {
  apiGroup?: string
  kind: string
  namespace?: string
  name: string
  uid?: string
}

export type ContainerType = 'regular' | 'init' | 'ephemeral'
export type RestartSeverity = 'healthy' | 'attention' | 'warning' | 'critical'

export interface DashboardRestart {
  namespace: string
  pod: string
  owner: ResourceRef | null
  container: string
  containerType: ContainerType
  restarts: number
  severity: RestartSeverity
  status: string
  lastReason: string
  ageSeconds: number
}

export type ProblemSource = 'podStatus' | 'containerWaiting' | 'containerTerminated' | 'containerStatus' | 'condition' | 'event'
export type ProblemSeverity = 'warning' | 'critical'

export interface DashboardProblem {
  namespace: string
  pod: string
  owner: ResourceRef | null
  container: string | null
  containerType: ContainerType | null
  status: string
  reason: string | null
  message: string | null
  source: ProblemSource
  severity: ProblemSeverity
  ageSeconds: number
}

export interface DashboardEvent {
  timestamp: string | null
  namespace: string
  objectKind: string
  objectName: string
  reason: string
  message: string
  count: number
  source: string | null
  type: string
}

export type LogReasonCode = 'ERROR_KEYWORD' | 'JSON_ERROR_LEVEL' | 'JSON_ERROR_FIELD' | 'STACK_TRACE' | 'OOM' | 'PANIC'

export interface DashboardLogMatch {
  namespace: string
  pod: string
  container: string
  workload: ResourceRef | null
  timestamp: string | null
  excerpt: string
  reasonCode: LogReasonCode
  redacted: boolean
  truncated: boolean
}

export interface LogScanRequest {
  window: '15m' | '30m' | '1h' | '4h'
  tailLines: number
  maxPods: number
  maxConcurrentContainers: number
}

export interface ContainerMetric {
  name: string
  cpuMillicores: number
  memoryBytes: number
}

export interface PodMetric {
  namespace: string
  pod: string
  cpuMillicores: number
  memoryBytes: number
  containers: ContainerMetric[]
}

export interface MetricRank {
  namespace: string
  pod: string
  cpuMillicores: number
  memoryBytes: number
}

export interface DashboardMetrics {
  collectedAt: string
  windowSeconds: number
  pods: PodMetric[]
  topCPU: MetricRank[]
  topMemory: MetricRank[]
}

export interface PageQuery {
  limit?: number
  continueToken?: string
  search?: string
}

export interface EnvelopeMeta {
  requestId?: string
  generation?: string
  collectedAt?: string
  continue?: string
}

export interface Envelope<T> {
  data: T
  meta?: EnvelopeMeta
}

export interface APIErrorPayload {
  code?: string
  message?: string
  requestId?: string
  details?: unknown
}
