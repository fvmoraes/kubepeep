import type {
  APIErrorPayload,
  CapabilityMatrix,
  ClusterProfile,
  CollectionResult,
  ConfigMapDetail,
  ConfigMapResource,
  DashboardBlock,
  DashboardEvent,
  DashboardLogMatch,
  DashboardMetrics,
  DashboardNamespaceHealth,
  DashboardProblem,
  DashboardResponse,
  DashboardRestart,
  DashboardSummary,
  Envelope,
  EndpointSliceDetail,
  EndpointSliceResource,
  EventResource,
  ExecTicket,
  IngressDetail,
  IngressResource,
  KubernetesContext,
  CSIDriver,
  CSIDriverDetail,
  CSINode,
  CSINodeDetail,
  Lease,
  LeaseDetail,
  NamespaceObjectDetail,
  NodeDetail,
  NodeSummary,
  PersistentVolume,
  PersistentVolumeClaim,
  PersistentVolumeClaimDetail,
  PersistentVolumeDetail,
  StorageClass,
  StorageClassDetail,
  VolumeAttachment,
  VolumeAttachmentDetail,
  YAMLDiff,
  LogScanRequest,
  LogQuery,
  LogRead,
  Namespace,
  NamespaceScope,
  NamespaceScopeDeleteRequest,
  NamespaceScopeInput,
  NamespaceScopeUpdateRequest,
  NamespaceScopeValidation,
  NamespaceScopeWriteRequest,
  PageQuery,
  Pod,
  PodDeleteActionRequest,
  PodDetail,
  PortForward,
  PortForwardCreateRequest,
  Preferences,
  PermissionQuery,
  SelectContextRequest,
  SelectionData,
  SelectNamespaceScopeRequest,
  SecretMetadata,
  ServiceDetail,
  ServiceResource,
  SessionData,
  StatusData,
  ResourceListQuery,
  RestartActionRequest,
  ScaleActionRequest,
  ExecInit,
  Workload,
  WorkloadDetail,
} from './types'
import { desktopRequest } from './desktop'

export type * from './types'

export class APIError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId?: string
  readonly details?: unknown

  constructor(status: number, payload: APIErrorPayload, requestId?: string) {
    super(payload.message ?? 'The local API returned an unexpected response.')
    this.name = 'APIError'
    this.status = status
    this.code = payload.code ?? 'UNKNOWN'
    this.requestId = requestId ?? payload.requestId
    this.details = payload.details
  }
}

// requestIdFrom extracts the backend correlation id (O-04) so support and
// logs can be joined on the same identifier shown in the JSONL log file.
function requestIdFrom(response: ResponseLike): string | undefined {
  return response.headers.get('X-Request-ID') ?? undefined
}

async function decodeJSON(response: ResponseLike): Promise<unknown> {
  const contentType = response.headers.get('content-type') ?? ''
  if (!contentType.toLowerCase().startsWith('application/json')) {
    throw new APIError(response.status, { code: 'INVALID_RESPONSE' }, requestIdFrom(response))
  }
  return response.json()
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  return (await requestEnvelope<T>(path, init)).data
}

async function requestEnvelope<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
  const response = await transport(path, init)
  if (response.status === 204) {
    return { data: undefined as T }
  }
  const body = (await decodeJSON(response)) as Envelope<T> & APIErrorPayload
  if (!response.ok) {
    throw new APIError(response.status, body, requestIdFrom(response))
  }
  if (body && typeof body === 'object' && 'data' in body) {
    return body
  }
  // Keep the local client compatible with an already-unwrapped block while
  // the public contract continues to use the standard data/meta envelope.
  return { data: body as T }
}

interface ResponseLike {
  status: number
  ok: boolean
  headers: { get(name: string): string | null }
  json(): Promise<unknown>
  text(): Promise<string>
}

async function transport(path: string, init: RequestInit): Promise<ResponseLike> {
  const method = init.method ?? 'GET'
  if (method !== 'GET' && method !== 'POST' && method !== 'PUT' && method !== 'DELETE') {
    return fetch(path, {
      ...init,
      headers: { Accept: 'application/json', ...init.headers },
      cache: 'no-store',
      credentials: 'same-origin',
    })
  }
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (init.headers) {
    for (const [name, value] of Object.entries(init.headers as Record<string, string>)) {
      headers[name] = value
    }
  }
  const desktop = await desktopRequest(method, path, headers, init.body ? String(init.body) : undefined)
  if (desktop) return desktop
  return fetch(path, {
    ...init,
    headers: { Accept: 'application/json', ...init.headers },
    cache: 'no-store',
    credentials: 'same-origin',
  })
}

