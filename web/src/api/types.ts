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

export interface DashboardNamespaceHealth {
  namespace: string
  problematicPods: number
  containerRestarts: number
  degradedWorkloads: number
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
  page?: CollectionPageMeta
  coverage?: DashboardCoverage | null
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

export interface CollectionPageMeta {
  limit: number
  next: string
  complete: boolean
  truncated: boolean
  filterScope: 'page' | 'collection'
}

export interface CollectionResult<T> {
  items: T[]
  page: CollectionPageMeta
  coverage: DashboardCoverage | null
  generation?: string
  collectedAt?: string
}

export interface ResourceListQuery extends PageQuery {
  namespaces?: string[]
  kinds?: string[]
  statuses?: string[]
  sort?: string
  order?: 'asc' | 'desc'
  workload?: string
  node?: string
  restarts?: 'any' | 'gt0' | 'gte3' | 'gte10'
  problematic?: boolean
  objectKind?: string
  reason?: string
  addressType?: string
}

export interface ResourceMetadata {
  namespace: string
  name: string
  uid: string
  resourceVersion: string
  creationTimestamp: string
  labels: Record<string, string>
}

export interface Condition {
  type: string
  status: 'True' | 'False' | 'Unknown' | string
  reason: string | null
  message: string | null
  lastTransitionTime: string | null
}

export interface ContainerPort {
  name: string | null
  containerPort: number
  protocol: string
}

export interface ContainerSpec {
  name: string
  image: string
  ports: ContainerPort[]
}

export type WorkloadStatus = 'Healthy' | 'Progressing' | 'Degraded' | 'Suspended' | 'Completed' | 'Failed' | 'Unknown'

export interface Workload {
  namespace: string
  kind: 'Deployment' | 'StatefulSet' | 'DaemonSet' | 'Job' | 'CronJob'
  name: string
  ready: number | null
  desired: number | null
  available: number | null
  updated: number | null
  status: WorkloadStatus
  ageSeconds: number
}

export interface WorkloadDetail {
  metadata: ResourceMetadata
  kind: Workload['kind']
  ready: number | null
  desired: number | null
  available: number | null
  updated: number | null
  status: WorkloadStatus
  selector: Record<string, string> | null
  restartAt: string | null
  conditions: Condition[]
  containers: ContainerSpec[]
  related: ResourceRef[]
}

export interface ReadyCount {
  current: number
  desired: number
}

export interface ResourceOwner {
  kind: string
  name: string
}

export interface Pod {
  namespace: string
  name: string
  status: 'Running' | 'Pending' | 'Succeeded' | 'Failed' | 'Unknown'
  ready: ReadyCount
  restarts: number
  node: string | null
  ip: string | null
  owner: ResourceOwner | null
  ageSeconds: number
  problematic: boolean
}

export interface PodContainer {
  spec: ContainerSpec
  type: ContainerType
  ready: boolean | null
  restartCount: number
  state: 'waiting' | 'running' | 'terminated' | 'unknown'
  reason: string | null
}

export interface PodDetail {
  metadata: ResourceMetadata
  summary: Pod
  conditions: Condition[]
  containers: PodContainer[]
  initContainers: PodContainer[]
  ephemeralContainers: PodContainer[]
  relatedEvents: ResourceRef[]
}

export interface EventResource {
  timestamp: string | null
  namespace: string
  objectKind: string
  objectName: string
  reason: string
  message: string
  count: number
  source: string | null
  type: 'Normal' | 'Warning' | 'Unknown'
}

export interface TypedValue {
  type: 'number' | 'name'
  value: number | string
}

export interface ServicePort {
  name: string | null
  protocol: string
  port: number
  targetPort: TypedValue
  nodePort: number | null
  appProtocol: string | null
}

export interface ServiceResource {
  namespace: string
  name: string
  type: string
  clusterIPs: string[]
  ports: ServicePort[]
  selector: Record<string, string> | null
  externalEndpoints: Array<{ address: string; port: number; protocol: string }>
}

export interface ServiceDetail {
  metadata: ResourceMetadata
  summary: ServiceResource
  sessionAffinity: string
  externalTrafficPolicy: string | null
  ipFamilies: string[]
  healthCheckNodePort: number | null
}

export interface IngressBackend {
  serviceName: string
  servicePort: TypedValue
}

export interface IngressPath {
  host: string
  path: string
  pathType: string
  backend: IngressBackend
}

export interface IngressResource {
  namespace: string
  name: string
  className: string | null
  hosts: string[]
  paths: IngressPath[]
  tlsHosts: string[]
}

export interface IngressDetail {
  metadata: ResourceMetadata
  summary: IngressResource
  defaultBackend: IngressBackend | null
  loadBalancerAddresses: string[]
}

export interface EndpointSliceResource {
  namespace: string
  name: string
  addressType: 'IPv4' | 'IPv6' | 'FQDN' | 'Unknown'
  ports: Array<{ name: string | null; protocol: string | null; port: number | null; appProtocol: string | null }>
  endpoints: Array<{
    addresses: string[]
    hostname: string | null
    nodeName: string | null
    zone: string | null
    conditions: { ready: boolean | null; serving: boolean | null; terminating: boolean | null }
    targetRef: ResourceRef | null
  }>
}

export interface EndpointSliceDetail {
  metadata: ResourceMetadata
  summary: EndpointSliceResource
}

export interface ConfigMapResource {
  namespace: string
  name: string
  uid: string
  creationTimestamp: string
}

export interface ConfigMapDetail {
  metadata: ResourceMetadata
  entries: Array<{ key: string; encoding: 'utf-8' | 'base64'; value: string; truncated: boolean }>
  totalBytes: number
  truncated: boolean
}

export interface SecretMetadata {
  apiVersion: 'v1'
  kind: 'Secret'
  metadata: {
    name: string
    namespace: string
    uid: string
    creationTimestamp: string
    deletionTimestamp?: string
  }
}

export interface NodeSummary {
  name: string
  status: string
  ready: boolean
  roles: string[]
  kubeletVersion: string
  internalIP: string | null
  ageSeconds: number
}

export interface NodeTaint {
  key: string
  value: string
  effect: string
}

export interface NodeDetail {
  metadata: ResourceMetadata
  status: string
  ready: boolean
  roles: string[]
  kubeletVersion: string
  internalIP: string | null
  conditions: Condition[]
  capacity: Record<string, string> | null
  allocatable: Record<string, string> | null
  taints: NodeTaint[]
  truncated: boolean
}

export interface Lease {
  namespace: string
  name: string
  holderName: string
  durationSeconds: number
  renewTime: string | null
  ageSeconds: number
}

export interface LeaseDetail {
  metadata: ResourceMetadata
  holderName: string
  durationSeconds: number
  renewTime: string | null
  acquireTime: string | null
}

export interface VolumeClaimRef {
  namespace: string
  name: string
}

export interface PersistentVolume {
  name: string
  status: string
  capacity: string
  accessModes: string[]
  reclaimPolicy: string
  storageClass: string
  claim: VolumeClaimRef | null
  ageSeconds: number
}

export interface PersistentVolumeDetail {
  metadata: ResourceMetadata
  status: string
  capacity: Record<string, string> | null
  accessModes: string[]
  reclaimPolicy: string
  storageClass: string
  volumeMode: string
  claim: VolumeClaimRef | null
  reason: string | null
  message: string | null
  omitted: string[]
}

export interface PersistentVolumeClaim {
  namespace: string
  name: string
  status: string
  volumeName: string
  capacity: string | null
  accessModes: string[]
  storageClass: string | null
  ageSeconds: number
}

export interface PersistentVolumeClaimDetail {
  metadata: ResourceMetadata
  status: string
  volumeName: string
  capacity: Record<string, string> | null
  accessModes: string[]
  storageClass: string | null
  volumeMode: string
  conditions: Condition[]
  truncated: boolean
}

export interface StorageClass {
  name: string
  provisioner: string
  default: boolean
  reclaimPolicy: string
  volumeBindingMode: string
  allowVolumeExpansion: boolean
  ageSeconds: number
}

export interface StorageClassDetail {
  metadata: ResourceMetadata
  provisioner: string
  default: boolean
  reclaimPolicy: string
  volumeBindingMode: string
  allowVolumeExpansion: boolean
  omitted: string[]
}

export interface CSIDriver {
  name: string
  attachRequired: boolean
  podInfoOnMount: boolean
  storageCapacity: boolean
  ageSeconds: number
}

export interface CSIDriverDetail {
  metadata: ResourceMetadata
  attachRequired: boolean
  podInfoOnMount: boolean
  storageCapacity: boolean
  fsGroupPolicy: string
}

export interface CSINode {
  name: string
  driverCount: number
  ageSeconds: number
}

export interface CSINodeDriver {
  name: string
  nodeID: string
  topologyKeys: string[]
}

export interface CSINodeDetail {
  metadata: ResourceMetadata
  driverCount: number
  drivers: CSINodeDriver[]
  truncated: boolean
}

export interface VolumeAttachment {
  name: string
  nodeName: string
  attacher: string
  volumeName: string
  attached: boolean
  ageSeconds: number
}

export interface VolumeAttachmentDetail {
  metadata: ResourceMetadata
  nodeName: string
  attacher: string
  volumeName: string
  persistentVolumeName: string
  attached: boolean
  omitted: string[]
}

export interface NamespaceObjectDetail {
  metadata: ResourceMetadata
  status: string
  conditions: Condition[]
}

export interface LogLine {
  timestamp: string | null
  text: string
  truncated: boolean
}

export interface LogRead {
  container: string
  previous: boolean
  lines: LogLine[]
  truncated: boolean
}

export interface LogQuery {
  container: string
  previous?: boolean
  timestamps?: boolean
  tailLines?: number
  since?: string
}

export type SavedFilterCollection = 'workloads' | 'pods' | 'events' | 'logs'

export interface SavedFilter {
  id: string
  name: string
  query: Record<string, unknown>
}

export interface SavedFilterSet {
  version: 1
  items: SavedFilter[]
}

export interface Preferences {
  version: 1
  ui: { language: 'en' | 'pt-BR' }
  logs: { wrap: boolean; timestamps: boolean; tailLines: number }
  dashboard: { logScanWindow: '15m' | '30m' | '1h' | '4h'; sectionOrder: string[]; hiddenSections: string[] }
  filters: Record<SavedFilterCollection, SavedFilterSet>
  favorites?: FavoriteSet
}

export type FavoriteKind =
  | 'pod'
  | 'deployment'
  | 'statefulset'
  | 'daemonset'
  | 'job'
  | 'cronjob'
  | 'service'
  | 'ingress'
  | 'endpointslice'
  | 'configmap'
  | 'secret'

export interface FavoriteItem {
  id: string
  kind: FavoriteKind
  namespace: string
  name: string
}

export interface FavoriteSet {
  version: 1
  items: FavoriteItem[]
}

export interface YAMLDiffLine {
  kind: 'same' | 'added' | 'removed'
  text: string
}

export interface YAMLDiff {
  absent: boolean
  truncated: boolean
  lines: YAMLDiffLine[]
}

export interface ActionTarget {
  clusterProfileId: number
  context: string
  namespace: string
  kind: 'Deployment' | 'StatefulSet' | 'Pod'
  name: string
}

export interface ConfirmedAction {
  confirmed: true
  target: ActionTarget
  expectedGeneration: string
}

export interface RestartActionRequest extends ConfirmedAction {
  action: 'restart'
  consequenceCode: 'RECREATE_WORKLOAD_PODS'
  expectedResourceVersion: string
}

export interface ScaleActionRequest extends ConfirmedAction {
  replicas: number
  action: 'scale'
  consequenceCode: 'CHANGE_REPLICA_COUNT'
  expectedResourceVersion: string
}

export interface PodDeleteActionRequest extends ConfirmedAction {
  action: 'deletePod'
  consequenceCode: 'DELETE_POD'
  expectedUid: string
  expectedResourceVersion: string
}

export interface PortForwardCreateRequest extends ConfirmedAction {
  remotePort: number
  localPort: number | null
  action: 'portForward'
  consequenceCode: 'EXPOSE_POD_PORT_LOCALLY'
}

export interface ExecInit extends ConfirmedAction {
  container: string
  command: string[]
  tty: boolean
  stdin: boolean
  action: 'exec'
  consequenceCode: 'OPEN_INTERACTIVE_PROCESS'
}

export interface ActionAccepted {
  accepted: true
  action: string
  target: ActionTarget
  generation: string
  resourceVersion: string | null
  replicas?: number
}

export interface PortForward {
  id: string
  clusterProfileId: number
  context: string
  generation: string
  namespace: string
  pod: string
  remotePort: number
  localAddress: '127.0.0.1'
  localPort: number
  status: 'active' | 'closed' | 'expired' | 'podGone' | 'failed'
  createdAt: string
  expiresAt: string
  endedAt: string | null
  endReason: string | null
}

export interface ExecTicket {
  sessionId: string
  websocketUrl: string
  protocols: string[]
  expiresAt: string
}
