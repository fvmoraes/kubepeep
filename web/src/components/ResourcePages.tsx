import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'

import {
  APIError,
  closePortForward,
  getConfigMap,
  getConfigMaps,
  getConfigMapYAML,
  getEndpointSlice,
  getEndpointSlices,
  getEndpointSliceYAML,
  getEvents,
  getIngress,
  getIngresses,
  getIngressYAML,
  getPod,
  getPods,
  getPodYAML,
  getPortForwards,
  getSecret,
  getSecrets,
  getService,
  getServices,
  getServiceYAML,
  getSession,
  getStatus,
  getWorkload,
  getWorkloads,
  getWorkloadYAML,
} from '../api/client'
import type {
  CollectionResult,
  ConfigMapDetail,
  ConfigMapResource,
  EndpointSliceDetail,
  EndpointSliceResource,
  IngressDetail,
  IngressResource,
  Pod,
  SecretMetadata,
  SelectionSummary,
  ServiceDetail,
  ServiceResource,
  Workload,
} from '../api/types'
import { Badge, Button, DataTable, Drawer, Input, Select } from '../components/ui'
import type { BadgeVariant } from '../components/ui'
import { PodActions, WorkloadActions } from './ResourceActions'
import { ResourceListControls } from './ResourceListControls'
import type { ActiveListFilter, ListSortOrder, ListSortOption } from './ResourceListControls'
import { ResourceLiveUpdates } from './ResourceLiveUpdates'
import { SavedFilterControls } from './SavedFilterControls'
import { StatePanel } from './StatePanel'
import { YamlViewer } from './YamlViewer'

function errorMessage(error: unknown): string {
  if (error instanceof APIError) return `${error.code}: ${error.message}`
  return error instanceof Error ? error.message : 'The local API could not load this resource.'
}

function age(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3_600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86_400) return `${Math.floor(seconds / 3_600)}h`
  return `${Math.floor(seconds / 86_400)}d`
}

function dateTime(value: string | null | undefined): string {
  return value ? new Date(value).toLocaleString() : '—'
}

function PageHeading({ title, description, action }: { title: string; description: string; action?: ReactNode }) {
  return (
    <header className="resource-header">
      <div><span className="eyebrow">cluster resources</span><h1>{title}</h1><p>{description}</p></div>
      {action}
    </header>
  )
}

function CollectionFooter<T>({ result, onNext, onRestart }: { result: CollectionResult<T>; onNext: (cursor: string) => void; onRestart: () => void }) {
  const coverage = result.coverage
  return (
    <footer className="collection-footer">
      <div>
        <span>{result.items.length} item{result.items.length === 1 ? '' : 's'} in this page</span>
        <small>{result.page.complete ? 'Collection complete' : `Bounded ${result.page.filterScope} result`}{result.page.truncated ? ' · truncated' : ''}</small>
        {coverage ? <small>{coverage.completedNamespaces}/{coverage.requestedNamespaces} namespaces completed · {coverage.deniedNamespaces.length} denied · {coverage.failed.length} failed</small> : null}
      </div>
      <div>
        <Button variant="secondary" size="compact" onClick={onRestart}>First page</Button>
        <Button size="compact" disabled={!result.page.next} onClick={() => onNext(result.page.next)}>Next page</Button>
      </div>
    </footer>
  )
}

function EmptySelection() {
  return <StatePanel kind="empty" title="Choose a Kubernetes context">Select a context and namespace scope before querying cluster resources.</StatePanel>
}

function SelectionGate({ pending, error, selected, children }: { pending: boolean; error: unknown; selected: boolean; children: ReactNode }) {
  if (pending) return <StatePanel kind="loading" title="Loading active selection">The local service is resolving the current generation.</StatePanel>
  if (error) return <StatePanel kind="error" title="Selection unavailable">{errorMessage(error)}</StatePanel>
  if (!selected) return <EmptySelection />
  return children
}

function QueryState({ pending, error, empty, children }: { pending: boolean; error: unknown; empty: boolean; children: ReactNode }) {
  if (pending) return <StatePanel kind="loading" title="Loading resources">The request is bounded and tied to the active selection generation.</StatePanel>
  if (error) return <StatePanel kind="error" title="Resource request failed">{errorMessage(error)}</StatePanel>
  if (empty) return <StatePanel kind="empty" title="No matching resources">The authorized page returned no items for these filters.</StatePanel>
  return children
}

function detailTitle(title: string) {
  return <h2 className="text-xl text-kp-text">{title}</h2>
}

function useActiveSelection() {
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  return { status, selection: status.data?.selection ?? null }
}

function useGenerationRequests(generation: string | undefined) {
  const active = useRef(new Set<AbortController>())
  const abortAll = useCallback(() => {
    for (const controller of active.current) controller.abort()
    active.current.clear()
  }, [])
  useEffect(() => abortAll, [abortAll, generation])
  const run = useCallback(function run<T>(operation: (signal: AbortSignal) => Promise<T>): Promise<T> {
    const controller = new AbortController()
    active.current.add(controller)
    return operation(controller.signal).finally(() => active.current.delete(controller))
  }, [])
  return { run, abortAll }
}

function useGenerationCursor(generation: string | undefined): [string, (value: string) => void] {
  const [state, setState] = useState<{ generation: string | undefined; value: string }>({ generation, value: '' })
  const value = state.generation === generation ? state.value : ''
  const setValue = useCallback((next: string) => setState({ generation, value: next }), [generation])
  return [value, setValue]
}

function useGenerationCursorMap<K extends string>(generation: string | undefined, empty: Record<K, string>): [Record<K, string>, (key: K, value: string) => void] {
  const [state, setState] = useState<{ generation: string | undefined; values: Record<K, string> }>(() => ({ generation, values: { ...empty } }))
  const values = state.generation === generation ? state.values : empty
  const setValue = useCallback((key: K, value: string) => setState((current) => ({
    generation,
    values: { ...(current.generation === generation ? current.values : empty), [key]: value },
  })), [empty, generation])
  return [values, setValue]
}

function workloadKindPath(kind: Workload['kind']): string {
  return ({ Deployment: 'deployments', StatefulSet: 'statefulsets', DaemonSet: 'daemonsets', Job: 'jobs', CronJob: 'cronjobs' } as const)[kind]
}

function workloadFromParams(kind: string, namespace: string, name: string): Workload | null {
  const kindMap: Record<string, Workload['kind']> = {
    deployments: 'Deployment',
    statefulsets: 'StatefulSet',
    daemonsets: 'DaemonSet',
    jobs: 'Job',
    cronjobs: 'CronJob',
  }
  const mapped = kindMap[kind]
  if (!mapped) return null
  return { namespace, kind: mapped, name, ready: null, desired: null, available: null, updated: null, status: 'Unknown', ageSeconds: 0 }
}