async function dashboardRequest<T>(path: string, init: RequestInit = {}, expectedGeneration?: string): Promise<DashboardResponse<T>> {
	const response = await requestEnvelope<DashboardBlock<T>>(path, init)
  const candidate = response.data
	if (!candidate || typeof candidate !== 'object' || !('value' in candidate)) {
		throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The dashboard returned an invalid response.' })
	}
	if (expectedGeneration && response.meta?.generation !== expectedGeneration) {
		throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The dashboard response belongs to another selection generation.' })
	}
  return {
    block: {
      ...candidate,
      complete: candidate.complete === true,
      truncated: candidate.truncated === true,
      coverage: candidate.coverage ?? null,
      errors: Array.isArray(candidate.errors) ? candidate.errors : [],
    },
    meta: response.meta,
  }
}

function queryString(entries: Array<[string, string | number | boolean | undefined]>): string {
  const query = new URLSearchParams()
  for (const [key, value] of entries) {
    if (value !== undefined) {
      query.append(key, String(value))
    }
  }
  const encoded = query.toString()
  return encoded === '' ? '' : `?${encoded}`
}

function mutation<T>(path: string, method: 'POST' | 'PUT' | 'DELETE', body: unknown, csrfToken: string, signal?: AbortSignal): Promise<T> {
  return request<T>(path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'X-KubePeep-CSRF': csrfToken,
    },
    body: JSON.stringify(body),
    signal,
  })
}

function resourceQuery(options: ResourceListQuery = {}): string {
  const entries: Array<[string, string | number | boolean | undefined]> = [
    ['limit', options.limit],
    ['continue', options.continueToken],
    ['search', options.search],
    ['sort', options.sort],
    ['order', options.order],
    ['workload', options.workload],
    ['node', options.node],
    ['restarts', options.restarts],
    ['problematic', options.problematic],
    ['objectKind', options.objectKind],
    ['reason', options.reason],
    ['addressType', options.addressType],
  ]
  for (const namespace of options.namespaces ?? []) entries.push(['namespace', namespace])
  for (const kind of options.kinds ?? []) entries.push(['kind', kind])
  for (const status of options.statuses ?? []) entries.push(['status', status])
  return queryString(entries)
}

