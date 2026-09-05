import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { ScrollText } from 'lucide-react'

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
  getDashboardMetrics,
  getIngress,
  getIngresses,
  getIngressYAML,
  getEndpointsItem,
  getEndpointsList,
  getIngressClass,
  getIngressClasses,
  getNetworkPolicy,
  getNetworkPolicies,
  getNode,
  getNodes,
  getNodeYAML,
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
  Endpoints,
  IngressClass,
  NetworkPolicy,
  IngressDetail,
  IngressResource,
  NodeSummary,
  Pod,
  SecretMetadata,
  SelectionSummary,
  ServiceDetail,
  ServiceResource,
  Workload,
} from '../api/types'
import { Badge, Button, DataTable, Drawer, Input, Select, StatusBadge } from './ui'
import { PodActions, WorkloadActions } from './ResourceActions'
import { ResourceListControls } from './ResourceListControls'
import type { ActiveListFilter, ListSortOrder, ListSortOption } from './ResourceListControls'
import { ResourceLiveUpdates } from './ResourceLiveUpdates'
import { SavedFilterControls } from './SavedFilterControls'
import { FavoriteButton } from './FavoriteButton'
import { YamlViewer } from './YamlViewer'
import { errorMessage } from './resource/errors'
import { CollectionFooter, QueryState, SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import { TableLink } from './resource/TableLink'
import { Facts } from './resource/Facts'
import { age, dateTime } from './resource/format'
import { eventBadgeVariant, statusBadgeVariant } from './resource/status'

function detailTitle(title: string) {
  return <h2 className="text-lg text-kp-text break-words">{title}</h2>
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
  return ({ Deployment: 'deployments', StatefulSet: 'statefulsets', DaemonSet: 'daemonsets', Job: 'jobs', CronJob: 'cronjobs', ReplicaSet: 'replicasets' } as const)[kind]
}

function workloadFromParams(kind: string, namespace: string, name: string): Workload | null {
  const kindMap: Record<string, Workload['kind']> = {
    deployments: 'Deployment',
    statefulsets: 'StatefulSet',
    daemonsets: 'DaemonSet',
    jobs: 'Job',
    cronjobs: 'CronJob',
    replicasets: 'ReplicaSet',
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
  return [...new Set(value.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean))]
}

// Mirrors the backend collection limit for one list request (MaximumNamespaces).
const maximumNamespaceFilter = 100

function NamespaceFilterInput({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const entries = namespaceValues(value)
  return (
    <label className="min-w-0">
      Namespace
      <Input value={value} maxLength={4096} placeholder="active scope; comma-separated" onChange={(event) => onChange(event.target.value)} />
      {entries.length > maximumNamespaceFilter ? (
        <small role="note" className="mt-1 block text-xs text-kp-yellow">
          {entries.length} namespaces listed; a query accepts at most {maximumNamespaceFilter}. Narrow the filter before applying.
        </small>
      ) : null}
    </label>
  )
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

const workloadKinds = ['deployments', 'statefulsets', 'daemonsets', 'jobs', 'cronjobs', 'replicasets'] as const
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
    kind: listedValue(paramValue(params, 'kind'), workloadKinds),
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
  const { kind: kindParam, namespace, name } = useParams<{ kind: string; namespace: string; name: string }>()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const paramItem = useMemo(() => {
    if (!kindParam || !namespace || !name) return null
    return workloadFromParams(kindParam, namespace, name)
  }, [kindParam, namespace, name])
  // Sidebar deep links use /workloads/kind/:kind; the path param presets the filter.
  const kindPreset = useMemo(() => (kindParam && !namespace && !name && (workloadKinds as readonly string[]).includes(kindParam) ? kindParam : ''), [kindParam, namespace, name])
  const [draft, setDraft] = useState<WorkloadListState>(() => ({ ...workloadsStateFromParams(params), kind: kindPreset }))
  const [applied, setApplied] = useState<WorkloadListState>(() => ({ ...workloadsStateFromParams(params), kind: kindPreset }))
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
    navigate(kindPreset ? `/workloads/kind/${kindPreset}` : '/workloads')
  }

  return (
    <ResourcePage
      title="Workloads"
      description="Deployments, StatefulSets, DaemonSets, Jobs and CronJobs in the active scope."
      actions={selection ? <ResourceLiveUpdates key={`workloads/${selection.generation}`} generation={selection.generation} topics={['workloads']} queryKeys={[["resources", "workloads"]]} /> : null}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={() => { setApplied(draft); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'workloads'] })} onClear={() => { setDraft({ ...defaultWorkloadList, kind: kindPreset }); setApplied({ ...defaultWorkloadList, kind: kindPreset }); setCursor(''); closeDetail() }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('kind', 'Kind', applied.kind), ...activeFilter('status', 'Status', applied.workloadStatus),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={workloadSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <NamespaceFilterInput value={draft.namespace} onChange={(value) => setDraft((current) => ({ ...current, namespace: value }))} />
        <label>Kind<Select value={draft.kind} onChange={(event) => setDraft((current) => ({ ...current, kind: event.target.value }))}><option value="">All kinds</option><option value="deployments">Deployments</option><option value="replicasets">ReplicaSets</option><option value="statefulsets">StatefulSets</option><option value="daemonsets">DaemonSets</option><option value="jobs">Jobs</option><option value="cronjobs">CronJobs</option></Select></label>
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
          <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
            <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
              <DataTable
                caption="Authorized workload page"
                rows={list.data?.items ?? []}
                getRowKey={(item) => `${item.kind}/${item.namespace}/${item.name}`}
                columns={[
                  { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
                  { key: 'name', header: 'Kind / name', cell: (item) => <TableLink aria-label={`Open ${item.kind} ${item.name} in ${item.namespace}`} onClick={() => { requests.abortAll(); setSelected({ generation: selection!.generation, item }); yaml.reset(); navigate(`/workloads/${workloadKindPath(item.kind)}/${encodeURIComponent(item.namespace)}/${encodeURIComponent(item.name)}`) }} primary={item.name} secondary={item.kind} /> },
                  { key: 'ready', header: 'Ready', cell: (item) => `${item.ready ?? '—'} / ${item.desired ?? '—'}` },
                  { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
                  { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
                ]}
              />
              {list.data ? <CollectionFooter result={list.data} onNext={(next) => { setCursor(next); closeDetail() }} onRestart={() => { setCursor(''); closeDetail() }} /> : null}
            </div>
            <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={<span className="flex items-center gap-2">{detailTitle(activeSelected ? `${activeSelected.kind} ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}{activeSelected && activeSelected.kind !== 'ReplicaSet' ? <FavoriteButton kind={({ Deployment: 'deployment', StatefulSet: 'statefulset', DaemonSet: 'daemonset', Job: 'job', CronJob: 'cronjob' } as const)[activeSelected.kind]} namespace={activeSelected.namespace} name={activeSelected.name} generation={selection?.generation} label={activeSelected.kind} /> : null}</span>}>
              {activeSelected ? <>
                {detail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading detail…</p> : detail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(detail.error)}</p> : detail.data ? <>
                  <Facts facts={[
                    { label: 'Status', value: detail.data.status },
                    { label: 'Resource version', value: detail.data.metadata.resourceVersion },
                    { label: 'Containers', value: detail.data.containers.map((value) => value.name).join(', ') || 'none' },
                    { label: 'Labels', value: Object.entries(detail.data.metadata.labels ?? {}).map(([label, value]) => `${label}=${value}`).join(', ') || 'none' },
                  ]} />
                  <WorkloadActions key={`${selection!.generation}/${detail.data.metadata.uid}`} detail={detail.data} selection={selection as SelectionSummary} />
                </> : null}
                <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} diffTarget={activeSelected ? { collection: workloadKindPath(activeSelected.kind), namespace: activeSelected.namespace, name: activeSelected.name, generation: selection?.generation } : undefined} />
              </> : null}
            </Drawer>
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}

export function PodsPage() {
  const { status, selection } = useActiveSelection()
  const { namespace, name } = useParams<{ namespace: string; name: string }>()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const paramItem = useMemo(() => {
    if (!namespace || !name) return null
    return podFromParams(namespace, name)
  }, [namespace, name])
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
  // V5-11: Pod metrics render only when the Metrics API is healthy; absence,
  // denial or partial coverage touches this block alone, never the Pod view.
  const metricsAvailable = status.data?.components.metrics.status === 'healthy'
  const metrics = useQuery({ queryKey: ['pod-metrics', selection?.generation], queryFn: ({ signal }) => getDashboardMetrics(signal, selection?.generation), enabled: Boolean(selection && activeSelected && metricsAvailable), staleTime: 30_000 })
  const podMetric = metrics.data?.block.value.pods.find((value) => value.namespace === activeSelected?.namespace && value.pod === activeSelected?.name)
  const yaml = useMutation({ mutationFn: ({ namespace, name }: Pod) => requests.run((signal) => getPodYAML(namespace, name, signal)) })

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
    navigate('/pods')
  }

  return (
    <ResourcePage
      title="Pods"
      description="Pod inventory with readiness, restarts, owner and problem evidence in the active scope."
      actions={<div className="flex items-center gap-2"><Link to="/logs"><Button variant="secondary" size="md"><ScrollText size={14} aria-hidden="true" /> Open logs</Button></Link>{selection ? <ResourceLiveUpdates key={`pods/${selection.generation}`} generation={selection.generation} topics={['pods']} queryKeys={[["resources", "pods"]]} /> : null}</div>}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={() => { setApplied(draft); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'pods'] })} onClear={() => { setDraft({ ...defaultPodList }); setApplied({ ...defaultPodList }); setCursor(''); closeDetail() }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('workload', 'Workload owner', applied.workload), ...activeFilter('node', 'Node', applied.node), ...activeFilter('status', 'Status', applied.podStatus), ...activeFilter('restarts', 'Restarts', applied.restarts === 'any' ? '' : applied.restarts), ...activeFilter('problematic', 'Problem evidence', applied.problematic === 'true' ? 'problematic only' : applied.problematic === 'false' ? 'without evidence' : ''),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={podSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <NamespaceFilterInput value={draft.namespace} onChange={(value) => setDraft((current) => ({ ...current, namespace: value }))} />
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
          <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
            <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
              <DataTable
                caption="Authorized Pod page"
                rows={list.data?.items ?? []}
                getRowKey={(item) => `${item.namespace}/${item.name}`}
                columns={[
                  { key: 'namespace', header: 'Namespace / Pod', cell: (item) => <TableLink aria-label={`Open Pod ${item.name} in ${item.namespace}`} onClick={() => { requests.abortAll(); setSelected({ generation: selection!.generation, item }); yaml.reset(); navigate(`/pods/${encodeURIComponent(item.namespace)}/${encodeURIComponent(item.name)}`) }} primary={<>{item.name}{item.problematic ? <Badge variant="danger" className="ml-2">problem</Badge> : null}</>} secondary={item.namespace} /> },
                  { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
                  { key: 'ready', header: 'Ready', cell: (item) => `${item.ready.current}/${item.ready.desired}` },
                  { key: 'restarts', header: 'Restarts', cell: (item) => item.restarts },
                  { key: 'node', header: 'Node', cell: (item) => item.node ?? '—' },
                  { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
                ]}
              />
              {list.data ? <CollectionFooter result={list.data} onNext={(next) => { setCursor(next); closeDetail() }} onRestart={() => { setCursor(''); closeDetail() }} /> : null}
            </div>
            <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={<span className="flex items-center gap-2">{detailTitle(activeSelected ? `Pod ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}{activeSelected ? <FavoriteButton kind="pod" namespace={activeSelected.namespace} name={activeSelected.name} generation={selection?.generation} label="Pod" /> : null}</span>}>
              {activeSelected ? <>
                {detail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading detail…</p> : detail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(detail.error)}</p> : detail.data ? <>
                  <Facts facts={[
                    { label: 'UID', value: detail.data.metadata.uid },
                    { label: 'Owner', value: detail.data.summary.owner ? `${detail.data.summary.owner.kind}/${detail.data.summary.owner.name}` : 'standalone' },
                    { label: 'Containers', value: detail.data.containers.map((value) => `${value.spec.name} (${value.state})`).join(', ') || 'none' },
                    { label: 'IP', value: detail.data.summary.ip ?? '—' },
                  ]} />
                  <div className="mt-3">
                    <Link to={`/logs?namespace=${encodeURIComponent(activeSelected.namespace)}&pod=${encodeURIComponent(activeSelected.name)}&container=${encodeURIComponent(detail.data.containers[0]?.spec.name ?? '')}`}><Button variant="secondary" size="sm">View logs</Button></Link>
                  </div>
                  <PodActions key={`${selection!.generation}/${detail.data.metadata.uid}`} detail={detail.data} selection={selection as SelectionSummary} />
                </> : null}
                {activeSelected ? (
                  metricsAvailable ? (
                    podMetric ? (
                      <p className="mt-3 rounded-r-md border-l-2 border-kp-blue-border bg-kp-blue-bg px-3 py-2 text-xs text-kp-subtext" role="status">
                        CPU {podMetric.cpuMillicores} m · memory {Math.round(podMetric.memoryBytes / (1024 * 1024))} MiB
                        {metrics.data?.block.truncated ? ' · metrics truncated' : ''}
                      </p>
                    ) : metrics.isPending ? null : (
                      <p className="mt-3 text-xs text-kp-overlay-text" role="note">No metrics sample for this Pod in the current window.</p>
                    )
                  ) : (
                    <p className="mt-3 text-xs text-kp-overlay-text" role="note">Metrics API unavailable; Pod metrics are not collected.</p>
                  )
                ) : null}
                <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} diffTarget={activeSelected ? { collection: 'pods', namespace: activeSelected.namespace, name: activeSelected.name, generation: selection?.generation } : undefined} />
              </> : null}
            </Drawer>
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
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
    <ResourcePage
      title="Events"
      description="Kubernetes events ordered within the bounded page; type, source and count are preserved."
      actions={selection ? <ResourceLiveUpdates key={`events/${selection.generation}`} generation={selection.generation} topics={['events']} queryKeys={[["resources", "events"]]} /> : null}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={() => { setApplied(draft); setCursor('') }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'events'] })} onClear={() => { setDraft({ ...defaultEventList }); setApplied({ ...defaultEventList }); setCursor('') }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('type', 'Type', applied.eventType), ...activeFilter('objectKind', 'Object kind', applied.objectKind), ...activeFilter('reason', 'Reason', applied.reason),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="timestamp" defaultOrder="desc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={eventSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <NamespaceFilterInput value={draft.namespace} onChange={(value) => setDraft((current) => ({ ...current, namespace: value }))} />
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
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}>
          <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
            <DataTable
              caption="Authorized event page"
              rows={list.data?.items ?? []}
              getRowKey={(item, index) => `${item.namespace}/${item.objectKind}/${item.objectName}/${item.timestamp ?? index}`}
              columns={[
                { key: 'time', header: 'Time', cell: (item) => dateTime(item.timestamp) },
                { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
                { key: 'object', header: 'Object', cell: (item) => `${item.objectKind}/${item.objectName}` },
                { key: 'type', header: 'Type / reason', cell: (item) => <><Badge variant={eventBadgeVariant(item.type)}>{item.type}</Badge><small className="mt-0.5 block text-xs text-kp-overlay-text">{item.reason}</small></> },
                { key: 'count', header: 'Count', cell: (item) => item.count },
                { key: 'message', header: 'Message', cell: (item) => <span className="block max-w-[480px] break-words text-sm leading-snug">{item.message}</span> },
              ]}
            />
            {list.data ? <CollectionFooter result={list.data} onNext={setCursor} onRestart={() => setCursor('')} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}

type NetworkTab = 'services' | 'endpoints' | 'ingresses' | 'ingress-classes' | 'endpoint-slices' | 'network-policies' | 'port-forwards'
type NetworkResourceTab = Exclude<NetworkTab, 'port-forwards'>
type NetworkItem = ServiceResource | IngressResource | EndpointSliceResource | Endpoints | IngressClass | NetworkPolicy
type NetworkSelection = { generation: string; tab: NetworkResourceTab; namespace: string; name: string }

interface SimpleListState {
  search: string
  sort: string
  order: ListSortOrder
}

const defaultSimpleList: SimpleListState = { search: '', sort: 'identity', order: 'asc' }
const defaultNetworkLists: Record<NetworkResourceTab, SimpleListState> = {
  services: { ...defaultSimpleList },
  endpoints: { ...defaultSimpleList },
  ingresses: { ...defaultSimpleList },
  'ingress-classes': { ...defaultSimpleList },
  'endpoint-slices': { ...defaultSimpleList },
  'network-policies': { ...defaultSimpleList },
}
const defaultNetworkCursors: Record<NetworkResourceTab, string> = { services: '', endpoints: '', ingresses: '', 'ingress-classes': '', 'endpoint-slices': '', 'network-policies': '' }

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
  endpoints: [
    { value: 'identity', label: 'Namespace and name' },
    { value: 'name', label: 'Name' },
  ],
  'ingress-classes': [
    { value: 'identity', label: 'Name' },
    { value: 'name', label: 'Name (natural)' },
  ],
  'network-policies': [
    { value: 'identity', label: 'Namespace and name' },
    { value: 'name', label: 'Name' },
  ],
}

function NetworkDetailView({ tab, service, ingress, slice, endpoints, ingressClass, policy }: { tab: NetworkSelection['tab']; service?: ServiceDetail; ingress?: IngressDetail; slice?: EndpointSliceDetail; endpoints?: Endpoints; ingressClass?: IngressClass; policy?: NetworkPolicy }) {
  if (tab === 'endpoints' && endpoints) return <Facts facts={[
    { label: 'Ready addresses', value: String(endpoints.readyCount) },
    { label: 'Not ready', value: String(endpoints.notReadyCount) },
    { label: 'Ports', value: endpoints.ports.join(', ') || 'none' },
    { label: 'Truncated', value: endpoints.truncated ? 'yes — address list exceeds the bounded window' : 'no' },
  ]} />
  if (tab === 'ingress-classes' && ingressClass) return <Facts facts={[
    { label: 'Controller', value: ingressClass.controller },
    { label: 'Default', value: ingressClass.default ? 'yes' : 'no' },
    { label: 'Parameters', value: ingressClass.parameters ?? 'none' },
    { label: 'Age', value: age(ingressClass.ageSeconds) },
  ]} />
  if (tab === 'network-policies' && policy) return <Facts facts={[
    { label: 'Pod selector', value: policy.podSelector || 'none' },
    { label: 'Policy types', value: policy.policyTypes.join(', ') || 'unknown' },
    { label: 'Rules', value: policy.ruleSummary.length ? policy.ruleSummary.join(', ') : 'none declared' },
    { label: 'Age', value: age(policy.ageSeconds) },
  ]} />
  if (tab === 'services' && service) return <Facts facts={[
    { label: 'Type', value: service.summary.type },
    { label: 'Cluster IPs', value: service.summary.clusterIPs.join(', ') || 'none' },
    { label: 'Session affinity', value: service.sessionAffinity },
    { label: 'Ports', value: service.summary.ports.map((port) => `${port.port}/${port.protocol}`).join(', ') || 'none' },
  ]} />
  if (tab === 'ingresses' && ingress) return <Facts facts={[
    { label: 'Class', value: ingress.summary.className ?? 'default' },
    { label: 'Hosts', value: ingress.summary.hosts.join(', ') || 'none' },
    { label: 'Paths', value: ingress.summary.paths.map((path) => `${path.host}${path.path} → ${path.backend.serviceName}`).join(', ') || 'none' },
    { label: 'Load balancers', value: ingress.loadBalancerAddresses.join(', ') || 'none' },
  ]} />
  if (tab === 'endpoint-slices' && slice) return <Facts facts={[
    { label: 'Address type', value: slice.summary.addressType },
    { label: 'Endpoints', value: slice.summary.endpoints.length },
    { label: 'Addresses', value: slice.summary.endpoints.flatMap((endpoint) => endpoint.addresses).join(', ') || 'none' },
    { label: 'Ports', value: slice.summary.ports.map((port) => port.port ?? 'named').join(', ') || 'none' },
  ]} />
  return null
}

function networkTabFromParams(tab: string): NetworkResourceTab | null {
  return tab === 'services' || tab === 'endpoints' || tab === 'ingresses' || tab === 'ingress-classes' || tab === 'endpoint-slices' || tab === 'network-policies' ? tab : null
}

export function NetworkPage() {
  const { status, selection } = useActiveSelection()
  const { tab: tabParam, namespace, name } = useParams<{ tab: string; namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const [tabState, setTab] = useState<NetworkTab>(() => networkTabFromParams(tabParam ?? '') ?? 'services')
  const tab = useMemo(() => networkTabFromParams(tabParam ?? '') ?? tabState, [tabParam, tabState])
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
  const endpoints = useQuery({ queryKey: ['resources', 'endpoints', selection?.generation, appliedLists['endpoints'], cursors['endpoints']], queryFn: ({ signal }) => getEndpointsList(networkOptions('endpoints'), signal, selection?.generation), enabled: Boolean(selection && tab === 'endpoints') })
  const ingressClasses = useQuery({ queryKey: ['resources', 'ingress-classes', selection?.generation, appliedLists['ingress-classes'], cursors['ingress-classes']], queryFn: ({ signal }) => getIngressClasses(networkOptions('ingress-classes'), signal, selection?.generation), enabled: Boolean(selection && tab === 'ingress-classes') })
  const networkPolicies = useQuery({ queryKey: ['resources', 'network-policies', selection?.generation, appliedLists['network-policies'], cursors['network-policies']], queryFn: ({ signal }) => getNetworkPolicies(networkOptions('network-policies'), signal, selection?.generation), enabled: Boolean(selection && tab === 'network-policies') })
  const forwards = useQuery({ queryKey: ['port-forwards', selection?.generation], queryFn: ({ signal }) => getPortForwards(signal, selection!.generation), enabled: Boolean(selection && tab === 'port-forwards'), refetchInterval: 10_000 })
  const serviceDetail = useQuery({ queryKey: ['resources', 'service-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getService(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: activeSelected?.tab === 'services' })
  const ingressDetail = useQuery({ queryKey: ['resources', 'ingress-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getIngress(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: activeSelected?.tab === 'ingresses' })
  const sliceDetail = useQuery({ queryKey: ['resources', 'slice-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getEndpointSlice(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: activeSelected?.tab === 'endpoint-slices' })
  const endpointsDetail = useQuery({ queryKey: ['resources', 'endpoints-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getEndpointsItem(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: activeSelected?.tab === 'endpoints' })
  const ingressClassDetail = useQuery({ queryKey: ['resources', 'ingressclass-detail', selection?.generation, activeSelected?.name], queryFn: ({ signal }) => getIngressClass(activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && activeSelected.tab === 'ingress-classes') })
  const networkPolicyDetail = useQuery({ queryKey: ['resources', 'networkpolicy-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getNetworkPolicy(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: activeSelected?.tab === 'network-policies' })
  const yaml = useMutation({ mutationFn: (value: NetworkSelection) => requests.run((signal) => value.tab === 'services' ? getServiceYAML(value.namespace, value.name, signal) : value.tab === 'ingresses' ? getIngressYAML(value.namespace, value.name, signal) : getEndpointSliceYAML(value.namespace, value.name, signal)) })
  const close = useMutation({ mutationFn: (id: string) => requests.run(async (signal) => { const session = await getSession(signal); if (session.generation !== selection!.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' }); return closePortForward(id, selection!.generation, session.csrfToken, signal) }), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['port-forwards'] }) })
  const [stopAllState, setStopAllState] = useState<'idle' | 'confirm'>('idle')
  const [stopAllResult, setStopAllResult] = useState<{ closed: number; failed: number } | null>(null)
  const stopAll = useMutation({ mutationFn: async () => {
    const active = (forwards.data ?? []).filter((item) => item.status === 'active')
    let closed = 0
    let failed = 0
    for (const session of active) {
      try {
        await requests.run(async (signal) => {
          const sessionData = await getSession(signal)
          if (sessionData.generation !== selection!.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
          return closePortForward(session.id, selection!.generation, sessionData.csrfToken, signal)
        })
        closed += 1
      } catch {
        failed += 1
      }
    }
    return { closed, failed }
  }, onSuccess: (result) => { setStopAllResult(result); setStopAllState('idle'); queryClient.invalidateQueries({ queryKey: ['port-forwards'] }) } })

  const active: CollectionResult<NetworkItem> | undefined = tab === 'services' && services.data ? { ...services.data, items: services.data.items } : tab === 'ingresses' && ingresses.data ? { ...ingresses.data, items: ingresses.data.items } : tab === 'endpoint-slices' && slices.data ? { ...slices.data, items: slices.data.items } : tab === 'endpoints' && endpoints.data ? { ...endpoints.data, items: endpoints.data.items } : tab === 'ingress-classes' && ingressClasses.data ? { ...ingressClasses.data, items: ingressClasses.data.items } : tab === 'network-policies' && networkPolicies.data ? { ...networkPolicies.data, items: networkPolicies.data.items } : undefined
  const activeQuery = tab === 'services' ? services : tab === 'ingresses' ? ingresses : tab === 'endpoint-slices' ? slices : tab === 'endpoints' ? endpoints : tab === 'ingress-classes' ? ingressClasses : networkPolicies
  const currentDetail = activeSelected?.tab === 'services' ? serviceDetail : activeSelected?.tab === 'ingresses' ? ingressDetail : activeSelected?.tab === 'endpoint-slices' ? sliceDetail : activeSelected?.tab === 'endpoints' ? endpointsDetail : activeSelected?.tab === 'ingress-classes' ? ingressClassDetail : networkPolicyDetail

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
    navigate(tab === 'port-forwards' ? '/network/port-forwards' : `/network/${tab}`)
  }

  function setCursor(value: string) {
    if (tab === 'port-forwards') return
    setCursorValue(tab, value)
    closeDetail()
  }

  return (
    <ResourcePage title="Network" description="Services, Ingresses, EndpointSlices and loopback-only port-forward sessions.">
      <ResourceTabStrip ariaLabel="Network resource type" panelId="network-panel" active={tab} onChange={(value) => { setTab(value as NetworkTab); closeDetail() }} tabs={[
        { id: 'services', label: 'services' },
        { id: 'endpoints', label: 'endpoints' },
        { id: 'ingresses', label: 'ingresses' },
        { id: 'ingress-classes', label: 'ingress-classes' },
        { id: 'endpoint-slices', label: 'endpoint-slices' },
        { id: 'network-policies', label: 'network-policies' },
        { id: 'port-forwards', label: 'port-forwards' },
      ]} />
      {selection && (tab === 'services' || tab === 'ingresses' || tab === 'endpoint-slices') ? <ResourceLiveUpdates key={`${tab}/${selection.generation}`} generation={selection.generation} topics={[tab]} queryKeys={[["resources", tab]]} /> : null}
      {tab !== 'port-forwards' ? <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], search: value } }))} onApply={() => { setAppliedLists((current) => ({ ...current, [resourceTab]: { ...draft } })); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} onClear={() => { setDrafts((current) => ({ ...current, [resourceTab]: { ...defaultSimpleList } })); setAppliedLists((current) => ({ ...current, [resourceTab]: { ...defaultSimpleList } })); setCursor(''); closeDetail() }} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={networkSortOptions[resourceTab]} onSortChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], sort: value } }))} onOrderChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], order: value } }))} /> : null}
      <div id="network-panel" role="tabpanel">
        <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
          {tab === 'port-forwards' ? <QueryState pending={forwards.isPending} error={forwards.error ?? close.error} empty={forwards.data?.length === 0}>
            {stopAllState !== 'idle' ? (
              <div role="alertdialog" aria-labelledby="stop-all-title" className="grid gap-2 rounded-lg border border-kp-red-border bg-kp-red-bg/50 p-3.5">
                <strong id="stop-all-title" className="text-sm text-kp-text">Close all active loopback sessions?</strong>
                <p className="m-0 text-xs text-kp-subtext">Only sessions of the current selection are closed; each close re-verifies authorization and reports per session.</p>
                <div className="flex gap-2">
                  <Button variant="secondary" size="sm" onClick={() => setStopAllState('idle')}>Cancel</Button>
                  <Button variant="danger" size="sm" disabled={stopAll.isPending} onClick={() => stopAll.mutate()}>{stopAll.isPending ? 'Closing…' : 'Confirm close all'}</Button>
                </div>
              </div>
            ) : null}
            {stopAllResult ? <p className={stopAllResult.failed === 0 ? 'm-0 text-xs text-kp-green' : 'm-0 text-xs text-kp-yellow'} role="status">{stopAllResult.closed} session{stopAllResult.closed === 1 ? '' : 's'} closed{stopAllResult.failed ? ` · ${stopAllResult.failed} failed` : ''}.</p> : null}
            {forwards.data?.some((item) => item.status === 'active') && stopAllState === 'idle' ? (
              <Button variant="danger" size="sm" className="justify-self-start" onClick={() => setStopAllState('confirm')}>Stop all active sessions</Button>
            ) : null}
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {forwards.data?.map((item) => (
                <article key={item.id} className="grid content-start gap-1.5 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-3.5">
                  <strong className="mono text-base text-kp-text">{item.localAddress}:{item.localPort}</strong>
                  <span className="text-sm text-kp-subtext break-words">{item.context} · {item.namespace}/{item.pod} → {item.remotePort}</span>
                  <small className="text-xs text-kp-overlay-text">{item.status} · created {dateTime(item.createdAt)} · expires {dateTime(item.expiresAt)}</small>
                  {item.endedAt ? <small className="text-xs text-kp-overlay-text">ended {dateTime(item.endedAt)} · {item.endReason ?? item.status}</small> : null}
                  {item.status === 'active' ? <Button variant="danger" size="sm" className="mt-1 justify-self-start" onClick={() => close.mutate(item.id)}>Close loopback session</Button> : null}
                </article>
              ))}
            </div>
          </QueryState> : <QueryState pending={activeQuery.isPending} error={activeQuery.error} empty={active?.items.length === 0}>
            <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
              <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
                <DataTable
                  caption={`Authorized ${tab} page`}
                  rows={active?.items ?? []}
                  getRowKey={(item) => `${'namespace' in item ? `${item.namespace}/` : ''}${item.name}`}
                  columns={[
                    { key: 'namespace', header: 'Name', cell: (item) => <TableLink aria-label={`Open ${tab} ${item.name}${'namespace' in item ? ` in ${item.namespace}` : ''}`} onClick={() => { closeDetail(); setSelected({ generation: selection!.generation, tab: tab as NetworkSelection['tab'], namespace: 'namespace' in item ? item.namespace : '', name: item.name }); navigate(`/network/${tab}/${'namespace' in item ? `${encodeURIComponent(item.namespace)}/` : ''}${encodeURIComponent(item.name)}`) }} primary={item.name} secondary={'namespace' in item ? item.namespace : 'cluster'} /> },
                    { key: 'type', header: 'Type', cell: (item) => ('type' in item ? item.type : 'className' in item ? (item.className ?? 'Ingress') : 'addressType' in item ? item.addressType : 'podSelector' in item ? item.podSelector || 'none' : 'controller' in item ? item.controller : '—') },
                    { key: 'summary', header: 'Summary', cell: (item) => ('clusterIPs' in item ? item.clusterIPs.join(', ') : 'hosts' in item ? item.hosts.join(', ') : 'addressType' in item ? `${item.endpoints.length} endpoints` : 'readyCount' in item ? `${item.readyCount} ready / ${item.notReadyCount} not ready${item.truncated ? ' (truncated)' : ''}` : 'ruleSummary' in item ? item.ruleSummary.length + ' rules' : item.default ? 'default class' : '—') },
                  ]}
                />
                {active ? <CollectionFooter result={active} onNext={setCursor} onRestart={() => setCursor('')} /> : null}
              </div>
              <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={<span className="flex items-center gap-2">{detailTitle(activeSelected ? `${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}{activeSelected && (activeSelected.tab === 'services' || activeSelected.tab === 'ingresses' || activeSelected.tab === 'endpoint-slices') ? <FavoriteButton kind={({ services: 'service', ingresses: 'ingress', 'endpoint-slices': 'endpointslice' } as const)[activeSelected.tab]} namespace={activeSelected.namespace} name={activeSelected.name} generation={selection?.generation} label={activeSelected.tab} /> : null}</span>}>
                {activeSelected ? <>
                  {currentDetail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading detail…</p> : currentDetail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(currentDetail.error)}</p> : <NetworkDetailView tab={activeSelected.tab} service={serviceDetail.data} ingress={ingressDetail.data} slice={sliceDetail.data} endpoints={endpointsDetail.data} ingressClass={ingressClassDetail.data} policy={networkPolicyDetail.data} />}
                  {activeSelected.tab === 'services' || activeSelected.tab === 'ingresses' || activeSelected.tab === 'endpoint-slices' ? <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} diffTarget={activeSelected ? { collection: activeSelected.tab, namespace: activeSelected.namespace, name: activeSelected.name, generation: selection?.generation } : undefined} /> : <p className="mt-3 text-xs text-kp-overlay-text" role="note">YAML is not offered for this resource type.</p>}
                </> : null}
              </Drawer>
            </div>
          </QueryState>}
        </SelectionGate>
      </div>
    </ResourcePage>
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
  return <>
    <Facts facts={[
      { label: 'Resource version', value: detail.metadata.resourceVersion },
      { label: 'Total bytes', value: detail.totalBytes },
      { label: 'Entries', value: detail.entries.length },
      { label: 'Truncated', value: detail.truncated ? 'yes' : 'no' },
    ]} />
    <div className="mt-3 grid gap-2">
      {detail.entries.map((entry) => (
        <details key={entry.key} className="rounded-lg border border-kp-overlay-0 bg-kp-surface-1">
          <summary className="cursor-pointer px-2.5 py-2 text-sm text-kp-sky">{entry.key} · {entry.encoding}{entry.truncated ? ' · truncated' : ''}</summary>
          <pre className="max-h-[260px] overflow-auto border-t border-kp-overlay-0 px-2.5 py-2 text-xs leading-relaxed text-kp-subtext break-words whitespace-pre-wrap">{entry.value}</pre>
        </details>
      ))}
    </div>
  </>
}

function SecretMetadataView({ secret }: { secret: SecretMetadata }) {
  return <>
    <Facts facts={[
      { label: 'API version', value: secret.apiVersion },
      { label: 'Kind', value: secret.kind },
      { label: 'UID', value: secret.metadata.uid },
      { label: 'Created', value: dateTime(secret.metadata.creationTimestamp) },
      ...(secret.metadata.deletionTimestamp ? [{ label: 'Deleting', value: dateTime(secret.metadata.deletionTimestamp) }] : []),
    ]} />
    <p className="mt-3 rounded-r-md border-l-2 border-kp-yellow-border bg-kp-yellow-bg px-3 py-2 text-sm text-kp-yellow">Secret values, annotations, managed fields and YAML are intentionally unavailable.</p>
  </>
}

export function ConfigPage() {
  const { status, selection } = useActiveSelection()
  const { tab: tabParam, namespace, name } = useParams<{ tab: string; namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const [tabState, setTab] = useState<ConfigTab>(() => configTabFromParams(tabParam ?? '') ?? 'configmaps')
  const tab = useMemo(() => configTabFromParams(tabParam ?? '') ?? tabState, [tabParam, tabState])
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
    navigate(`/config/${tab}`)
  }

  function setCursor(value: string) {
    setCursorValue(tab, value)
    closeDetail()
  }

  return (
    <ResourcePage title="Configuration" description="ConfigMaps are fetched on detail; Secrets remain metadata-only and never expose values or YAML.">
      <ResourceTabStrip ariaLabel="Configuration resource type" panelId="config-panel" active={tab} onChange={(value) => { setTab(value as ConfigTab); closeDetail() }} tabs={[
        { id: 'configmaps', label: 'configmaps' },
        { id: 'secrets', label: 'secrets' },
      ]} />
      {selection && tab === 'configmaps' ? <ResourceLiveUpdates key={`configmaps/${selection.generation}`} generation={selection.generation} topics={['configmaps']} queryKeys={[["resources", "configmaps"]]} /> : null}
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], search: value } }))} onApply={() => { setAppliedLists((current) => ({ ...current, [tab]: { ...draft } })); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} onClear={() => { setDrafts((current) => ({ ...current, [tab]: { ...defaultSimpleList } })); setAppliedLists((current) => ({ ...current, [tab]: { ...defaultSimpleList } })); setCursor(''); closeDetail() }} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={configSortOptions} onSortChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], sort: value } }))} onOrderChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], order: value } }))} />
      <div id="config-panel" role="tabpanel">
        <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
          <QueryState pending={activeQuery.isPending} error={activeQuery.error} empty={active?.items.length === 0}>
            <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
              <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
                <DataTable
                  caption={`Authorized ${tab} metadata page`}
                  rows={active?.items ?? []}
                  getRowKey={(item) => { const value = 'metadata' in item ? item.metadata : item; return `${value.namespace}/${value.name}` }}
                  columns={[
                    { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return <TableLink aria-label={`Open ${tab === 'secrets' ? 'Secret' : 'ConfigMap'} ${value.name} in ${value.namespace}`} onClick={() => { closeDetail(); setSelected({ generation: selection!.generation, namespace: value.namespace, name: value.name }); navigate(`/config/${tab}/${encodeURIComponent(value.namespace)}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.namespace} /> } },
                    { key: 'uid', header: 'UID', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return <span className="mono text-xs">{value.uid}</span> } },
                    { key: 'created', header: 'Created', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return dateTime(value.creationTimestamp) } },
                  ]}
                />
                {active ? <CollectionFooter result={active} onNext={setCursor} onRestart={() => setCursor('')} /> : null}
              </div>
              <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={<span className="flex items-center gap-2">{detailTitle(activeSelected ? `${tab === 'secrets' ? 'Secret metadata' : 'ConfigMap'} ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}{activeSelected ? <FavoriteButton kind={tab === 'secrets' ? 'secret' : 'configmap'} namespace={activeSelected.namespace} name={activeSelected.name} generation={selection?.generation} label={tab === 'secrets' ? 'Secret' : 'ConfigMap'} /> : null}</span>}>
                {activeSelected ? <>
                  {tab === 'configmaps' ? configDetail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading authorized entries…</p> : configDetail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(configDetail.error)}</p> : configDetail.data ? <ConfigMapDetailView detail={configDetail.data} /> : null : secretDetail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading metadata…</p> : secretDetail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(secretDetail.error)}</p> : secretDetail.data ? <SecretMetadataView secret={secretDetail.data} /> : null}
                  {tab === 'configmaps' ? <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} diffTarget={activeSelected ? { collection: 'configmaps', namespace: activeSelected.namespace, name: activeSelected.name, generation: selection?.generation } : undefined} /> : null}
                </> : null}
              </Drawer>
            </div>
          </QueryState>
        </SelectionGate>
      </div>
    </ResourcePage>
  )
}

interface NodeListState {
  search: string
  nodeStatus: string
  sort: string
  order: ListSortOrder
}

const nodeStatuses = ['Ready', 'NotReady', 'Unknown'] as const
const defaultNodeList: NodeListState = { search: '', nodeStatus: '', sort: 'identity', order: 'asc' }
const nodeSortOptions: readonly ListSortOption[] = [
  { value: 'identity', label: 'Name' },
  { value: 'name', label: 'Name (natural)' },
  { value: 'age', label: 'Age' },
  { value: 'status', label: 'Status' },
]

function nodeStateFromParams(params: URLSearchParams): NodeListState {
  return {
    ...defaultNodeList,
    search: paramValue(params, 'search'),
    nodeStatus: listedValue(paramValue(params, 'status'), nodeStatuses),
  }
}

// NodesPage is the cluster-scoped reference family (F1/ADR 0006): a selected
// context is enough; no namespace scope and no namespace filter exist here.
export function NodesPage() {
  const { status, selection } = useActiveSelection()
  const { name } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [draft, setDraft] = useState<NodeListState>(() => nodeStateFromParams(params))
  const [applied, setApplied] = useState<NodeListState>(() => nodeStateFromParams(params))
  const [cursor, setCursor] = useGenerationCursor(selection?.generation)
  const [selected, setSelected] = useState<GenerationSelection<NodeSummary> | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const paramItem = useMemo(() => (name ? { name, status: 'Unknown', ready: false, roles: [], kubeletVersion: '', internalIP: null, ageSeconds: 0 } : null), [name])
  const activeSelected = useMemo(() => {
    if (paramItem) return paramItem
    if (selected && selected.generation === selection?.generation) return selected.item
    return null
  }, [paramItem, selected, selection?.generation])
  const list = useQuery({
    queryKey: ['resources', 'nodes', selection?.generation, applied, cursor],
    queryFn: ({ signal }) => getNodes({ limit: 100, search: applied.search || undefined, statuses: applied.nodeStatus ? [applied.nodeStatus] : undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, selection?.generation),
    enabled: Boolean(selection),
  })
  const detail = useQuery({
    queryKey: ['resources', 'node-detail', selection?.generation, activeSelected?.name],
    queryFn: ({ signal }) => getNode(activeSelected!.name, signal, selection!.generation),
    enabled: Boolean(selection && activeSelected),
  })
  const yaml = useMutation({ mutationFn: (value: NodeSummary) => requests.run((signal) => getNodeYAML(value.name, signal)) })

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
    navigate('/nodes')
  }

  return (
    <ResourcePage
      title="Nodes"
      description="Cluster nodes with readiness, roles, capacity and taints; a selected context is enough and no namespace filter applies."
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={() => { setApplied(draft); setCursor(''); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'nodes'] })} onClear={() => { setDraft({ ...defaultNodeList }); setApplied({ ...defaultNodeList }); setCursor(''); closeDetail() }} activeFilters={[
        ...activeFilter('status', 'Status', applied.nodeStatus),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={nodeSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <label>Status<Select value={draft.nodeStatus} onChange={(event) => setDraft((current) => ({ ...current, nodeStatus: event.target.value }))}><option value="">All statuses</option>{nodeStatuses.map((value) => <option key={value}>{value}</option>)}</Select></label>
      </ResourceListControls>
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}>
          <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
            <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
              <DataTable
                caption="Authorized node page"
                rows={list.data?.items ?? []}
                getRowKey={(item) => item.name}
                columns={[
                  { key: 'name', header: 'Node', cell: (item) => <TableLink aria-label={`Open Node ${item.name}`} onClick={() => { requests.abortAll(); setSelected({ generation: selection!.generation, item }); yaml.reset(); navigate(`/nodes/${encodeURIComponent(item.name)}`) }} primary={item.name} secondary={item.roles.join(', ') || 'no role'} /> },
                  { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
                  { key: 'version', header: 'Version', cell: (item) => item.kubeletVersion || '—' },
                  { key: 'internal-ip', header: 'Internal IP', cell: (item) => item.internalIP ?? '—' },
                  { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
                ]}
              />
              {list.data ? <CollectionFooter result={list.data} onNext={(next) => { setCursor(next); closeDetail() }} onRestart={() => { setCursor(''); closeDetail() }} /> : null}
            </div>
            <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={detailTitle(activeSelected ? `Node ${activeSelected.name}` : 'Resource detail')}>
              {activeSelected ? <>
                {detail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading detail…</p> : detail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(detail.error)}</p> : detail.data ? <>
                  <Facts facts={[
                    { label: 'Status', value: detail.data.status },
                    { label: 'Roles', value: detail.data.roles.join(', ') || 'none' },
                    { label: 'Kubelet', value: detail.data.kubeletVersion || 'unknown' },
                    { label: 'Internal IP', value: detail.data.internalIP ?? '—' },
                    { label: 'UID', value: detail.data.metadata.uid },
                    { label: 'Taints', value: detail.data.taints.map((taint) => `${taint.key}=${taint.value}:${taint.effect}`).join(', ') || 'none' },
                  ]} />
                  {detail.data.conditions.length ? (
                    <div className="mt-3 overflow-x-auto rounded-lg border border-kp-overlay-0">
                      <table className="w-full border-collapse text-left text-sm">
                        <thead><tr className="border-b border-kp-overlay-0 text-2xs uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-medium">Condition</th><th className="px-2.5 py-1.5 font-medium">Status</th><th className="px-2.5 py-1.5 font-medium">Since</th></tr></thead>
                        <tbody>
                          {detail.data.conditions.map((condition) => (
                            <tr key={condition.type} className="border-b border-kp-overlay-0/50 last:border-0">
                              <td className="px-2.5 py-1.5 text-kp-text">{condition.type}</td>
                              <td className="px-2.5 py-1.5"><StatusBadge variant={statusBadgeVariant(condition.status === 'True' ? 'Healthy' : condition.status === 'False' ? 'Degraded' : 'Unknown')}>{condition.status}</StatusBadge></td>
                              <td className="px-2.5 py-1.5 text-kp-subtext">{dateTime(condition.lastTransitionTime)}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : null}
                  {detail.data.capacity ? (
                    <p className="mt-3 text-xs text-kp-overlay-text">
                      Capacity: {Object.entries(detail.data.capacity).slice(0, 4).map(([key, value]) => `${key} ${value}`).join(' · ')}
                    </p>
                  ) : null}
                </> : null}
                <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} />
              </> : null}
            </Drawer>
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}