function podFromParams(namespace: string, name: string): Pod {
  return { namespace, name, status: 'Unknown', ready: { current: 0, desired: 0 }, restarts: 0, node: null, ip: null, owner: null, ageSeconds: 0, problematic: false }
}

interface GenerationSelection<T> {
  generation: string
  item: T
}

function compactFilterQuery(entries: Array<[string, unknown]>): Record<string, unknown> {
  const query: Record<string, unknown> = {}
  for (const [key, value] of entries) {
    if (value === undefined || value === '' || Array.isArray(value) && value.length === 0) continue
    query[key] = value
  }
  return query
}

function savedString(query: Record<string, unknown>, key: string, allowed?: readonly string[]): string {
  const value = query[key]
  if (typeof value !== 'string' || allowed && !allowed.includes(value)) return ''
  return value
}

function savedFirst(query: Record<string, unknown>, key: string, allowed?: readonly string[]): string {
  const values = query[key]
  if (!Array.isArray(values) || typeof values[0] !== 'string' || allowed && !allowed.includes(values[0])) return ''
  return values[0]
}

function namespaceValues(value: string): string[] {
  return [...new Set(value.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean))].slice(0, 50)
}

function savedNamespaces(query: Record<string, unknown>): string {
  const values = query.namespace
  return Array.isArray(values) ? values.filter((value): value is string => typeof value === 'string').join(', ') : ''
}

function savedSort(query: Record<string, unknown>, allowed: readonly string[], fallback: string): string {
  return savedString(query, 'sort', allowed) || fallback
}

function savedOrder(query: Record<string, unknown>, fallback: ListSortOrder): ListSortOrder {
  const value = savedString(query, 'order', ['asc', 'desc'])
  return value === 'asc' || value === 'desc' ? value : fallback
}

function optionalSort(sort: string, order: ListSortOrder, defaultSort: string, defaultOrder: ListSortOrder): { sort?: string; order?: ListSortOrder } {
  return sort === defaultSort && order === defaultOrder ? {} : { sort, order }
}

function activeFilter(id: string, label: string, value: string | string[]): ActiveListFilter[] {
  const display = Array.isArray(value) ? value.join(', ') : value
  return display === '' ? [] : [{ id, label, value: display }]
}

function statusBadgeVariant(status: string): BadgeVariant {
  switch (status.toLowerCase()) {
    case 'healthy':
    case 'completed':
    case 'running':
    case 'succeeded':
      return 'healthy'
    case 'progressing':
    case 'suspended':
    case 'pending':
      return 'warning'
    case 'degraded':
    case 'failed':
      return 'danger'
    default:
      return 'unknown'
  }
}

function eventBadgeVariant(type: string): BadgeVariant {
  switch (type.toLowerCase()) {
    case 'normal':
      return 'healthy'
    case 'warning':
      return 'warning'
    default:
      return 'unknown'
  }
}

interface TableLinkProps {
  'aria-label': string
  onClick: () => void
  primary: ReactNode
  secondary: ReactNode
}

function TableLink({ 'aria-label': label, onClick, primary, secondary }: TableLinkProps) {
  return (
    <Button type="button" variant="ghost" size="compact" className="h-auto p-0 justify-start text-left text-kp-sky hover:not-disabled:bg-transparent hover:not-disabled:text-kp-sky" aria-label={label} onClick={onClick}>
      <strong className="block hover:underline">{primary}</strong>
      <small className="block text-kp-overlay-text">{secondary}</small>
    </Button>
  )
}

const workloadKinds = ['deployments', 'statefulsets', 'daemonsets', 'jobs', 'cronjobs'] as const
const workloadStatuses = ['Healthy', 'Progressing', 'Degraded', 'Suspended', 'Completed', 'Failed', 'Unknown'] as const
const podStatuses = ['Running', 'Pending', 'Succeeded', 'Failed', 'Unknown'] as const
const restartFilters = ['any', 'gt0', 'gte3', 'gte10'] as const
const eventTypes = ['Normal', 'Warning', 'Unknown'] as const
const workloadSorts = ['identity', 'name', 'age', 'status'] as const
const podSorts = ['identity', 'name', 'age', 'restarts', 'status'] as const
const eventSorts = ['timestamp', 'count', 'identity'] as const
const workloadSortOptions: readonly ListSortOption[] = [
  { value: 'identity', label: 'Namespace, kind and name' },
  { value: 'name', label: 'Name' },
  { value: 'age', label: 'Age' },
  { value: 'status', label: 'Status' },
]
const podSortOptions: readonly ListSortOption[] = [
  { value: 'identity', label: 'Namespace and name' },
  { value: 'name', label: 'Name' },
  { value: 'age', label: 'Age' },
  { value: 'restarts', label: 'Restarts' },
  { value: 'status', label: 'Status' },
]
const eventSortOptions: readonly ListSortOption[] = [
  { value: 'timestamp', label: 'Timestamp' },
  { value: 'count', label: 'Count' },
  { value: 'identity', label: 'Object identity' },
]

interface WorkloadListState {
  search: string
  namespace: string
  kind: string
  workloadStatus: string
  sort: string
  order: ListSortOrder
}

interface PodListState {
  search: string
  namespace: string
  podStatus: string
  workload: string
  node: string
  restarts: string
  problematic: string
  sort: string
  order: ListSortOrder
}

interface EventListState {
  search: string
  namespace: string
  eventType: string
  objectKind: string
  reason: string
  sort: string
  order: ListSortOrder
}

const defaultWorkloadList: WorkloadListState = { search: '', namespace: '', kind: '', workloadStatus: '', sort: 'identity', order: 'asc' }
const defaultPodList: PodListState = { search: '', namespace: '', podStatus: '', workload: '', node: '', restarts: 'any', problematic: '', sort: 'identity', order: 'asc' }
const defaultEventList: EventListState = { search: '', namespace: '', eventType: '', objectKind: '', reason: '', sort: 'timestamp', order: 'desc' }

function paramValue(params: URLSearchParams, key: string): string {
  return params.get(key) ?? ''
}

function listedValue(value: string, allowed: readonly string[]): string {
  return (allowed as readonly string[]).includes(value) ? value : ''
}

function workloadsStateFromParams(params: URLSearchParams): WorkloadListState {
  return {
    ...defaultWorkloadList,
    search: paramValue(params, 'search'),
    namespace: paramValue(params, 'namespace'),
    workloadStatus: listedValue(paramValue(params, 'status'), workloadStatuses),
  }
}

function podsStateFromParams(params: URLSearchParams): PodListState {
  return {
    ...defaultPodList,
    search: paramValue(params, 'search'),
    namespace: paramValue(params, 'namespace'),
    podStatus: listedValue(paramValue(params, 'status'), podStatuses),
    restarts: listedValue(paramValue(params, 'restarts'), restartFilters) || 'any',
    problematic: ['true', 'false'].includes(paramValue(params, 'problematic')) ? paramValue(params, 'problematic') : '',
  }
}