async function collectionRequest<T>(path: string, options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<T>> {
  const response = await requestEnvelope<T[]>(`${path}${resourceQuery(options)}`, { method: 'GET', signal })
  if (!Array.isArray(response.data)) {
    throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The resource collection returned an invalid response.' })
  }
  if (expectedGeneration && response.meta?.generation !== expectedGeneration) {
    throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The resource response belongs to another selection generation.' })
  }
  return {
    items: response.data,
    page: response.meta?.page ?? {
      limit: options.limit ?? response.data.length,
      next: '',
      complete: false,
      truncated: true,
      filterScope: 'page',
    },
    coverage: response.meta?.coverage ?? null,
    generation: response.meta?.generation,
    collectedAt: response.meta?.collectedAt,
  }
}

async function resourceRequest<T>(path: string, signal?: AbortSignal, expectedGeneration?: string): Promise<T> {
  const response = await requestEnvelope<T>(path, { method: 'GET', signal })
  if (expectedGeneration && response.meta?.generation !== expectedGeneration) {
    throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The resource response belongs to another selection generation.' })
  }
  return response.data
}

async function requestYAML(path: string, signal?: AbortSignal): Promise<string> {
  const method = 'GET'
  const headers: Record<string, string> = { Accept: 'application/yaml, text/yaml' }
  const desktop = await desktopRequest(method, path, headers)
  if (desktop) {
    const contentType = desktop.headers.get('content-type')?.toLowerCase() ?? ''
    if (!contentType.includes('yaml')) {
      throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The YAML response used an unexpected content type.' })
    }
    return desktop.text()
  }
  const response = await fetch(path, {
    method,
    headers: { Accept: 'application/yaml, text/yaml' },
    cache: 'no-store',
    credentials: 'same-origin',
    signal,
  })
  if (!response.ok) {
    const payload = (await decodeJSON(response)) as APIErrorPayload
    throw new APIError(response.status, payload, requestIdFrom(response))
  }
  const contentType = response.headers.get('content-type')?.toLowerCase() ?? ''
  if (!contentType.includes('yaml')) {
    throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The YAML response used an unexpected content type.' })
  }
  return response.text()
}

function resourcePath(value: string): string {
  return encodeURIComponent(value)
}

export function createIdempotencyKey(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return `kp-${Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')}`
}

export async function getStatus(signal?: AbortSignal): Promise<StatusData> {
  return request<StatusData>('/api/v1/status', { method: 'GET', signal })
}

export function getSession(signal?: AbortSignal): Promise<SessionData> {
  return request<SessionData>('/api/v1/session', { method: 'GET', signal })
}

export function getClusterProfiles(signal?: AbortSignal): Promise<ClusterProfile[]> {
  return request<ClusterProfile[]>('/api/v1/cluster/profiles', { method: 'GET', signal })
}

export function getActiveClusterProfile(signal?: AbortSignal): Promise<ClusterProfile> {
  return request<ClusterProfile>('/api/v1/cluster/profile', { method: 'GET', signal })
}

export function getContexts(clusterProfileId: number, signal?: AbortSignal): Promise<KubernetesContext[]> {
  const query = queryString([['clusterProfileId', clusterProfileId]])
  return request<KubernetesContext[]>(`/api/v1/contexts${query}`, { method: 'GET', signal })
}

export function selectContext(body: SelectContextRequest, csrfToken: string, signal?: AbortSignal): Promise<SelectionData> {
  return mutation<SelectionData>('/api/v1/contexts/select', 'POST', body, csrfToken, signal)
}

export function getNamespaces(page: PageQuery = {}, signal?: AbortSignal): Promise<Namespace[]> {
  const query = queryString([
    ['limit', page.limit],
    ['continue', page.continueToken],
    ['search', page.search],
  ])
  return request<Namespace[]>(`/api/v1/namespaces${query}`, { method: 'GET', signal })
}

export function getNamespaceScopes(page: PageQuery = {}, signal?: AbortSignal): Promise<NamespaceScope[]> {
  const query = queryString([
    ['limit', page.limit],
    ['continue', page.continueToken],
    ['search', page.search],
  ])
  return request<NamespaceScope[]>(`/api/v1/namespace-scopes${query}`, { method: 'GET', signal })
}

export function getNamespaceScope(id: number, signal?: AbortSignal): Promise<NamespaceScope> {
  return request<NamespaceScope>(`/api/v1/namespace-scopes/${id}`, { method: 'GET', signal })
}

export function validateNamespaceScope(body: NamespaceScopeInput, csrfToken: string, signal?: AbortSignal): Promise<NamespaceScopeValidation> {
  return mutation<NamespaceScopeValidation>('/api/v1/namespace-scopes/validate', 'POST', body, csrfToken, signal)
}

export function createNamespaceScope(body: NamespaceScopeWriteRequest, csrfToken: string, signal?: AbortSignal): Promise<NamespaceScope> {
  return mutation<NamespaceScope>('/api/v1/namespace-scopes', 'POST', body, csrfToken, signal)
}

export function updateNamespaceScope(id: number, body: NamespaceScopeUpdateRequest, csrfToken: string, signal?: AbortSignal): Promise<NamespaceScope> {
  return mutation<NamespaceScope>(`/api/v1/namespace-scopes/${id}`, 'PUT', body, csrfToken, signal)
}

export function deleteNamespaceScope(id: number, body: NamespaceScopeDeleteRequest, csrfToken: string, signal?: AbortSignal): Promise<SelectionData | undefined> {
  return mutation<SelectionData | undefined>(`/api/v1/namespace-scopes/${id}`, 'DELETE', body, csrfToken, signal)
}

export function selectNamespaceScope(id: number, body: SelectNamespaceScopeRequest, csrfToken: string, signal?: AbortSignal): Promise<SelectionData> {
  return mutation<SelectionData>(`/api/v1/namespace-scopes/${id}/select`, 'POST', body, csrfToken, signal)
}

export async function getPermissions(options: PermissionQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CapabilityMatrix> {
  const entries: Array<[string, string | number | boolean | undefined]> = []
  for (const namespace of options.namespaces ?? []) {
    entries.push(['namespace', namespace])
  }
  for (const capability of options.capabilityIds ?? []) {
    entries.push(['capability', capability])
  }
  for (const resourceName of options.resourceNames ?? []) {
    entries.push(['resourceName', resourceName])
  }
  entries.push(['refresh', options.refresh])
  const matrix = await request<CapabilityMatrix>(`/api/v1/permissions${queryString(entries)}`, { method: 'GET', signal })
  if (expectedGeneration && matrix.generation !== expectedGeneration) {
    throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The permission response belongs to another selection generation.' })
  }
  return matrix
}

export function getDashboardSummary(signal?: AbortSignal, expectedGeneration?: string): Promise<DashboardResponse<DashboardSummary>> {
	return dashboardRequest<DashboardSummary>('/api/v1/dashboard/summary', { method: 'GET', signal }, expectedGeneration)
}

export function getDashboardProblems(signal?: AbortSignal, expectedGeneration?: string): Promise<DashboardResponse<DashboardProblem[]>> {
	return dashboardRequest<DashboardProblem[]>('/api/v1/dashboard/problems', { method: 'GET', signal }, expectedGeneration)
}

export function getDashboardRestarts(limit = 10, signal?: AbortSignal, expectedGeneration?: string): Promise<DashboardResponse<DashboardRestart[]>> {
	return dashboardRequest<DashboardRestart[]>(`/api/v1/dashboard/restarts${queryString([['limit', limit]])}`, { method: 'GET', signal }, expectedGeneration)
}

export function getDashboardEvents(signal?: AbortSignal, expectedGeneration?: string): Promise<DashboardResponse<DashboardEvent[]>> {
	return dashboardRequest<DashboardEvent[]>('/api/v1/dashboard/events', { method: 'GET', signal }, expectedGeneration)
}

export function getDashboardMetrics(signal?: AbortSignal, expectedGeneration?: string): Promise<DashboardResponse<DashboardMetrics>> {
	return dashboardRequest<DashboardMetrics>('/api/v1/metrics', { method: 'GET', signal }, expectedGeneration)
}

export function getDashboardNamespaceHealth(signal?: AbortSignal, expectedGeneration?: string): Promise<DashboardResponse<DashboardNamespaceHealth[]>> {
	return dashboardRequest<DashboardNamespaceHealth[]>('/api/v1/dashboard/namespace-health', { method: 'GET', signal }, expectedGeneration)
}

export function getYAMLDiff(collection: string, namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<YAMLDiff> {
	return requestEnvelope<YAMLDiff>(`/api/v1/resources/${encodeURIComponent(collection)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/yaml-diff`, { method: 'GET', signal }).then((response) => {
		if (expectedGeneration && response.meta?.generation && response.meta.generation !== expectedGeneration) {
			throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The YAML diff belongs to another selection generation.' })
		}
		return response.data
	})
}

export function scanDashboardLogs(body: LogScanRequest, csrfToken: string, signal?: AbortSignal, expectedGeneration?: string): Promise<DashboardResponse<DashboardLogMatch[]>> {
	return dashboardRequest<DashboardLogMatch[]>('/api/v1/dashboard/log-scan', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-KubePeep-CSRF': csrfToken,
    },
    body: JSON.stringify(body),
    signal,
	}, expectedGeneration)
}

export function getWorkloads(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<Workload>> {
  return collectionRequest<Workload>('/api/v1/workloads', options, signal, expectedGeneration)
}

export function getWorkload(kind: string, namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<WorkloadDetail> {
  return resourceRequest<WorkloadDetail>(`/api/v1/workloads/${resourcePath(kind)}/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getWorkloadYAML(kind: string, namespace: string, name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/workloads/${resourcePath(kind)}/${resourcePath(namespace)}/${resourcePath(name)}/yaml`, signal)
}

export function getPods(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<Pod>> {
  return collectionRequest<Pod>('/api/v1/pods', options, signal, expectedGeneration)
}

export function getPod(namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<PodDetail> {
  return resourceRequest<PodDetail>(`/api/v1/pods/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getPodYAML(namespace: string, name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/pods/${resourcePath(namespace)}/${resourcePath(name)}/yaml`, signal)
}

export function getPodLogs(namespace: string, name: string, options: LogQuery, signal?: AbortSignal, expectedGeneration?: string): Promise<LogRead> {
  const query = queryString([
    ['container', options.container],
    ['previous', options.previous],
    ['timestamps', options.timestamps],
    ['tailLines', options.tailLines],
    ['since', options.since],
  ])
  return resourceRequest<LogRead>(`/api/v1/pods/${resourcePath(namespace)}/${resourcePath(name)}/logs${query}`, signal, expectedGeneration)
}

export function getEvents(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<EventResource>> {
  return collectionRequest<EventResource>('/api/v1/events', options, signal, expectedGeneration)
}

export function getServices(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<ServiceResource>> {
  return collectionRequest<ServiceResource>('/api/v1/services', options, signal, expectedGeneration)
}

export function getService(namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<ServiceDetail> {
  return resourceRequest<ServiceDetail>(`/api/v1/services/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getServiceYAML(namespace: string, name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/services/${resourcePath(namespace)}/${resourcePath(name)}/yaml`, signal)
}

export function getIngresses(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<IngressResource>> {
  return collectionRequest<IngressResource>('/api/v1/ingresses', options, signal, expectedGeneration)
}

export function getIngress(namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<IngressDetail> {
  return resourceRequest<IngressDetail>(`/api/v1/ingresses/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getIngressYAML(namespace: string, name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/ingresses/${resourcePath(namespace)}/${resourcePath(name)}/yaml`, signal)
}

export function getEndpointSlices(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<EndpointSliceResource>> {
  return collectionRequest<EndpointSliceResource>('/api/v1/endpoint-slices', options, signal, expectedGeneration)
}

export function getEndpointSlice(namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<EndpointSliceDetail> {
  return resourceRequest<EndpointSliceDetail>(`/api/v1/endpoint-slices/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getEndpointSliceYAML(namespace: string, name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/endpoint-slices/${resourcePath(namespace)}/${resourcePath(name)}/yaml`, signal)
}

export function getConfigMaps(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<ConfigMapResource>> {
  return collectionRequest<ConfigMapResource>('/api/v1/configmaps', options, signal, expectedGeneration)
}

export function getConfigMap(namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<ConfigMapDetail> {
  return resourceRequest<ConfigMapDetail>(`/api/v1/configmaps/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getConfigMapYAML(namespace: string, name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/configmaps/${resourcePath(namespace)}/${resourcePath(name)}/yaml`, signal)
}

export function getSecrets(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<SecretMetadata>> {
  return collectionRequest<SecretMetadata>('/api/v1/secrets', options, signal, expectedGeneration)
}

export function getSecret(namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<SecretMetadata> {
  return resourceRequest<SecretMetadata>(`/api/v1/secrets/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getNodes(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<NodeSummary>> {
  return collectionRequest<NodeSummary>('/api/v1/nodes', options, signal, expectedGeneration)
}

export function getNode(name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<NodeDetail> {
  return resourceRequest<NodeDetail>(`/api/v1/nodes/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getNodeYAML(name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/nodes/${resourcePath(name)}/yaml`, signal)
}

export function getLeases(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<Lease>> {
  return collectionRequest<Lease>('/api/v1/leases', options, signal, expectedGeneration)
}

export function getLease(namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<LeaseDetail> {
  return resourceRequest<LeaseDetail>(`/api/v1/leases/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getLeaseYAML(namespace: string, name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/leases/${resourcePath(namespace)}/${resourcePath(name)}/yaml`, signal)
}

export function getPersistentVolumes(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<PersistentVolume>> {
  return collectionRequest<PersistentVolume>('/api/v1/persistent-volumes', options, signal, expectedGeneration)
}

export function getPersistentVolume(name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<PersistentVolumeDetail> {
  return resourceRequest<PersistentVolumeDetail>(`/api/v1/persistent-volumes/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getPersistentVolumeYAML(name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/persistent-volumes/${resourcePath(name)}/yaml`, signal)
}

export function getPersistentVolumeClaims(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<PersistentVolumeClaim>> {
  return collectionRequest<PersistentVolumeClaim>('/api/v1/persistent-volume-claims', options, signal, expectedGeneration)
}

export function getPersistentVolumeClaim(namespace: string, name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<PersistentVolumeClaimDetail> {
  return resourceRequest<PersistentVolumeClaimDetail>(`/api/v1/persistent-volume-claims/${resourcePath(namespace)}/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getPersistentVolumeClaimYAML(namespace: string, name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/persistent-volume-claims/${resourcePath(namespace)}/${resourcePath(name)}/yaml`, signal)
}

export function getStorageClasses(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<StorageClass>> {
  return collectionRequest<StorageClass>('/api/v1/storage-classes', options, signal, expectedGeneration)
}

export function getStorageClass(name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<StorageClassDetail> {
  return resourceRequest<StorageClassDetail>(`/api/v1/storage-classes/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getStorageClassYAML(name: string, signal?: AbortSignal): Promise<string> {
  return requestYAML(`/api/v1/storage-classes/${resourcePath(name)}/yaml`, signal)
}

export function getCSIDrivers(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<CSIDriver>> {
  return collectionRequest<CSIDriver>('/api/v1/csi-drivers', options, signal, expectedGeneration)
}

export function getCSIDriver(name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<CSIDriverDetail> {
  return resourceRequest<CSIDriverDetail>(`/api/v1/csi-drivers/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getCSINodes(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<CSINode>> {
  return collectionRequest<CSINode>('/api/v1/csi-nodes', options, signal, expectedGeneration)
}

export function getCSINode(name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<CSINodeDetail> {
  return resourceRequest<CSINodeDetail>(`/api/v1/csi-nodes/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getVolumeAttachments(options: ResourceListQuery = {}, signal?: AbortSignal, expectedGeneration?: string): Promise<CollectionResult<VolumeAttachment>> {
  return collectionRequest<VolumeAttachment>('/api/v1/volume-attachments', options, signal, expectedGeneration)
}

export function getVolumeAttachment(name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<VolumeAttachmentDetail> {
  return resourceRequest<VolumeAttachmentDetail>(`/api/v1/volume-attachments/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getNamespaceObject(name: string, signal?: AbortSignal, expectedGeneration?: string): Promise<NamespaceObjectDetail> {
  return resourceRequest<NamespaceObjectDetail>(`/api/v1/namespaces/${resourcePath(name)}`, signal, expectedGeneration)
}

export function getPreferences(signal?: AbortSignal): Promise<Preferences> {
  return request<Preferences>('/api/v1/preferences', { method: 'GET', signal })
}

export function putPreferences(value: Preferences, csrfToken: string, signal?: AbortSignal): Promise<Preferences> {
  return mutation<Preferences>('/api/v1/preferences', 'PUT', value, csrfToken, signal)
}

export function restartWorkload(kind: string, namespace: string, name: string, body: RestartActionRequest, csrfToken: string, idempotencyKey: string, signal?: AbortSignal): Promise<import('./types').ActionAccepted> {
  return request<import('./types').ActionAccepted>(`/api/v1/workloads/${resourcePath(kind)}/${resourcePath(namespace)}/${resourcePath(name)}/restart`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-KubePeep-CSRF': csrfToken, 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify(body),
    signal,
  })
}

export function scaleWorkload(kind: string, namespace: string, name: string, body: ScaleActionRequest, csrfToken: string, signal?: AbortSignal): Promise<import('./types').ActionAccepted> {
  return mutation<import('./types').ActionAccepted>(`/api/v1/workloads/${resourcePath(kind)}/${resourcePath(namespace)}/${resourcePath(name)}/scale`, 'PUT', body, csrfToken, signal)
}

export function deletePod(namespace: string, name: string, body: PodDeleteActionRequest, csrfToken: string, signal?: AbortSignal): Promise<import('./types').ActionAccepted> {
  return mutation<import('./types').ActionAccepted>(`/api/v1/pods/${resourcePath(namespace)}/${resourcePath(name)}`, 'DELETE', body, csrfToken, signal)
}

export function createPortForward(namespace: string, name: string, body: PortForwardCreateRequest, csrfToken: string, idempotencyKey: string, signal?: AbortSignal): Promise<PortForward> {
  return request<PortForward>(`/api/v1/pods/${resourcePath(namespace)}/${resourcePath(name)}/port-forward`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-KubePeep-CSRF': csrfToken, 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify(body),
    signal,
  })
}

export async function getPortForwards(signal?: AbortSignal, expectedGeneration?: string): Promise<PortForward[]> {
  const sessions = await request<PortForward[]>('/api/v1/port-forwards', { method: 'GET', signal })
  if (!Array.isArray(sessions) || sessions.some((session) => session.localAddress !== '127.0.0.1')) {
    throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The port-forward collection contains a non-loopback or invalid session.' })
  }
  if (expectedGeneration && sessions.some((session) => session.generation !== expectedGeneration)) {
    throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The port-forward collection belongs to another selection generation.' })
  }
  return sessions
}

export function closePortForward(id: string, expectedGeneration: string, csrfToken: string, signal?: AbortSignal): Promise<void> {
  return mutation<void>(`/api/v1/port-forwards/${resourcePath(id)}`, 'DELETE', { confirmed: true, expectedGeneration }, csrfToken, signal)
}

export function createExecTicket(namespace: string, name: string, body: ExecInit, csrfToken: string, signal?: AbortSignal): Promise<ExecTicket> {
  return mutation<ExecTicket>(`/api/v1/pods/${resourcePath(namespace)}/${resourcePath(name)}/exec`, 'POST', body, csrfToken, signal)
}
