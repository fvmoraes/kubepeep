export interface ComponentState {
  status: 'healthy' | 'degraded' | 'unhealthy' | 'unknown'
  code: string
  message: string
  checkedAt: string | null
}

export interface SelectionSummary {
  clusterProfileId: number
  context: string
  cluster: string
  scopeId: number | null
  scopeName: string | null
  scopeMode: 'single' | 'list' | 'all' | null
  scopeSource: 'saved' | 'cli' | 'none'
  defaultNamespace: string | null
  namespaceCount: number
  generation: string
}

export interface StatusData {
  version: string
  commit: string
  buildDate: string
  port: number
  components: Record<'application' | 'sqlite' | 'kubeconfig' | 'context' | 'cluster' | 'metrics', ComponentState>
  selection: SelectionSummary | null
}

interface Envelope<T> {
  data: T
  meta?: {
    requestId?: string
    generation?: string
    collectedAt?: string
  }
}

interface APIErrorPayload {
  code?: string
  message?: string
  requestId?: string
}

export class APIError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId?: string

  constructor(status: number, payload: APIErrorPayload) {
    super(payload.message ?? 'The local API returned an unexpected response.')
    this.name = 'APIError'
    this.status = status
    this.code = payload.code ?? 'UNKNOWN'
    this.requestId = payload.requestId
  }
}

async function decodeJSON(response: Response): Promise<unknown> {
  const contentType = response.headers.get('content-type') ?? ''
  if (!contentType.toLowerCase().startsWith('application/json')) {
    throw new APIError(response.status, { code: 'INVALID_RESPONSE' })
  }
  return response.json()
}

export async function getStatus(signal?: AbortSignal): Promise<StatusData> {
  const response = await fetch('/api/v1/status', {
    method: 'GET',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
    credentials: 'same-origin',
    signal,
  })
  const body = (await decodeJSON(response)) as Envelope<StatusData> & APIErrorPayload
  if (!response.ok) {
    throw new APIError(response.status, body)
  }
  return body.data
}