function eventsStateFromParams(params: URLSearchParams): EventListState {
  const type = listedValue(paramValue(params, 'status'), eventTypes) || listedValue(paramValue(params, 'type'), eventTypes)
  return {
    ...defaultEventList,
    search: paramValue(params, 'search'),
    namespace: paramValue(params, 'namespace'),
    eventType: type,
  }
}

function sameListState<T extends object>(left: T, right: T): boolean {
  return (Object.keys(left) as Array<keyof T>).every((key) => left[key] === right[key])
}

export function WorkloadsPage() {
  const { status, selection } = useActiveSelection()
  const { kind, namespace, name } = useParams<{ kind: string; namespace: string; name: string }>()
  const navigate = useNavigate()
  const paramItem = useMemo(() => {
    if (!kind || !namespace || !name) return null
    return workloadFromParams(kind, namespace, name)
  }, [kind, namespace, name])
  const [params] = useSearchParams()
  const [draft, setDraft] = useState<WorkloadListState>(() => workloadsStateFromParams(params))
  const [applied, setApplied] = useState<WorkloadListState>(() => workloadsStateFromParams(params))
  const [cursor, setCursor] = useGenerationCursor(selection?.generation)
  const [selected, setSelected] = useState<GenerationSelection<Workload> | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const activeSelected = useMemo(() => {
    if (paramItem) return paramItem
    if (selected && selected.generation === selection?.generation) return selected.item
    return null
  }, [paramItem, selected, selection?.generation])
  const list = useQuery({
    queryKey: ['resources', 'workloads', selection?.generation, applied, cursor],
    queryFn: ({ signal }) => getWorkloads({ limit: 100, search: applied.search || undefined, namespaces: namespaceValues(applied.namespace), kinds: applied.kind ? [applied.kind] : undefined, statuses: applied.workloadStatus ? [applied.workloadStatus] : undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, selection?.generation),
    enabled: Boolean(selection),
  })
  const detail = useQuery({
    queryKey: ['resources', 'workload-detail', selection?.generation, activeSelected?.kind, activeSelected?.namespace, activeSelected?.name],
    queryFn: ({ signal }) => getWorkload(workloadKindPath(activeSelected!.kind), activeSelected!.namespace, activeSelected!.name, signal, selection!.generation),
    enabled: Boolean(selection && activeSelected),
  })
  const yaml = useMutation({ mutationFn: ({ kind: targetKind, namespace, name }: Workload) => requests.run((signal) => getWorkloadYAML(workloadKindPath(targetKind), namespace, name, signal)) })

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
    navigate('/workloads')
  }

  return (
    <div className="resource-page">
      <PageHeading title="Workloads" description="Deployments, StatefulSets, DaemonSets, Jobs and CronJobs in the active scope." action={selection ? <ResourceLiveUpdates key={`workloads/${selection.generation}`} generation={selection.generation} topics={['workloads']} queryKeys={[["resources", "workloads"]]} /> : null} />
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={() => { setApplied(draft); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'workloads'] })} onClear={() => { setDraft({ ...defaultWorkloadList }); setApplied({ ...defaultWorkloadList }); setCursor(''); closeDetail() }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('kind', 'Kind', applied.kind), ...activeFilter('status', 'Status', applied.workloadStatus),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={workloadSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <label>Namespace<Input value={draft.namespace} maxLength={256} placeholder="active scope; comma-separated" onChange={(event) => setDraft((current) => ({ ...current, namespace: event.target.value }))} /></label>
        <label>Kind<Select value={draft.kind} onChange={(event) => setDraft((current) => ({ ...current, kind: event.target.value }))}><option value="">All kinds</option><option value="deployments">Deployments</option><option value="statefulsets">StatefulSets</option><option value="daemonsets">DaemonSets</option><option value="jobs">Jobs</option><option value="cronjobs">CronJobs</option></Select></label>
        <label>Status<Select value={draft.workloadStatus} onChange={(event) => setDraft((current) => ({ ...current, workloadStatus: event.target.value }))}><option value="">All statuses</option>{workloadStatuses.map((value) => <option key={value}>{value}</option>)}</Select></label>
      </ResourceListControls>
      {selection ? <SavedFilterControls collection="workloads" generation={selection.generation} currentQuery={compactFilterQuery([
        ['namespace', namespaceValues(applied.namespace)], ['search', applied.search], ['kind', applied.kind ? [applied.kind] : []], ['status', applied.workloadStatus ? [applied.workloadStatus] : []], ['sort', applied.sort], ['order', applied.order],
      ])} onApply={(query) => {
        const next: WorkloadListState = {
          search: savedString(query, 'search'),
          namespace: savedNamespaces(query),
          kind: savedFirst(query, 'kind', workloadKinds),
          workloadStatus: savedFirst(query, 'status', workloadStatuses),
          sort: savedSort(query, workloadSorts, 'identity'),
          order: savedOrder(query, 'asc'),
        }
        setDraft(next)
        setApplied(next)
        setCursor('')
        closeDetail()
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}>
          <div className="resource-layout">
            <div className="resource-table-wrap">
              <DataTable
                caption="Authorized workload page"
                rows={list.data?.items ?? []}
                getRowKey={(item) => `${item.kind}/${item.namespace}/${item.name}`}
                columns={[
                  { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
                    { key: 'name', header: 'Kind / name', cell: (item) => <TableLink aria-label={`Open ${item.kind} ${item.name} in ${item.namespace}`} onClick={() => { requests.abortAll(); setSelected({ generation: selection!.generation, item }); yaml.reset(); navigate(`/workloads/${workloadKindPath(item.kind)}/${encodeURIComponent(item.namespace)}/${encodeURIComponent(item.name)}`) }} primary={item.name} secondary={item.kind} /> },

                  { key: 'ready', header: 'Ready', cell: (item) => `${item.ready ?? '—'} / ${item.desired ?? '—'}` },
                  { key: 'status', header: 'Status', cell: (item) => <Badge variant={statusBadgeVariant(item.status)}>{item.status}</Badge> },
                  { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
                ]}
              />
              {list.data ? <CollectionFooter result={list.data} onNext={(next) => { setCursor(next); closeDetail() }} onRestart={() => { setCursor(''); closeDetail() }} /> : null}
            </div>
            <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={detailTitle(activeSelected ? `${activeSelected.kind} ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}>
              {activeSelected ? <>
                {detail.isPending ? <p>Loading detail…</p> : detail.isError ? <p className="field-error">{errorMessage(detail.error)}</p> : detail.data ? <>
                  <dl className="resource-facts"><div><dt>Status</dt><dd>{detail.data.status}</dd></div><div><dt>Resource version</dt><dd>{detail.data.metadata.resourceVersion}</dd></div><div><dt>Containers</dt><dd>{detail.data.containers.map((value) => value.name).join(', ') || 'none'}</dd></div><div><dt>Labels</dt><dd>{Object.entries(detail.data.metadata.labels ?? {}).map(([label, value]) => `${label}=${value}`).join(', ') || 'none'}</dd></div></dl>
                  <WorkloadActions key={`${selection!.generation}/${detail.data.metadata.uid}`} detail={detail.data} selection={selection as SelectionSummary} />
                </> : null}
                <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} />
              </> : null}
            </Drawer>
          </div>
        </QueryState>
      </SelectionGate>
    </div>
  )
}

export function PodsPage() {
  const { status, selection } = useActiveSelection()
  const { namespace, name } = useParams<{ namespace: string; name: string }>()
  const navigate = useNavigate()
  const paramItem = useMemo(() => {
    if (!namespace || !name) return null
    return podFromParams(namespace, name)
  }, [namespace, name])
  const [params] = useSearchParams()
  const [draft, setDraft] = useState<PodListState>(() => podsStateFromParams(params))
  const [applied, setApplied] = useState<PodListState>(() => podsStateFromParams(params))
  const [cursor, setCursor] = useGenerationCursor(selection?.generation)
  const [selected, setSelected] = useState<GenerationSelection<Pod> | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const activeSelected = useMemo(() => {
    if (paramItem) return paramItem
    if (selected && selected.generation === selection?.generation) return selected.item
    return null
  }, [paramItem, selected, selection?.generation])
  const list = useQuery({
    queryKey: ['resources', 'pods', selection?.generation, applied, cursor],
    queryFn: ({ signal }) => getPods({ limit: 100, search: applied.search || undefined, namespaces: namespaceValues(applied.namespace), statuses: applied.podStatus ? [applied.podStatus] : undefined, workload: applied.workload || undefined, node: applied.node || undefined, restarts: applied.restarts as 'any' | 'gt0' | 'gte3' | 'gte10', problematic: applied.problematic === '' ? undefined : applied.problematic === 'true', ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, selection?.generation),
    enabled: Boolean(selection),
  })
  const detail = useQuery({ queryKey: ['resources', 'pod-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getPod(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(selection && activeSelected) })
  const yaml = useMutation({ mutationFn: ({ namespace, name }: Pod) => requests.run((signal) => getPodYAML(namespace, name, signal)) })

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
    navigate('/pods')
  }

  return (
    <div className="resource-page">
      <PageHeading title="Pods" description="Bounded Pod inventory with readiness, restarts, owner and problem evidence." action={<div className="resource-header-actions"><Link className="button button--secondary" to="/logs">Open logs</Link>{selection ? <ResourceLiveUpdates key={`pods/${selection.generation}`} generation={selection.generation} topics={['pods']} queryKeys={[["resources", "pods"]]} /> : null}</div>} />
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={() => { setApplied(draft); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'pods'] })} onClear={() => { setDraft({ ...defaultPodList }); setApplied({ ...defaultPodList }); setCursor(''); closeDetail() }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('workload', 'Workload owner', applied.workload), ...activeFilter('node', 'Node', applied.node), ...activeFilter('status', 'Status', applied.podStatus), ...activeFilter('restarts', 'Restarts', applied.restarts === 'any' ? '' : applied.restarts), ...activeFilter('problematic', 'Problem evidence', applied.problematic === 'true' ? 'problematic only' : applied.problematic === 'false' ? 'without evidence' : ''),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={podSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <label>Namespace<Input value={draft.namespace} maxLength={256} placeholder="active scope; comma-separated" onChange={(event) => setDraft((current) => ({ ...current, namespace: event.target.value }))} /></label>
        <label>Workload owner<Input value={draft.workload} maxLength={256} placeholder="exact owner name" onChange={(event) => setDraft((current) => ({ ...current, workload: event.target.value }))} /></label>
        <label>Node<Input value={draft.node} maxLength={256} onChange={(event) => setDraft((current) => ({ ...current, node: event.target.value }))} /></label>
        <label>Status<Select value={draft.podStatus} onChange={(event) => setDraft((current) => ({ ...current, podStatus: event.target.value }))}><option value="">All statuses</option>{podStatuses.map((value) => <option key={value}>{value}</option>)}</Select></label>
        <label>Restarts<Select value={draft.restarts} onChange={(event) => setDraft((current) => ({ ...current, restarts: event.target.value }))}><option value="any">Any</option><option value="gt0">More than 0</option><option value="gte3">At least 3</option><option value="gte10">At least 10</option></Select></label>
        <label>Problem evidence<Select value={draft.problematic} onChange={(event) => setDraft((current) => ({ ...current, problematic: event.target.value }))}><option value="">All Pods</option><option value="true">Problematic only</option><option value="false">Without evidence</option></Select></label>
      </ResourceListControls>
      {selection ? <SavedFilterControls collection="pods" generation={selection.generation} currentQuery={compactFilterQuery([
        ['namespace', namespaceValues(applied.namespace)], ['search', applied.search], ['status', applied.podStatus ? [applied.podStatus] : []], ['workload', applied.workload], ['node', applied.node], ['restarts', applied.restarts === 'any' ? '' : applied.restarts], ['problematic', applied.problematic === '' ? undefined : applied.problematic === 'true'], ['sort', applied.sort], ['order', applied.order],
      ])} onApply={(query) => {
        const savedRestarts = savedString(query, 'restarts', restartFilters)
        const next: PodListState = {
          search: savedString(query, 'search'),
          namespace: savedNamespaces(query),
          podStatus: savedFirst(query, 'status', podStatuses),
          workload: savedString(query, 'workload'),
          node: savedString(query, 'node'),
          restarts: savedRestarts || 'any',
          problematic: typeof query.problematic === 'boolean' ? String(query.problematic) : '',
          sort: savedSort(query, podSorts, 'identity'),
          order: savedOrder(query, 'asc'),
        }
        setDraft(next)
        setApplied(next)
        setCursor('')
        closeDetail()
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}>
          <div className="resource-layout"><div className="resource-table-wrap">
            <DataTable
              caption="Authorized Pod page"
              rows={list.data?.items ?? []}
              getRowKey={(item) => `${item.namespace}/${item.name}`}
              columns={[
                { key: 'namespace', header: 'Namespace / Pod', cell: (item) => <TableLink aria-label={`Open Pod ${item.name} in ${item.namespace}`} onClick={() => { requests.abortAll(); setSelected({ generation: selection!.generation, item }); yaml.reset(); navigate(`/pods/${encodeURIComponent(item.namespace)}/${encodeURIComponent(item.name)}`) }} primary={<>{item.name}{item.problematic ? <Badge variant="danger" className="ml-2">problem</Badge> : null}</>} secondary={item.namespace} /> },
                { key: 'status', header: 'Status', cell: (item) => <Badge variant={statusBadgeVariant(item.status)}>{item.status}</Badge> },
                { key: 'ready', header: 'Ready', cell: (item) => `${item.ready.current}/${item.ready.desired}` },
                { key: 'restarts', header: 'Restarts', cell: (item) => item.restarts },
                { key: 'node', header: 'Node', cell: (item) => item.node ?? '—' },
                { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
              ]}
            />
            {list.data ? <CollectionFooter result={list.data} onNext={(next) => { setCursor(next); closeDetail() }} onRestart={() => { setCursor(''); closeDetail() }} /> : null}</div>
            <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={detailTitle(activeSelected ? `Pod ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}>
              {activeSelected ? <>
                {detail.isPending ? <p>Loading detail…</p> : detail.isError ? <p className="field-error">{errorMessage(detail.error)}</p> : detail.data ? <><dl className="resource-facts"><div><dt>UID</dt><dd>{detail.data.metadata.uid}</dd></div><div><dt>Owner</dt><dd>{detail.data.summary.owner ? `${detail.data.summary.owner.kind}/${detail.data.summary.owner.name}` : 'standalone'}</dd></div><div><dt>Containers</dt><dd>{detail.data.containers.map((value) => `${value.spec.name} (${value.state})`).join(', ') || 'none'}</dd></div><div><dt>IP</dt><dd>{detail.data.summary.ip ?? '—'}</dd></div></dl><Link className="button button--secondary button--compact" to={`/logs?namespace=${encodeURIComponent(activeSelected.namespace)}&pod=${encodeURIComponent(activeSelected.name)}&container=${encodeURIComponent(detail.data.containers[0]?.spec.name ?? '')}`}>View logs</Link><PodActions key={`${selection!.generation}/${detail.data.metadata.uid}`} detail={detail.data} selection={selection as SelectionSummary} /></> : null}
                <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} />
              </> : null}
            </Drawer>
          </div>
        </QueryState>
      </SelectionGate>
    </div>
  )
}

export function EventsPage() {
  const { status, selection } = useActiveSelection()
  const [params] = useSearchParams()
  const [draft, setDraft] = useState<EventListState>(() => eventsStateFromParams(params))
  const [applied, setApplied] = useState<EventListState>(() => eventsStateFromParams(params))
  const [cursor, setCursor] = useGenerationCursor(selection?.generation)
  const queryClient = useQueryClient()
  const list = useQuery({ queryKey: ['resources', 'events', selection?.generation, applied, cursor], queryFn: ({ signal }) => getEvents({ limit: 100, search: applied.search || undefined, namespaces: namespaceValues(applied.namespace), statuses: applied.eventType ? [applied.eventType] : undefined, objectKind: applied.objectKind || undefined, reason: applied.reason || undefined, continueToken: cursor || undefined, ...optionalSort(applied.sort, applied.order, 'timestamp', 'desc') }, signal, selection?.generation), enabled: Boolean(selection) })
  return (
    <div className="resource-page">
      <PageHeading title="Events" description="Real Kubernetes events ordered within the bounded page; type, source and count are preserved." action={selection ? <ResourceLiveUpdates key={`events/${selection.generation}`} generation={selection.generation} topics={['events']} queryKeys={[["resources", "events"]]} /> : null} />
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={() => { setApplied(draft); setCursor('') }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'events'] })} onClear={() => { setDraft({ ...defaultEventList }); setApplied({ ...defaultEventList }); setCursor('') }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('type', 'Type', applied.eventType), ...activeFilter('objectKind', 'Object kind', applied.objectKind), ...activeFilter('reason', 'Reason', applied.reason),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="timestamp" defaultOrder="desc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={eventSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <label>Namespace<Input value={draft.namespace} maxLength={256} placeholder="active scope; comma-separated" onChange={(event) => setDraft((current) => ({ ...current, namespace: event.target.value }))} /></label>
        <label>Type<Select value={draft.eventType} onChange={(event) => setDraft((current) => ({ ...current, eventType: event.target.value }))}><option value="">All types</option>{eventTypes.map((value) => <option key={value}>{value}</option>)}</Select></label>
        <label>Object kind<Input value={draft.objectKind} maxLength={256} onChange={(event) => setDraft((current) => ({ ...current, objectKind: event.target.value }))} /></label>
        <label>Reason<Input value={draft.reason} maxLength={256} onChange={(event) => setDraft((current) => ({ ...current, reason: event.target.value }))} /></label>
      </ResourceListControls>
      {selection ? <SavedFilterControls collection="events" generation={selection.generation} currentQuery={compactFilterQuery([
        ['namespace', namespaceValues(applied.namespace)], ['search', applied.search], ['status', applied.eventType ? [applied.eventType] : []], ['sort', applied.sort], ['order', applied.order], ['objectKind', applied.objectKind], ['reason', applied.reason],
      ])} onApply={(query) => {
        const next: EventListState = {
          search: savedString(query, 'search'),
          namespace: savedNamespaces(query),
          eventType: savedFirst(query, 'status', eventTypes),
          objectKind: savedString(query, 'objectKind'),
          reason: savedString(query, 'reason'),
          sort: savedSort(query, eventSorts, 'timestamp'),
          order: savedOrder(query, 'desc'),
        }
        setDraft(next)
        setApplied(next)
        setCursor('')
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}><QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}><div className="resource-table-wrap">
        <DataTable
          caption="Authorized event page"
          rows={list.data?.items ?? []}
          getRowKey={(item, index) => `${item.namespace}/${item.objectKind}/${item.objectName}/${item.timestamp ?? index}`}
          columns={[
            { key: 'time', header: 'Time', cell: (item) => dateTime(item.timestamp) },
            { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
            { key: 'object', header: 'Object', cell: (item) => `${item.objectKind}/${item.objectName}` },
            { key: 'type', header: 'Type / reason', cell: (item) => <><Badge variant={eventBadgeVariant(item.type)}>{item.type}</Badge><small className="block text-kp-overlay-text">{item.reason}</small></> },
            { key: 'count', header: 'Count', cell: (item) => item.count },
            { key: 'message', header: 'Message', cell: (item) => <span className="resource-message">{item.message}</span> },
          ]}
        />
        {list.data ? <CollectionFooter result={list.data} onNext={setCursor} onRestart={() => setCursor('')} /> : null}</div></QueryState></SelectionGate>
    </div>
  )
}

type NetworkTab = 'services' | 'ingresses' | 'endpoint-slices' | 'port-forwards'
type NetworkResourceTab = Exclude<NetworkTab, 'port-forwards'>
type NetworkItem = ServiceResource | IngressResource | EndpointSliceResource
type NetworkSelection = { generation: string; tab: NetworkResourceTab; namespace: string; name: string }

interface SimpleListState {
  search: string
  sort: string
  order: ListSortOrder
}

const defaultSimpleList: SimpleListState = { search: '', sort: 'identity', order: 'asc' }
const defaultNetworkLists: Record<NetworkResourceTab, SimpleListState> = {
  services: { ...defaultSimpleList },
  ingresses: { ...defaultSimpleList },
  'endpoint-slices': { ...defaultSimpleList },
}
const defaultNetworkCursors: Record<NetworkResourceTab, string> = { services: '', ingresses: '', 'endpoint-slices': '' }

const networkSortOptions: Record<NetworkResourceTab, readonly ListSortOption[]> = {
  services: [
    { value: 'identity', label: 'Namespace and name' },
    { value: 'name', label: 'Name' },
    { value: 'type', label: 'Service type' },
  ],
  ingresses: [
    { value: 'identity', label: 'Namespace and name' },
    { value: 'name', label: 'Name' },
  ],
  'endpoint-slices': [
    { value: 'identity', label: 'Namespace and name' },
    { value: 'name', label: 'Name' },
    { value: 'addressType', label: 'Address type' },
  ],
}

function NetworkDetailView({ tab, service, ingress, slice }: { tab: NetworkSelection['tab']; service?: ServiceDetail; ingress?: IngressDetail; slice?: EndpointSliceDetail }) {
  if (tab === 'services' && service) return <><dl className="resource-facts"><div><dt>Type</dt><dd>{service.summary.type}</dd></div><div><dt>Cluster IPs</dt><dd>{service.summary.clusterIPs.join(', ') || 'none'}</dd></div><div><dt>Session affinity</dt><dd>{service.sessionAffinity}</dd></div><div><dt>Ports</dt><dd>{service.summary.ports.map((port) => `${port.port}/${port.protocol}`).join(', ') || 'none'}</dd></div></dl></>
  if (tab === 'ingresses' && ingress) return <><dl className="resource-facts"><div><dt>Class</dt><dd>{ingress.summary.className ?? 'default'}</dd></div><div><dt>Hosts</dt><dd>{ingress.summary.hosts.join(', ') || 'none'}</dd></div><div><dt>Paths</dt><dd>{ingress.summary.paths.map((path) => `${path.host}${path.path} → ${path.backend.serviceName}`).join(', ') || 'none'}</dd></div><div><dt>Load balancers</dt><dd>{ingress.loadBalancerAddresses.join(', ') || 'none'}</dd></div></dl></>
  if (tab === 'endpoint-slices' && slice) return <><dl className="resource-facts"><div><dt>Address type</dt><dd>{slice.summary.addressType}</dd></div><div><dt>Endpoints</dt><dd>{slice.summary.endpoints.length}</dd></div><div><dt>Addresses</dt><dd>{slice.summary.endpoints.flatMap((endpoint) => endpoint.addresses).join(', ') || 'none'}</dd></div><div><dt>Ports</dt><dd>{slice.summary.ports.map((port) => port.port ?? 'named').join(', ') || 'none'}</dd></div></dl></>
  return null
}

function networkTabFromParams(tab: string): NetworkResourceTab | null {
  return tab === 'services' || tab === 'ingresses' || tab === 'endpoint-slices' ? tab : null
}

export function NetworkPage() {
  const { status, selection } = useActiveSelection()
  const { tab: tabParam, namespace, name } = useParams<{ tab: string; namespace: string; name: string }>()
  const navigate = useNavigate()
  const [tabState, setTab] = useState<NetworkTab>('services')
  const tab = useMemo(() => networkTabFromParams(tabParam ?? '') || tabState, [tabParam, tabState])
  const [cursors, setCursorValue] = useGenerationCursorMap(selection?.generation, defaultNetworkCursors)
  const [drafts, setDrafts] = useState<Record<NetworkResourceTab, SimpleListState>>(() => structuredClone(defaultNetworkLists))
  const [appliedLists, setAppliedLists] = useState<Record<NetworkResourceTab, SimpleListState>>(() => structuredClone(defaultNetworkLists))
  const [selected, setSelected] = useState<NetworkSelection | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const paramSelection = useMemo(() => {
    const parsedTab = networkTabFromParams(tabParam ?? '')
    if (!parsedTab || !namespace || !name || !selection) return null
    return { generation: selection.generation, tab: parsedTab, namespace, name }
  }, [tabParam, namespace, name, selection])
  const resourceTab: NetworkResourceTab = tab === 'port-forwards' ? 'services' : tab
  const draft = drafts[resourceTab]
  const applied = appliedLists[resourceTab]
  const activeSelected = paramSelection || (selected?.generation === selection?.generation ? selected : null)
  const networkOptions = (value: NetworkResourceTab) => ({ limit: 100, search: appliedLists[value].search || undefined, continueToken: cursors[value] || undefined, ...optionalSort(appliedLists[value].sort, appliedLists[value].order, 'identity', 'asc') })
  const services = useQuery({ queryKey: ['resources', 'services', selection?.generation, appliedLists.services, cursors.services], queryFn: ({ signal }) => getServices(networkOptions('services'), signal, selection?.generation), enabled: Boolean(selection && tab === 'services') })
  const ingresses = useQuery({ queryKey: ['resources', 'ingresses', selection?.generation, appliedLists.ingresses, cursors.ingresses], queryFn: ({ signal }) => getIngresses(networkOptions('ingresses'), signal, selection?.generation), enabled: Boolean(selection && tab === 'ingresses') })
  const slices = useQuery({ queryKey: ['resources', 'endpoint-slices', selection?.generation, appliedLists['endpoint-slices'], cursors['endpoint-slices']], queryFn: ({ signal }) => getEndpointSlices(networkOptions('endpoint-slices'), signal, selection?.generation), enabled: Boolean(selection && tab === 'endpoint-slices') })
  const forwards = useQuery({ queryKey: ['port-forwards', selection?.generation], queryFn: ({ signal }) => getPortForwards(signal, selection!.generation), enabled: Boolean(selection && tab === 'port-forwards'), refetchInterval: 10_000 })
  const serviceDetail = useQuery({ queryKey: ['resources', 'service-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getService(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: activeSelected?.tab === 'services' })
  const ingressDetail = useQuery({ queryKey: ['resources', 'ingress-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getIngress(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: activeSelected?.tab === 'ingresses' })
  const sliceDetail = useQuery({ queryKey: ['resources', 'slice-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getEndpointSlice(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: activeSelected?.tab === 'endpoint-slices' })
  const yaml = useMutation({ mutationFn: (value: NetworkSelection) => requests.run((signal) => value.tab === 'services' ? getServiceYAML(value.namespace, value.name, signal) : value.tab === 'ingresses' ? getIngressYAML(value.namespace, value.name, signal) : getEndpointSliceYAML(value.namespace, value.name, signal)) })
  const close = useMutation({ mutationFn: (id: string) => requests.run(async (signal) => { const session = await getSession(signal); if (session.generation !== selection!.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' }); return closePortForward(id, selection!.generation, session.csrfToken, signal) }), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['port-forwards'] }) })
  const active: CollectionResult<NetworkItem> | undefined = tab === 'services' && services.data ? { ...services.data, items: services.data.items } : tab === 'ingresses' && ingresses.data ? { ...ingresses.data, items: ingresses.data.items } : tab === 'endpoint-slices' && slices.data ? { ...slices.data, items: slices.data.items } : undefined
  const activeQuery = tab === 'services' ? services : tab === 'ingresses' ? ingresses : slices
  const currentDetail = activeSelected?.tab === 'services' ? serviceDetail : activeSelected?.tab === 'ingresses' ? ingressDetail : sliceDetail

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
    navigate('/network')
  }

  function setCursor(value: string) {
    if (tab === 'port-forwards') return
    setCursorValue(tab, value)
    closeDetail()
  }

  return (
    <div className="resource-page">
      <PageHeading title="Network" description="Services, Ingresses, EndpointSlices and loopback-only port-forward sessions." />
      <div className="resource-tabs" role="tablist" aria-label="Network resource type">{(['services', 'ingresses', 'endpoint-slices', 'port-forwards'] as const).map((value) => <Button type="button" role="tab" aria-selected={tab === value} aria-controls="network-panel" variant={tab === value ? 'secondary' : 'ghost'} size="compact" key={value} onClick={() => { setTab(value); closeDetail() }}>{value}</Button>)}</div>
      {selection && tab !== 'port-forwards' ? <ResourceLiveUpdates key={`${tab}/${selection.generation}`} generation={selection.generation} topics={[tab]} queryKeys={[["resources", tab]]} /> : null}
      {tab !== 'port-forwards' ? <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], search: value } }))} onApply={() => { setAppliedLists((current) => ({ ...current, [resourceTab]: { ...draft } })); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} onClear={() => { setDrafts((current) => ({ ...current, [resourceTab]: { ...defaultSimpleList } })); setAppliedLists((current) => ({ ...current, [resourceTab]: { ...defaultSimpleList } })); setCursor(''); closeDetail() }} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={networkSortOptions[resourceTab]} onSortChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], sort: value } }))} onOrderChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], order: value } }))} /> : null}
      <div id="network-panel" role="tabpanel">
        <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
          {tab === 'port-forwards' ? <QueryState pending={forwards.isPending} error={forwards.error ?? close.error} empty={forwards.data?.length === 0}><div className="session-grid">{forwards.data?.map((item) => <article key={item.id}><strong>{item.localAddress}:{item.localPort}</strong><span>{item.context} · {item.namespace}/{item.pod} → {item.remotePort}</span><small>{item.status} · created {dateTime(item.createdAt)} · expires {dateTime(item.expiresAt)}</small>{item.endedAt ? <small>ended {dateTime(item.endedAt)} · {item.endReason ?? item.status}</small> : null}{item.status === 'active' ? <Button variant="danger" size="compact" onClick={() => close.mutate(item.id)}>Close loopback session</Button> : null}</article>)}</div></QueryState> : <QueryState pending={activeQuery.isPending} error={activeQuery.error} empty={active?.items.length === 0}><div className="resource-layout"><div className="resource-table-wrap"><DataTable
                  caption={`Authorized ${tab} page`}
                  rows={active?.items ?? []}
                  getRowKey={(item) => `${item.namespace}/${item.name}`}
                  columns={[
                    { key: 'namespace', header: 'Namespace / name', cell: (item) => <TableLink aria-label={`Open ${tab} ${item.name} in ${item.namespace}`} onClick={() => { closeDetail(); setSelected({ generation: selection!.generation, tab: tab as NetworkSelection['tab'], namespace: item.namespace, name: item.name }); navigate(`/network/${tab}/${encodeURIComponent(item.namespace)}/${encodeURIComponent(item.name)}`) }} primary={item.name} secondary={item.namespace} /> },
                    { key: 'type', header: 'Type', cell: (item) => ('type' in item ? item.type : 'className' in item ? (item.className ?? 'Ingress') : item.addressType) },
                    { key: 'summary', header: 'Summary', cell: (item) => ('clusterIPs' in item ? item.clusterIPs.join(', ') : 'hosts' in item ? item.hosts.join(', ') : `${item.endpoints.length} endpoints`) },
                  ]}
                />{active ? <CollectionFooter result={active} onNext={setCursor} onRestart={() => setCursor('')} /> : null}</div><Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={detailTitle(activeSelected ? `${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}>
                  {activeSelected ? <>
                    {currentDetail.isPending ? <p>Loading detail…</p> : currentDetail.isError ? <p className="field-error">{errorMessage(currentDetail.error)}</p> : <NetworkDetailView tab={activeSelected.tab} service={serviceDetail.data} ingress={ingressDetail.data} slice={sliceDetail.data} />}
                    <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} />
                  </> : null}
                </Drawer></div></QueryState>}
        </SelectionGate>
      </div>
    </div>
  )
}

type ConfigTab = 'configmaps' | 'secrets'
type ConfigItem = ConfigMapResource | SecretMetadata
type ConfigSelection = { generation: string; namespace: string; name: string }

function configTabFromParams(tab: string): ConfigTab | null {
  return tab === 'configmaps' || tab === 'secrets' ? tab : null
}

const configSortOptions: readonly ListSortOption[] = [
  { value: 'identity', label: 'Namespace and name' },
  { value: 'name', label: 'Name' },
  { value: 'createdAt', label: 'Creation time' },
]
const defaultConfigLists: Record<ConfigTab, SimpleListState> = {
  configmaps: { ...defaultSimpleList },
  secrets: { ...defaultSimpleList },
}
const defaultConfigCursors: Record<ConfigTab, string> = { configmaps: '', secrets: '' }

function ConfigMapDetailView({ detail }: { detail: ConfigMapDetail }) {
  return <><dl className="resource-facts"><div><dt>Resource version</dt><dd>{detail.metadata.resourceVersion}</dd></div><div><dt>Total bytes</dt><dd>{detail.totalBytes}</dd></div><div><dt>Entries</dt><dd>{detail.entries.length}</dd></div><div><dt>Truncated</dt><dd>{detail.truncated ? 'yes' : 'no'}</dd></div></dl><div className="config-entry-list">{detail.entries.map((entry) => <details key={entry.key}><summary>{entry.key} · {entry.encoding}{entry.truncated ? ' · truncated' : ''}</summary><pre>{entry.value}</pre></details>)}</div></>
}

function SecretMetadataView({ secret }: { secret: SecretMetadata }) {
  return <><dl className="resource-facts"><div><dt>API version</dt><dd>{secret.apiVersion}</dd></div><div><dt>Kind</dt><dd>{secret.kind}</dd></div><div><dt>UID</dt><dd>{secret.metadata.uid}</dd></div><div><dt>Created</dt><dd>{dateTime(secret.metadata.creationTimestamp)}</dd></div>{secret.metadata.deletionTimestamp ? <div><dt>Deleting</dt><dd>{dateTime(secret.metadata.deletionTimestamp)}</dd></div> : null}</dl><p className="permission-notice">Secret values, annotations, managed fields and YAML are intentionally unavailable.</p></>
}

export function ConfigPage() {
  const { status, selection } = useActiveSelection()
  const { tab: tabParam, namespace, name } = useParams<{ tab: string; namespace: string; name: string }>()
  const navigate = useNavigate()
  const [tabState, setTab] = useState<ConfigTab>('configmaps')
  const tab = useMemo(() => configTabFromParams(tabParam ?? '') || tabState, [tabParam, tabState])
  const [cursors, setCursorValue] = useGenerationCursorMap(selection?.generation, defaultConfigCursors)
  const [drafts, setDrafts] = useState<Record<ConfigTab, SimpleListState>>(() => structuredClone(defaultConfigLists))
  const [appliedLists, setAppliedLists] = useState<Record<ConfigTab, SimpleListState>>(() => structuredClone(defaultConfigLists))
  const [selected, setSelected] = useState<ConfigSelection | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const paramSelection = useMemo(() => {
    if (!namespace || !name || !selection) return null
    return { generation: selection.generation, namespace, name }
  }, [namespace, name, selection])
  const activeSelected = paramSelection || (selected?.generation === selection?.generation ? selected : null)
  const draft = drafts[tab]
  const applied = appliedLists[tab]
  const configOptions = (value: ConfigTab) => ({ limit: 100, search: appliedLists[value].search || undefined, continueToken: cursors[value] || undefined, ...optionalSort(appliedLists[value].sort, appliedLists[value].order, 'identity', 'asc') })
  const configMaps = useQuery({ queryKey: ['resources', 'configmaps', selection?.generation, appliedLists.configmaps, cursors.configmaps], queryFn: ({ signal }) => getConfigMaps(configOptions('configmaps'), signal, selection?.generation), enabled: Boolean(selection && tab === 'configmaps') })
  const secrets = useQuery({ queryKey: ['resources', 'secrets', selection?.generation, appliedLists.secrets, cursors.secrets], queryFn: ({ signal }) => getSecrets(configOptions('secrets'), signal, selection?.generation), enabled: Boolean(selection && tab === 'secrets') })
  const configDetail = useQuery({ queryKey: ['resources', 'configmap-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getConfigMap(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'configmaps') })
  const secretDetail = useQuery({ queryKey: ['resources', 'secret-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getSecret(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'secrets') })
  const yaml = useMutation({ mutationFn: (value: ConfigSelection) => requests.run((signal) => getConfigMapYAML(value.namespace, value.name, signal)) })

  const active: CollectionResult<ConfigItem> | undefined = tab === 'configmaps' && configMaps.data ? { ...configMaps.data, items: configMaps.data.items } : tab === 'secrets' && secrets.data ? { ...secrets.data, items: secrets.data.items } : undefined
  const activeQuery = tab === 'configmaps' ? configMaps : secrets

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
    navigate('/config')
  }

  function setCursor(value: string) {
    setCursorValue(tab, value)
    closeDetail()
  }

  return (
    <div className="resource-page">
      <PageHeading title="Config" description="ConfigMaps are fetched on detail; Secrets remain metadata-only and never expose values or YAML." />
      <div className="resource-tabs" role="tablist" aria-label="Configuration resource type">{(['configmaps', 'secrets'] as const).map((value) => <Button type="button" role="tab" aria-selected={tab === value} aria-controls="config-panel" variant={tab === value ? 'secondary' : 'ghost'} size="compact" key={value} onClick={() => { setTab(value); closeDetail() }}>{value}</Button>)}</div>
      {selection && tab === 'configmaps' ? <ResourceLiveUpdates key={`configmaps/${selection.generation}`} generation={selection.generation} topics={['configmaps']} queryKeys={[["resources", "configmaps"]]} /> : null}
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], search: value } }))} onApply={() => { setAppliedLists((current) => ({ ...current, [tab]: { ...draft } })); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} onClear={() => { setDrafts((current) => ({ ...current, [tab]: { ...defaultSimpleList } })); setAppliedLists((current) => ({ ...current, [tab]: { ...defaultSimpleList } })); setCursor(''); closeDetail() }} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={configSortOptions} onSortChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], sort: value } }))} onOrderChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], order: value } }))} />
      <div id="config-panel" role="tabpanel"><SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}><QueryState pending={activeQuery.isPending} error={activeQuery.error} empty={active?.items.length === 0}><div className="resource-layout"><div className="resource-table-wrap"><DataTable
                  caption={`Authorized ${tab} metadata page`}
                  rows={active?.items ?? []}
                  getRowKey={(item) => { const value = 'metadata' in item ? item.metadata : item; return `${value.namespace}/${value.name}` }}
                  columns={[
                    { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return <TableLink aria-label={`Open ${tab === 'secrets' ? 'Secret' : 'ConfigMap'} ${value.name} in ${value.namespace}`} onClick={() => { closeDetail(); setSelected({ generation: selection!.generation, namespace: value.namespace, name: value.name }); navigate(`/config/${tab}/${encodeURIComponent(value.namespace)}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.namespace} /> } },
                    { key: 'uid', header: 'UID', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return value.uid } },
                    { key: 'created', header: 'Created', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return dateTime(value.creationTimestamp) } },
                  ]}
                />{active ? <CollectionFooter result={active} onNext={setCursor} onRestart={() => setCursor('')} /> : null}</div><Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={detailTitle(activeSelected ? `${tab === 'secrets' ? 'Secret metadata' : 'ConfigMap'} ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}>
                  {activeSelected ? <>
                    {tab === 'configmaps' ? configDetail.isPending ? <p>Loading authorized entries…</p> : configDetail.isError ? <p className="field-error">{errorMessage(configDetail.error)}</p> : configDetail.data ? <ConfigMapDetailView detail={configDetail.data} /> : null : secretDetail.isPending ? <p>Loading metadata…</p> : secretDetail.isError ? <p className="field-error">{errorMessage(secretDetail.error)}</p> : secretDetail.data ? <SecretMetadataView secret={secretDetail.data} /> : null}
                    {tab === 'configmaps' ? <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} /> : null}
                  </> : null}
                </Drawer></div></QueryState></SelectionGate></div>
    </div>
  )
}
