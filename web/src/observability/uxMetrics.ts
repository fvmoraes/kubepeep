export type UXMetricName =
  | 'time_to_first_row'
  | 'time_to_page_complete'
  | 'filter_interaction_latency'
  | 'sort_interaction_latency'
  | 'rendered_row_count'

export interface UXMetricSample {
  name: UXMetricName
  view: string
  value: number
  unit: 'milliseconds' | 'rows'
  recordedAt: number
}

export interface UXMetricsInspector {
  snapshot: () => UXMetricSample[]
  reset: () => void
}

export type ListRequestID = string
export type ListInteractionID = string

export interface ListRequestTimingOptions {
  view?: string
  interactionId?: ListInteractionID
}

declare global {
  interface Window {
    __KUBEPEEP_UX_METRICS__?: UXMetricsInspector
  }
}

interface RequestTiming {
  view: string
  startedAt: number
  interactionId?: ListInteractionID
  firstRowRecorded: boolean
  pageCompleteRecorded: boolean
}

interface InteractionTiming {
  view: string
  kind: 'filter' | 'sort'
  startedAt: number
}

const maximumSamples = 256
const maximumPendingRequests = 256
const maximumPendingInteractions = 256
const allowedViews = new Set([
  'access',
  'administration',
  'config',
  'configuration',
  'events',
  'leases',
  'network',
  'nodes',
  'overview',
  'pods',
  'service-accounts',
  'storage',
  'workloads',
])

const samples: UXMetricSample[] = []
const requestTimings = new Map<ListRequestID, RequestTiming>()
const interactionTimings = new Map<ListInteractionID, InteractionTiming>()
let rowsToRequest = new WeakMap<readonly unknown[], ListRequestID>()
let queryToInteraction = new WeakMap<object, ListInteractionID>()
let requestSequence = 0
let interactionSequence = 0

function now(): number {
  return typeof performance !== 'undefined' && typeof performance.now === 'function' ? performance.now() : Date.now()
}

export function currentUXView(pathname = typeof window === 'undefined' ? '' : window.location.pathname): string {
  const segment = pathname.split('/').filter(Boolean)[0] ?? 'overview'
  return allowedViews.has(segment) ? segment : segment === '' ? 'overview' : 'other'
}

function normalizedView(view: string): string {
  return allowedViews.has(view) ? view : view === 'overview' ? view : 'other'
}

function record(name: UXMetricName, value: number, unit: UXMetricSample['unit'], view: string): void {
  if (!Number.isFinite(value) || value < 0) return
  samples.push({ name, view: normalizedView(view), value, unit, recordedAt: Date.now() })
  if (samples.length > maximumSamples) samples.splice(0, samples.length - maximumSamples)
}

// The submit token is attached out-of-band to the exact applied-query state
// object. WeakMap storage keeps it out of query keys, URLs, samples and logs.
export function bindListInteraction<T extends object>(queryState: T, interactionId: ListInteractionID): T {
  queryToInteraction.set(queryState, interactionId)
  return queryState
}

export function listInteractionFor(queryState: object): ListInteractionID | undefined {
  return queryToInteraction.get(queryState)
}

// Every transport attempt gets a unique internal identity. An interaction is
// accepted only when the exact query that received its token starts a request;
// unrelated requests in the same view cannot capture it.
export function beginListRequest(options: ListRequestTimingOptions = {}): ListRequestID {
  const view = normalizedView(options.view ?? currentUXView())
  const candidate = options.interactionId ? interactionTimings.get(options.interactionId) : undefined
  const interactionId = candidate?.view === view ? options.interactionId : undefined
  const requestId = `list-${++requestSequence}`
  requestTimings.set(requestId, {
    view,
    startedAt: now(),
    interactionId,
    firstRowRecorded: false,
    pageCompleteRecorded: false,
  })

  if (requestTimings.size > maximumPendingRequests) {
    const oldest = requestTimings.keys().next().value as ListRequestID | undefined
    if (oldest) cancelListRequest(oldest)
  }
  return requestId
}

export function beginListInteraction(kind: 'filter' | 'sort', view = currentUXView()): ListInteractionID {
  const interactionId = `interaction-${++interactionSequence}`
  interactionTimings.set(interactionId, { view: normalizedView(view), kind, startedAt: now() })
  if (interactionTimings.size > maximumPendingInteractions) {
    const oldest = interactionTimings.keys().next().value as ListInteractionID | undefined
    if (oldest) interactionTimings.delete(oldest)
  }
  return interactionId
}

export function associateListRequestRows(requestId: ListRequestID, rows: readonly unknown[]): void {
  if (requestTimings.has(requestId)) rowsToRequest.set(rows, requestId)
}

export function cancelListRequest(requestId: ListRequestID): void {
  requestTimings.delete(requestId)
}

export function completeListRequest(requestId: ListRequestID, hasRows: boolean): void {
  const completedAt = now()
  const timing = requestTimings.get(requestId)
  if (!timing || timing.pageCompleteRecorded) return

  record('time_to_page_complete', completedAt - timing.startedAt, 'milliseconds', timing.view)
  timing.pageCompleteRecorded = true

  if (timing.interactionId) {
    const interaction = interactionTimings.get(timing.interactionId)
    if (interaction) {
      record(interaction.kind === 'sort' ? 'sort_interaction_latency' : 'filter_interaction_latency', completedAt - interaction.startedAt, 'milliseconds', timing.view)
      interactionTimings.delete(timing.interactionId)
    }
  }

  if (!hasRows || timing.firstRowRecorded) requestTimings.delete(requestId)
}

// DataTable calls this from a passive effect after committing its rows. The
// WeakMap associates that exact result array with the transport attempt that
// produced it; cached arrays do not create duplicate samples and same-sized
// refetches still carry a distinct identity.
export function recordFirstRowRendered(rows: readonly unknown[]): void {
  if (rows.length === 0) return
  const requestId = rowsToRequest.get(rows)
  if (!requestId) return
  const timing = requestTimings.get(requestId)
  if (!timing || timing.firstRowRecorded) return
  record('time_to_first_row', now() - timing.startedAt, 'milliseconds', timing.view)
  timing.firstRowRecorded = true
  if (timing.pageCompleteRecorded) requestTimings.delete(requestId)
}

export function recordRenderedRowCount(rows: number, view = currentUXView()): void {
  record('rendered_row_count', rows, 'rows', view)
}

export function snapshotUXMetrics(): UXMetricSample[] {
  return samples.map((sample) => ({ ...sample }))
}

export function resetUXMetrics(): void {
  samples.length = 0
  requestTimings.clear()
  interactionTimings.clear()
  rowsToRequest = new WeakMap<readonly unknown[], ListRequestID>()
  queryToInteraction = new WeakMap<object, ListInteractionID>()
  requestSequence = 0
  interactionSequence = 0
}

if (typeof window !== 'undefined') {
  window.__KUBEPEEP_UX_METRICS__ = { snapshot: snapshotUXMetrics, reset: resetUXMetrics }
}
