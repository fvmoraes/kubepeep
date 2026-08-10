import type {
  APIErrorPayload,
  CapabilityMatrix,
  ClusterProfile,
  DashboardBlock,
  DashboardEvent,
  DashboardLogMatch,
  DashboardMetrics,
  DashboardProblem,
  DashboardResponse,
  DashboardRestart,
  DashboardSummary,
  Envelope,
  KubernetesContext,
  LogScanRequest,
  Namespace,
  NamespaceScope,
  NamespaceScopeDeleteRequest,
  NamespaceScopeInput,
  NamespaceScopeUpdateRequest,
  NamespaceScopeValidation,
  NamespaceScopeWriteRequest,
  PageQuery,
  PermissionQuery,
  SelectContextRequest,
  SelectionData,
  SelectNamespaceScopeRequest,
  SessionData,
  StatusData,
} from './types'

export type * from './types'

export class APIError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId?: string
  readonly details?: unknown

  constructor(status: number, payload: APIErrorPayload) {
    super(payload.message ?? 'The local API returned an unexpected response.')
    this.name = 'APIError'
    this.status = status
    this.code = payload.code ?? 'UNKNOWN'
    this.requestId = payload.requestId
    this.details = payload.details
  }
}

async function decodeJSON(response: Response): Promise<unknown> {
  const contentType = response.headers.get('content-type') ?? ''
  if (!contentType.toLowerCase().startsWith('application/json')) {
    throw new APIError(response.status, { code: 'INVALID_RESPONSE' })
  }
  return response.json()
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  return (await requestEnvelope<T>(path, init)).data
}

async function requestEnvelope<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
  const response = await fetch(path, {
    ...init,
    headers: {
      Accept: 'application/json',
      ...init.headers,
    },
    cache: 'no-store',
    credentials: 'same-origin',
  })
  if (response.status === 204) {
    return { data: undefined as T }
  }
  const body = (await decodeJSON(response)) as Envelope<T> & APIErrorPayload
  if (!response.ok) {
    throw new APIError(response.status, body)
  }
  if (body && typeof body === 'object' && 'data' in body) {
    return body
  }
  // Keep the local client compatible with an already-unwrapped block while
  // the public contract continues to use the standard data/meta envelope.
  return { data: body as T }
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

export function getPermissions(options: PermissionQuery = {}, signal?: AbortSignal): Promise<CapabilityMatrix> {
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
  return request<CapabilityMatrix>(`/api/v1/permissions${queryString(entries)}`, { method: 'GET', signal })
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
