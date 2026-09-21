import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { ScrollText, Trash2 } from 'lucide-react'

import {
  closePortForward,
  getDashboardMetrics,
  getEndpointsList,
  getEndpointSlices,
  getEvents,
  getIngressClasses,
  getIngresses,
  getNetworkPolicies,
  getNodes,
  getPod,
  getPods,
  getPortForwards,
  getServices,
  getSession,
  getStatus,
  getWorkload,
  getWorkloads,
  deletePod,
  deleteWorkload,
  APIError,
} from '../api/client'
import type {
  EndpointSliceResource,
  Endpoints,
  EventResource,
  ConfigMapResource,
  IngressClass,
  NetworkPolicy,
  IngressResource,
  Pod,
  ServiceResource,
  Workload,
} from '../api/types'
import { Badge, Button, DataTable, Input, Select, StatusBadge, type DataTableColumn } from './ui'
import { ConfirmDialog } from './ui/ConfirmDialog'
import { useToast } from './ui/Toast'
import { csrfForGeneration } from '../actions/csrf'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { bindListInteraction, listInteractionFor } from '../observability/uxMetrics'
import { ResourceListControls } from './ResourceListControls'
import type { ActiveListFilter, ListSortOrder, ListSortOption } from './ResourceListControls'
import { ResourceLiveUpdates } from './ResourceLiveUpdates'
import { SavedFilterControls } from './SavedFilterControls'
import { InfiniteCollectionFooter, QueryState, SelectionGate } from './resource/states'
import { useInfiniteCollection } from './resource/useInfiniteCollection'
import { useSelectionBoundKeys } from './resource/useSelectionBoundKeys'
import { useResourceStreamPreview } from './resource/useResourceStreamPreview'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import { TableLink } from './resource/TableLink'
import { applyColumnVisibility, ColumnVisibilityControl, usePreferenceColumnVisibility } from './resource/columns'
import { age, dateTime } from './resource/format'
import { eventBadgeVariant, statusBadgeVariant } from './resource/status'
import { workloadKindPath } from '../navigation/paths'
import { useResourceWorkspace } from './workspace/ResourceWorkspaceProvider'

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



const workloadKindTitles: Record<string, Workload['kind']> = {
  deployments: 'Deployment',
  statefulsets: 'StatefulSet',
  daemonsets: 'DaemonSet',
  jobs: 'Job',
  cronjobs: 'CronJob',
  replicasets: 'ReplicaSet',
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
    <span className="min-w-0">
      <Input aria-label="Namespace" value={value} maxLength={4096} placeholder="ns filter" className="!h-7 !w-[10rem] text-sm" onChange={(event) => onChange(event.target.value)} />
      {entries.length > maximumNamespaceFilter ? (
        <small role="note" className="mt-1 block text-xs text-kp-yellow">
          {entries.length} namespaces listed; a query accepts at most {maximumNamespaceFilter}. Narrow the filter before applying.
        </small>
      ) : null}
    </span>
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

function isPodPreview(value: unknown): value is Pod {
  if (!value || typeof value !== 'object') return false
  const pod = value as Partial<Pod>
  return typeof pod.namespace === 'string' && typeof pod.name === 'string'
    && typeof pod.status === 'string' && typeof pod.ready?.current === 'number'
    && typeof pod.ready.desired === 'number' && typeof pod.restarts === 'number'
    && typeof pod.ageSeconds === 'number' && typeof pod.problematic === 'boolean'
}
function isNamedPreview(value: unknown): value is { namespace: string; name: string } {
  if (!value || typeof value !== 'object') return false
  const item = value as { namespace?: unknown; name?: unknown }
  return typeof item.namespace === 'string' && typeof item.name === 'string'
}

function isWorkloadPreview(value: unknown): value is Workload {
  if (!isNamedPreview(value)) return false
  const item = value as Partial<Workload>
  return typeof item.kind === 'string' && typeof item.status === 'string' && typeof item.ageSeconds === 'number'
}

function isEventPreview(value: unknown): value is EventResource {
  if (!value || typeof value !== 'object') return false
  const item = value as Partial<EventResource>
  return typeof item.namespace === 'string' && typeof item.objectKind === 'string' && typeof item.objectName === 'string'
    && typeof item.type === 'string' && typeof item.reason === 'string' && typeof item.message === 'string'
    && typeof item.count === 'number' && (item.timestamp === null || typeof item.timestamp === 'string')
}

function isNetworkPreview(value: unknown): value is ServiceResource | IngressResource | EndpointSliceResource {
  if (!isNamedPreview(value)) return false
  const item = value as Partial<ServiceResource & IngressResource & EndpointSliceResource>
  return typeof item.type === 'string' && Array.isArray(item.clusterIPs)
    || Array.isArray(item.hosts) && (item.className === null || typeof item.className === 'string')
    || typeof item.addressType === 'string' && Array.isArray(item.endpoints)
}

function isConfigMapPreview(value: unknown): value is ConfigMapResource {
  if (!isNamedPreview(value)) return false
  const item = value as Partial<ConfigMapResource>
  return typeof item.uid === 'string' && typeof item.creationTimestamp === 'string'
}

const namedPreviewKey = (item: { namespace: string; name: string }) => `${item.namespace}/${item.name}`
const eventPreviewKey = (item: EventResource) => `${item.namespace}/${item.objectKind}/${item.objectName}/${item.timestamp ?? ''}/${item.reason}`
const compareEventPreview = (left: EventResource, right: EventResource) => (right.timestamp ?? '').localeCompare(left.timestamp ?? '') || eventPreviewKey(left).localeCompare(eventPreviewKey(right))
const workloadPreviewKey = (item: Workload) => `${item.kind}/${item.namespace}/${item.name}`
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

/** Bulk destructive operations: per-resource detail fetch → authorized delete. */
interface BulkOutcome {
  deleted: number
  failed: Array<{ name: string; reason: string }>
}

function mutationError(error: unknown): string {
  if (error instanceof APIError) return `${error.code}: ${error.message}`
  return error instanceof Error ? error.message : 'The action could not be completed.'
}

function BulkToolbar({ count, children }: { count: number; children: React.ReactNode }) {
  return (
    <div className="mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-kp-accent-border bg-kp-accent-bg/50 px-3 py-2" role="toolbar" aria-label="Bulk actions">
      <strong className="text-sm text-kp-text">{count} selected</strong>
      {children}
    </div>
  )
}

export function WorkloadsPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const toast = useToast()
  const { kind: kindParam, namespace: paramNamespace, name: paramName } = useParams<{ kind: string; namespace: string; name: string }>()
  const [params] = useSearchParams()
  const generation = selection?.generation
  // Sidebar deep links use /workloads/kind/:kind; the path param presets the filter.
  const kindPreset = useMemo(() => (kindParam && !paramNamespace && !paramName && (workloadKinds as readonly string[]).includes(kindParam) ? kindParam : ''), [kindParam, paramNamespace, paramName])
  const [draft, setDraft] = useState<WorkloadListState>(() => ({ ...workloadsStateFromParams(params), kind: kindPreset }))
  const [applied, setApplied] = useState<WorkloadListState>(() => ({ ...workloadsStateFromParams(params), kind: kindPreset }))
  const queryClient = useQueryClient()
  const [selectedKeys, setSelectedKeys] = useSelectionBoundKeys([selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value])
  const [bulkConfirm, setBulkConfirm] = useState(false)

  // Deep links (/workloads/:kind/:ns/:name) open in the Resource Workspace.
  useEffect(() => {
    if (!kindParam || !paramNamespace || !paramName || !generation) return
    const kind = workloadKindTitles[kindParam]
    if (!kind) return
    workspace.openFromRoute({ collection: 'workloads', kind, namespace: paramNamespace, name: paramName })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [kindParam, paramNamespace, paramName, generation])

  const workloadColumnState = usePreferenceColumnVisibility('workloads')
  const workloadColumns: DataTableColumn<Workload>[] = [
    { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
    { key: 'name', header: 'Kind / name', cell: (item) => <TableLink aria-label={`Open ${item.kind} ${item.name} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'workloads', kind: item.kind, namespace: item.namespace, name: item.name })} primary={item.name} secondary={item.kind} /> },
    { key: 'ready', header: 'Ready', cell: (item) => `${item.ready ?? '—'} / ${item.desired ?? '—'}` },
    { key: 'available', header: 'Available', cell: (item) => item.available ?? '—' },
    { key: 'updated', header: 'Updated', cell: (item) => item.updated ?? '—' },
    { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
    { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
  ]
  const rowKey = useCallback((item: Workload) => `${item.kind}/${item.namespace}/${item.name}`, [])

  const collection = useInfiniteCollection<Workload>({
    identity: ['resources', 'workloads', selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value],
    filters: applied,
    fetchPage: (cursor, signal) => getWorkloads({ limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, namespaces: effectiveNamespaces(globalNamespace.value, namespaceValues(applied.namespace)), kinds: applied.kind ? [applied.kind] : undefined, statuses: applied.workloadStatus ? [applied.workloadStatus] : undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, generation),
    enabled: Boolean(selection),
  })
  const list = collection.query
  const preview = useResourceStreamPreview<Workload>({ identity: [selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value], topic: 'workloads', namespace: globalNamespace.value, isItem: isWorkloadPreview, itemKey: workloadPreviewKey })
  const previewActive = list.isPending && !collection.authorizationFailed && sameListState(applied, defaultWorkloadList) && Boolean(preview.preview?.items.length)
  const visibleItems = previewActive ? preview.preview!.items : collection.items
  useEffect(() => { if (collection.authorizationFailed) setSelectedKeys(new Set()) }, [collection.authorizationFailed, setSelectedKeys])
  const selectedItems = useMemo(() => collection.items.filter((item) => selectedKeys.has(rowKey(item))), [collection.items, selectedKeys, rowKey])

  const bulkDelete = useMutation({
    mutationFn: async (): Promise<BulkOutcome> => {
      const csrfToken = await csrfForGeneration(generation!)
      const outcome: BulkOutcome = { deleted: 0, failed: [] }
      for (const item of selectedItems) {
        try {
          const detail = await getWorkload(workloadKindPath(item.kind)!, item.namespace, item.name, undefined, generation)
          await deleteWorkload(workloadKindPath(item.kind)!, item.namespace, item.name, {
            confirmed: true,
            action: 'deleteWorkload',
            consequenceCode: 'DELETE_RESOURCE',
            target: { clusterProfileId: selection!.clusterProfileId, context: selection!.context, namespace: item.namespace, kind: item.kind, name: item.name },
            expectedGeneration: generation!,
            expectedUid: detail.metadata.uid,
            expectedResourceVersion: detail.metadata.resourceVersion,
          }, csrfToken)
          outcome.deleted += 1
        } catch (error) {
          outcome.failed.push({ name: `${item.kind}/${item.name}`, reason: mutationError(error) })
        }
      }
      return outcome
    },
    onSuccess: (outcome) => {
      toast.success(`Deleted ${outcome.deleted} workload${outcome.deleted === 1 ? '' : 's'}`, outcome.failed.length ? `${outcome.failed.length} failed: ${outcome.failed.map((item) => item.name).join(', ')}` : 'Every selected workload was removed.')
      setBulkConfirm(false)
      setSelectedKeys(new Set())
      void queryClient.invalidateQueries({ queryKey: ['resources', 'workloads'] })
    },
    onError: (error) => {
      toast.error('Bulk delete failed', mutationError(error))
      setBulkConfirm(false)
    },
  })

  return (
    <ResourcePage
      title="Workloads"
      description="Deployments, StatefulSets, DaemonSets, Jobs and CronJobs in the active scope."
      actions={selection ? <ResourceLiveUpdates key={`workloads/${generation}`} generation={generation!} topics={['workloads']} queryKeys={[["resources", "workloads"]]} autoStart={list.isPending} onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'workloads'] })} onClear={() => { setDraft({ ...defaultWorkloadList, kind: kindPreset }); setApplied({ ...defaultWorkloadList, kind: kindPreset }) }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('kind', 'Kind', applied.kind), ...activeFilter('status', 'Status', applied.workloadStatus),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={workloadSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <NamespaceFilterInput value={draft.namespace} onChange={(value) => setDraft((current) => ({ ...current, namespace: value }))} />
        <Select aria-label="Kind" className="!h-7 !w-auto max-w-[9rem] pr-6 text-sm" value={draft.kind} onChange={(event) => setDraft((current) => ({ ...current, kind: event.target.value }))}><option value="">All kinds</option><option value="deployments">Deployments</option><option value="replicasets">ReplicaSets</option><option value="statefulsets">StatefulSets</option><option value="daemonsets">DaemonSets</option><option value="jobs">Jobs</option><option value="cronjobs">CronJobs</option></Select>
        <Select aria-label="Status" className="!h-7 !w-auto max-w-[9rem] pr-6 text-sm" value={draft.workloadStatus} onChange={(event) => setDraft((current) => ({ ...current, workloadStatus: event.target.value }))}><option value="">All statuses</option>{workloadStatuses.map((value) => <option key={value}>{value}</option>)}</Select>
      </ResourceListControls>
      {selection ? <SavedFilterControls collection="workloads" generation={generation!} currentQuery={compactFilterQuery([
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
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending && !previewActive} error={!list.data || collection.authorizationFailed ? list.error : null} empty={visibleItems.length === 0 && !list.hasNextPage}>
          {selectedItems.length > 0 ? (
            <BulkToolbar count={selectedItems.length}>
              <Button variant="danger" size="sm" onClick={() => setBulkConfirm(true)}><Trash2 size={12} aria-hidden="true" /> Delete selected</Button>
              <Button variant="ghost" size="sm" onClick={() => setSelectedKeys(new Set())}>Clear selection</Button>
            </BulkToolbar>
          ) : null}
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            <ColumnVisibilityControl state={workloadColumnState} columns={workloadColumns} />
            {previewActive ? <p className="px-3 py-1.5 text-xs text-kp-sky" role="status">Receiving workloads · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
            {list.isPlaceholderData || list.isFetching && !list.isFetchingNextPage ? <p className="px-3 py-1.5 text-xs text-kp-overlay-text" role="status">Refreshing workloads…</p> : null}
            <DataTable
              caption="Authorized workload pages"
              rows={visibleItems}
              onScrollProgress={collection.onScrollProgress}
              getRowKey={rowKey}
              columns={applyColumnVisibility(workloadColumns, workloadColumnState)}
              selectable={!previewActive}
              selectedKeys={selectedKeys}
              onToggleRow={(key, checked) => {
                setSelectedKeys((current) => {
                  const next = new Set(current)
                  if (checked) next.add(key)
                  else next.delete(key)
                  return next
                })
              }}
              onToggleAll={(checked) => {
                setSelectedKeys(checked ? new Set(collection.items.map(rowKey)) : new Set())
              }}
            />
            {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={list.data?.pages.length ?? 0} firstPage={list.data?.pageParams[0] === '' && list.data.pages.length === 1} hasNextPage={Boolean(list.hasNextPage)} loading={list.isFetching} refreshing={list.isPlaceholderData} nextPageError={list.isFetchNextPageError} onNext={() => void list.fetchNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
      <ConfirmDialog
        open={bulkConfirm}
        severity="danger"
        title={`Delete ${selectedItems.length} workload${selectedItems.length === 1 ? '' : 's'}`}
        description="Each delete is authorized and re-validated by Kubernetes individually before it executes."
        resources={selectedItems.map((item) => ({ kind: item.kind, namespace: item.namespace, name: item.name }))}
        consequenceNote="Dependent ReplicaSets, Pods and Jobs are garbage-collected by Kubernetes after deletion. This action cannot be undone."
        confirmLabel="Delete selected"
        pendingLabel="Deleting…"
        pending={bulkDelete.isPending}
        onConfirm={() => bulkDelete.mutate()}
        onCancel={() => setBulkConfirm(false)}
      />
    </ResourcePage>
  )
}

export function PodsPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const toast = useToast()
  const { namespace: paramNamespace, name: paramName } = useParams<{ namespace: string; name: string }>()
  const [params] = useSearchParams()
  const generation = selection?.generation
  const [draft, setDraft] = useState<PodListState>(() => podsStateFromParams(params))
  const [applied, setApplied] = useState<PodListState>(() => podsStateFromParams(params))
  const queryClient = useQueryClient()
  const [selectedKeys, setSelectedKeys] = useSelectionBoundKeys([selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value])
  const [bulkConfirm, setBulkConfirm] = useState(false)
  const preview = useResourceStreamPreview<Pod>({ identity: [selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value], topic: 'pods', namespace: globalNamespace.value, isItem: isPodPreview, itemKey: namedPreviewKey })

  useEffect(() => {
    if (!paramNamespace || !paramName || !generation) return
    workspace.openFromRoute({ collection: 'pods', namespace: paramNamespace, name: paramName })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [paramNamespace, paramName, generation])

  const rowKey = useCallback((item: Pod) => `${item.namespace}/${item.name}`, [])
  const collection = useInfiniteCollection<Pod>({
    identity: ['resources', 'pods', selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value],
    filters: applied,
    fetchPage: (cursor, signal) => getPods({ limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, namespaces: effectiveNamespaces(globalNamespace.value, namespaceValues(applied.namespace)), statuses: applied.podStatus ? [applied.podStatus] : undefined, workload: applied.workload || undefined, node: applied.node || undefined, restarts: applied.restarts as 'any' | 'gt0' | 'gte3' | 'gte10', problematic: applied.problematic === '' ? undefined : applied.problematic === 'true', ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, generation),
    enabled: Boolean(selection),
  })
  const list = collection.query
  const listKey = collection.queryKey
  const listData = collection.lastPage
  const listItems = collection.items
  const snapshotRenewed = collection.snapshotRenewed
  const authorizationFailed = collection.authorizationFailed
  useEffect(() => { if (authorizationFailed) setSelectedKeys(new Set()) }, [authorizationFailed, setSelectedKeys])
  const previewActive = list.isPending && !authorizationFailed && sameListState(applied, defaultPodList) && Boolean(preview.preview?.items.length)
  const visibleItems = previewActive ? preview.preview!.items : listItems
  const selectedItems = useMemo(() => listItems.filter((item) => selectedKeys.has(rowKey(item))), [listItems, selectedKeys, rowKey])

  // V5-11: Pod metrics render only when the Metrics API is healthy; absence,
  // denial or partial coverage touches the metrics columns alone.
  const metricsAvailable = status.data?.components.metrics.status === 'healthy'
  const metrics = useQuery({ queryKey: ['pod-metrics', generation], queryFn: ({ signal }) => getDashboardMetrics(signal, generation), enabled: Boolean(selection && metricsAvailable), staleTime: 30_000 })
  const metricsByPod = useMemo(() => {
    const map = new Map<string, { cpuMillicores: number; memoryBytes: number }>()
    for (const entry of metrics.data?.block.value.pods ?? []) {
      map.set(`${entry.namespace}/${entry.pod}`, { cpuMillicores: entry.cpuMillicores, memoryBytes: entry.memoryBytes })
    }
    return map
  }, [metrics.data])

  const podColumnState = usePreferenceColumnVisibility('pods')
  const podColumns: DataTableColumn<Pod>[] = [
    { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
    { key: 'name', header: 'Pod', cell: (item) => <TableLink aria-label={`Open Pod ${item.name} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'pods', namespace: item.namespace, name: item.name })} primary={<>{item.name}{item.problematic ? <Badge variant="danger" className="ml-2">problem</Badge> : null}</>} /> },
    { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
    { key: 'ready', header: 'Ready', cell: (item) => `${item.ready.current}/${item.ready.desired}` },
    { key: 'restarts', header: 'Restarts', cell: (item) => item.restarts },
    { key: 'cpu', header: 'CPU', cell: (item) => { const value = metricsByPod.get(rowKey(item)); return value ? `${value.cpuMillicores} m` : '—' } },
    { key: 'memory', header: 'Memory', cell: (item) => { const value = metricsByPod.get(rowKey(item)); return value ? `${Math.round(value.memoryBytes / (1024 * 1024))} Mi` : '—' } },
    { key: 'node', header: 'Node', cell: (item) => item.node ?? '—' },
    { key: 'owner', header: 'Owner', cell: (item) => item.owner ? <span className="text-sm">{item.owner.kind}/{item.owner.name}</span> : 'standalone' },
    { key: 'ip', header: 'IP', cell: (item) => item.ip ?? '—' },
    { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
  ]

  const bulkDelete = useMutation({
    mutationFn: async (): Promise<BulkOutcome> => {
      const csrfToken = await csrfForGeneration(generation!)
      const outcome: BulkOutcome = { deleted: 0, failed: [] }
      for (const item of selectedItems) {
        try {
          const detail = await getPod(item.namespace, item.name, undefined, generation)
          await deletePod(item.namespace, item.name, {
            confirmed: true,
            action: 'deletePod',
            consequenceCode: 'DELETE_POD',
            target: { clusterProfileId: selection!.clusterProfileId, context: selection!.context, namespace: item.namespace, kind: 'Pod', name: item.name },
            expectedGeneration: generation!,
            expectedUid: detail.metadata.uid,
            expectedResourceVersion: detail.metadata.resourceVersion,
          }, csrfToken)
          outcome.deleted += 1
        } catch (error) {
          outcome.failed.push({ name: `${item.namespace}/${item.name}`, reason: mutationError(error) })
        }
      }
      return outcome
    },
    onSuccess: (outcome) => {
      toast.success(`Deleted ${outcome.deleted} Pod${outcome.deleted === 1 ? '' : 's'}`, outcome.failed.length ? `${outcome.failed.length} failed: ${outcome.failed.map((item) => item.name).join(', ')}` : 'Controllers recreate owned Pods according to their strategy.')
      setBulkConfirm(false)
      setSelectedKeys(new Set())
      void queryClient.invalidateQueries({ queryKey: ['resources', 'pods'] })
    },
    onError: (error) => {
      toast.error('Bulk delete failed', mutationError(error))
      setBulkConfirm(false)
    },
  })

  return (
    <ResourcePage
      title="Pods"
      description="Pod inventory with readiness, restarts, owner, metrics and problem evidence in the active scope."
      actions={<div className="flex items-center gap-2"><Link to="/logs"><Button variant="secondary" size="md"><ScrollText size={14} aria-hidden="true" /> Open logs</Button></Link>{selection ? <ResourceLiveUpdates key={`pods/${generation}`} generation={generation!} topics={['pods']} queryKeys={[["resources", "pods"]]} autoStart={list.isPending} onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}</div>}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={(interactionId) => setApplied(bindListInteraction({ ...draft }, interactionId))} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'pods'] })} onClear={() => { setDraft({ ...defaultPodList }); setApplied({ ...defaultPodList }) }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('workload', 'Workload owner', applied.workload), ...activeFilter('node', 'Node', applied.node), ...activeFilter('status', 'Status', applied.podStatus), ...activeFilter('restarts', 'Restarts', applied.restarts === 'any' ? '' : applied.restarts), ...activeFilter('problematic', 'Problem evidence', applied.problematic === 'true' ? 'problematic only' : applied.problematic === 'false' ? 'without evidence' : ''),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={podSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <NamespaceFilterInput value={draft.namespace} onChange={(value) => setDraft((current) => ({ ...current, namespace: value }))} />
        <Input aria-label="Workload owner" value={draft.workload} maxLength={256} placeholder="owner" className="!h-7 !w-[8rem] text-sm" onChange={(event) => setDraft((current) => ({ ...current, workload: event.target.value }))} />
        <Input aria-label="Node" value={draft.node} maxLength={256} placeholder="node" className="!h-7 !w-[8rem] text-sm" onChange={(event) => setDraft((current) => ({ ...current, node: event.target.value }))} />
        <Select aria-label="Status" className="!h-7 !w-auto max-w-[8rem] pr-6 text-sm" value={draft.podStatus} onChange={(event) => setDraft((current) => ({ ...current, podStatus: event.target.value }))}><option value="">All statuses</option>{podStatuses.map((value) => <option key={value}>{value}</option>)}</Select>
        <Select aria-label="Restarts" className="!h-7 !w-auto max-w-[8rem] pr-6 text-sm" value={draft.restarts} onChange={(event) => setDraft((current) => ({ ...current, restarts: event.target.value }))}><option value="any">Any restarts</option><option value="gt0">More than 0</option><option value="gte3">At least 3</option><option value="gte10">At least 10</option></Select>
        <Select aria-label="Problem evidence" className="!h-7 !w-auto max-w-[10rem] pr-6 text-sm" value={draft.problematic} onChange={(event) => setDraft((current) => ({ ...current, problematic: event.target.value }))}><option value="">All Pods</option><option value="true">Problematic only</option><option value="false">Without evidence</option></Select>
      </ResourceListControls>
      {selection ? <SavedFilterControls collection="pods" generation={generation!} currentQuery={compactFilterQuery([
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
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending && visibleItems.length === 0} error={!list.data || authorizationFailed ? list.error : null} empty={visibleItems.length === 0 && !list.hasNextPage}>
          {selectedItems.length > 0 ? (
            <BulkToolbar count={selectedItems.length}>
              {selectedItems.length === 1 ? (
                <Link to={`/logs?namespace=${encodeURIComponent(selectedItems[0].namespace)}&pod=${encodeURIComponent(selectedItems[0].name)}`}><Button variant="secondary" size="sm"><ScrollText size={12} aria-hidden="true" /> View logs</Button></Link>
              ) : null}
              <Button variant="danger" size="sm" onClick={() => setBulkConfirm(true)}><Trash2 size={12} aria-hidden="true" /> Delete selected</Button>
              <Button variant="ghost" size="sm" onClick={() => setSelectedKeys(new Set())}>Clear selection</Button>
            </BulkToolbar>
          ) : null}
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            <ColumnVisibilityControl state={podColumnState} columns={podColumns} />
            {previewActive ? <p className="px-3 py-1.5 text-xs text-kp-sky" role="status">Receiving Pods · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
            {list.isPlaceholderData || list.isFetching && !list.isFetchingNextPage ? <p className="px-3 py-1.5 text-xs text-kp-overlay-text" role="status">Refreshing Pods…</p> : null}
            <DataTable
              caption="Authorized Pod pages"
              rows={visibleItems}
              getRowKey={rowKey}
              columns={applyColumnVisibility(podColumns, podColumnState)}
              selectable={!previewActive}
              selectedKeys={selectedKeys}
              onToggleRow={(key, checked) => {
                setSelectedKeys((current) => {
                  const next = new Set(current)
                  if (checked) next.add(key)
                  else next.delete(key)
                  return next
                })
              }}
              onToggleAll={(checked) => {
                setSelectedKeys(checked ? new Set(listItems.map(rowKey)) : new Set())
              }}
              onScrollProgress={collection.onScrollProgress}
            />
            {listData ? (
              <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-kp-overlay-0 px-3 py-2.5">
                <div className="text-xs text-kp-overlay-text">
                  <span className="block text-kp-subtext">{listItems.length} Pods loaded · {list.data?.pages.length ?? 0} page{list.data?.pages.length === 1 ? '' : 's'} retained</span>
                  <small className="block">{listData.page.complete ? 'Collection complete' : `Bounded ${listData.page.filterScope} result`}</small>
                  {snapshotRenewed ? <small className="block text-kp-yellow" role="status">The list snapshot expired and was renewed from the first page.</small> : null}
                  {listData.coverage ? <small className="block">{listData.coverage.completedNamespaces}/{listData.coverage.requestedNamespaces} namespaces completed · {listData.coverage.deniedNamespaces.length} denied</small> : null}
                </div>
                <div className="flex gap-2">
                  <Button variant="secondary" size="sm" disabled={list.data?.pageParams[0] === '' && list.data?.pages.length === 1} disabledReason="Already on the first page." onClick={() => void queryClient.resetQueries({ queryKey: listKey })}>First page</Button>
                  <Button size="sm" disabled={!list.hasNextPage || list.isFetching || list.isPlaceholderData} disabledReason="The current result has no next page or is refreshing." onClick={() => void list.fetchNextPage()}>{list.isFetchingNextPage ? 'Loading…' : 'Load next page'}</Button>
                </div>
                {list.isFetchNextPageError ? <p className="w-full text-xs text-kp-red" role="alert">The next page could not be loaded. The loaded Pods remain available; retry when ready.</p> : null}
              </footer>
            ) : null}
          </div>
          {!metricsAvailable ? <p className="mt-1.5 text-xs text-kp-overlay-text" role="note">Metrics API unavailable; CPU and memory columns stay empty.</p> : null}
        </QueryState>
      </SelectionGate>
      <ConfirmDialog
        open={bulkConfirm}
        severity="danger"
        title={`Delete ${selectedItems.length} Pod${selectedItems.length === 1 ? '' : 's'}`}
        description="Each delete is authorized and re-validated by Kubernetes individually before it executes."
        resources={selectedItems.map((item) => ({ kind: 'Pod', namespace: item.namespace, name: item.name }))}
        consequenceNote="Owned Pods are recreated by their controllers; standalone Pods are gone for good. This action cannot be undone."
        confirmLabel="Delete selected"
        pendingLabel="Deleting…"
        pending={bulkDelete.isPending}
        onConfirm={() => bulkDelete.mutate()}
        onCancel={() => setBulkConfirm(false)}
      />
    </ResourcePage>
  )
}

export function EventsPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const [params] = useSearchParams()
  const generation = selection?.generation
  const [draft, setDraft] = useState<EventListState>(() => eventsStateFromParams(params))
  const [applied, setApplied] = useState<EventListState>(() => eventsStateFromParams(params))
  const queryClient = useQueryClient()
  const eventColumnState = usePreferenceColumnVisibility('events')
  const eventColumns: DataTableColumn<EventResource>[] = [
    { key: 'time', header: 'Time', cell: (item) => dateTime(item.timestamp) },
    { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
    { key: 'object', header: 'Object', cell: (item) => `${item.objectKind}/${item.objectName}` },
    { key: 'type', header: 'Type / reason', cell: (item) => <><Badge variant={eventBadgeVariant(item.type)}>{item.type}</Badge><small className="mt-0.5 block text-xs text-kp-overlay-text">{item.reason}</small></> },
    { key: 'count', header: 'Count', cell: (item) => item.count },
    { key: 'message', header: 'Message', cell: (item) => <span className="block max-w-[480px] break-words text-sm leading-snug">{item.message}</span> },
  ]
  const collection = useInfiniteCollection<EventResource>({
    identity: ['resources', 'events', selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value],
    filters: applied,
    fetchPage: (cursor, signal) => getEvents({ limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, namespaces: effectiveNamespaces(globalNamespace.value, namespaceValues(applied.namespace)), statuses: applied.eventType ? [applied.eventType] : undefined, objectKind: applied.objectKind || undefined, reason: applied.reason || undefined, continueToken: cursor || undefined, ...optionalSort(applied.sort, applied.order, 'timestamp', 'desc') }, signal, generation),
    enabled: Boolean(selection),
  })
  const list = collection.query
  const preview = useResourceStreamPreview<EventResource>({ identity: [selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value], topic: 'events', namespace: globalNamespace.value, isItem: isEventPreview, itemKey: eventPreviewKey, compare: compareEventPreview })
  const previewActive = list.isPending && !collection.authorizationFailed && sameListState(applied, defaultEventList) && Boolean(preview.preview?.items.length)
  const visibleItems = previewActive ? preview.preview!.items : collection.items
  return (
    <ResourcePage
      title="Events"
      description="Kubernetes events ordered within the bounded page; type, source and count are preserved."
      actions={selection ? <ResourceLiveUpdates key={`events/${generation}`} generation={generation!} topics={['events']} queryKeys={[["resources", "events"]]} autoStart={list.isPending} onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'events'] })} onClear={() => { setDraft({ ...defaultEventList }); setApplied({ ...defaultEventList }) }} activeFilters={[
        ...activeFilter('namespace', 'Namespace', namespaceValues(applied.namespace)), ...activeFilter('type', 'Type', applied.eventType), ...activeFilter('objectKind', 'Object kind', applied.objectKind), ...activeFilter('reason', 'Reason', applied.reason),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="timestamp" defaultOrder="desc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={eventSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <NamespaceFilterInput value={draft.namespace} onChange={(value) => setDraft((current) => ({ ...current, namespace: value }))} />
        <Select aria-label="Type" className="!h-7 !w-auto max-w-[8rem] pr-6 text-sm" value={draft.eventType} onChange={(event) => setDraft((current) => ({ ...current, eventType: event.target.value }))}><option value="">All types</option>{eventTypes.map((value) => <option key={value}>{value}</option>)}</Select>
        <Input aria-label="Object kind" value={draft.objectKind} maxLength={256} placeholder="object kind" className="!h-7 !w-[9rem] text-sm" onChange={(event) => setDraft((current) => ({ ...current, objectKind: event.target.value }))} />
        <Input aria-label="Reason" value={draft.reason} maxLength={256} placeholder="reason" className="!h-7 !w-[8rem] text-sm" onChange={(event) => setDraft((current) => ({ ...current, reason: event.target.value }))} />
      </ResourceListControls>
      {selection ? <SavedFilterControls collection="events" generation={generation!} currentQuery={compactFilterQuery([
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
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending && !previewActive} error={!list.data || collection.authorizationFailed ? list.error : null} empty={visibleItems.length === 0 && !list.hasNextPage}>
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            <ColumnVisibilityControl state={eventColumnState} columns={eventColumns} />
            {previewActive ? <p className="px-3 py-1.5 text-xs text-kp-sky" role="status">Receiving events · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
            {list.isPlaceholderData || list.isFetching && !list.isFetchingNextPage ? <p className="px-3 py-1.5 text-xs text-kp-overlay-text" role="status">Refreshing events…</p> : null}
            <DataTable
              caption="Authorized event pages"
              rows={visibleItems}
              onScrollProgress={collection.onScrollProgress}
              getRowKey={(item, index) => `${item.namespace}/${item.objectKind}/${item.objectName}/${item.timestamp ?? index}`}
              columns={applyColumnVisibility(eventColumns, eventColumnState)}
              stickyHeader
            />
            {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={list.data?.pages.length ?? 0} firstPage={list.data?.pageParams[0] === '' && list.data.pages.length === 1} hasNextPage={Boolean(list.hasNextPage)} loading={list.isFetching} refreshing={list.isPlaceholderData} nextPageError={list.isFetchNextPageError} onNext={() => void list.fetchNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}

type NetworkTab = 'services' | 'endpoints' | 'ingresses' | 'ingress-classes' | 'endpoint-slices' | 'network-policies' | 'port-forwards'
type NetworkResourceTab = Exclude<NetworkTab, 'port-forwards'>
type NetworkItem = ServiceResource | IngressResource | EndpointSliceResource | Endpoints | IngressClass | NetworkPolicy
type NetworkPreviewItem = ServiceResource | IngressResource | EndpointSliceResource

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

const networkCollections: Record<NetworkResourceTab, string> = {
  services: 'services',
  endpoints: 'endpoints',
  ingresses: 'ingresses',
  'ingress-classes': 'ingress-classes',
  'endpoint-slices': 'endpoint-slices',
  'network-policies': 'network-policies',
}

function networkTabFromParams(tab: string): NetworkResourceTab | null {
  return tab === 'services' || tab === 'endpoints' || tab === 'ingresses' || tab === 'ingress-classes' || tab === 'endpoint-slices' || tab === 'network-policies' ? tab : null
}

export function NetworkPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const navigate = useNavigate()
  const toast = useToast()
  const { tab: tabParam, namespace: paramNamespace, name: paramName } = useParams<{ tab: string; namespace?: string; name?: string }>()
  const generation = selection?.generation
  const [tabState, setTab] = useState<NetworkTab>(() => networkTabFromParams(tabParam ?? '') ?? 'services')
  const tab = useMemo(() => networkTabFromParams(tabParam ?? '') ?? tabState, [tabParam, tabState])
  const [drafts, setDrafts] = useState<Record<NetworkResourceTab, SimpleListState>>(() => structuredClone(defaultNetworkLists))
  const [appliedLists, setAppliedLists] = useState<Record<NetworkResourceTab, SimpleListState>>(() => structuredClone(defaultNetworkLists))
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(generation)
  const resourceTab: NetworkResourceTab = tab === 'port-forwards' ? 'services' : tab
  const draft = drafts[resourceTab]
  const applied = appliedLists[resourceTab]

  // Deep links (/network/:tab/:ns/:name or /network/:tab/:name) open the workspace.
  useEffect(() => {
    const parsedTab = networkTabFromParams(tabParam ?? '')
    if (!parsedTab || !paramName || !generation) return
    workspace.openFromRoute({ collection: networkCollections[parsedTab], namespace: paramNamespace ?? null, name: paramName })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tabParam, paramNamespace, paramName, generation])

  const networkOptions = (cursor: string) => ({ limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, continueToken: cursor || undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc') })
  const collection = useInfiniteCollection<NetworkItem>({
    identity: ['resources', resourceTab, selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value],
    filters: applied,
    enabled: Boolean(selection && tab !== 'port-forwards'),
    fetchPage: (cursor, signal) => {
      const options = networkOptions(cursor)
      const namespacedOptions = { ...options, namespaces: effectiveNamespaces(globalNamespace.value, []) }
      switch (resourceTab) {
        case 'services': return getServices(namespacedOptions, signal, generation)
        case 'ingresses': return getIngresses(namespacedOptions, signal, generation)
        case 'endpoint-slices': return getEndpointSlices(namespacedOptions, signal, generation)
        case 'endpoints': return getEndpointsList(namespacedOptions, signal, generation)
        case 'ingress-classes': return getIngressClasses(options, signal, generation)
        case 'network-policies': return getNetworkPolicies(namespacedOptions, signal, generation)
      }
    },
  })
  const activeQuery = collection.query
  const streamTopic = resourceTab === 'ingresses' || resourceTab === 'endpoint-slices' ? resourceTab : 'services'
  const preview = useResourceStreamPreview<NetworkPreviewItem>({ identity: [resourceTab, selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value], topic: streamTopic, namespace: globalNamespace.value, isItem: isNetworkPreview, itemKey: namedPreviewKey })
  const previewActive = (resourceTab === 'services' || resourceTab === 'ingresses' || resourceTab === 'endpoint-slices') && activeQuery.isPending && !collection.authorizationFailed && sameListState(applied, defaultSimpleList) && Boolean(preview.preview?.items.length)
  const visibleItems: NetworkItem[] = previewActive ? preview.preview!.items : collection.items
  const forwards = useQuery({ queryKey: ['port-forwards', generation], queryFn: ({ signal }) => getPortForwards(signal, generation!), enabled: Boolean(selection && tab === 'port-forwards'), refetchInterval: 10_000 })
  const close = useMutation({ mutationFn: (id: string) => requests.run(async (signal) => { const session = await getSession(signal); if (session.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' }); return closePortForward(id, generation!, session.csrfToken, signal) }), onSuccess: () => { toast.info('Loopback session closed'); queryClient.invalidateQueries({ queryKey: ['port-forwards'] }) }, onError: (error) => toast.error('Failed to close session', mutationError(error)) })
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
          if (sessionData.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
          return closePortForward(session.id, generation!, sessionData.csrfToken, signal)
        })
        closed += 1
      } catch {
        failed += 1
      }
    }
    return { closed, failed }
  }, onSuccess: (result) => { setStopAllResult(result); setStopAllState('idle'); toast.info(`Closed ${result.closed} session${result.closed === 1 ? '' : 's'}`, result.failed ? `${result.failed} failed` : undefined); queryClient.invalidateQueries({ queryKey: ['port-forwards'] }) } })

  const networkColumnState = usePreferenceColumnVisibility(`network/${tab}`)
  const networkColumns: DataTableColumn<NetworkItem>[] = [
    { key: 'name', header: 'Name', cell: (item) => <TableLink aria-label={`Open ${tab} ${item.name}${'namespace' in item ? ` in ${item.namespace}` : ''}`} onClick={() => workspace.openResource({ collection: networkCollections[resourceTab], namespace: 'namespace' in item ? item.namespace : null, name: item.name })} primary={item.name} secondary={'namespace' in item ? item.namespace : 'cluster'} /> },
    { key: 'type', header: 'Type', cell: (item) => ('type' in item ? item.type : 'className' in item ? (item.className ?? 'Ingress') : 'addressType' in item ? item.addressType : 'podSelector' in item ? item.podSelector || 'none' : 'controller' in item ? item.controller : '—') },
    { key: 'summary', header: 'Summary', cell: (item) => ('clusterIPs' in item ? item.clusterIPs.join(', ') : 'hosts' in item ? item.hosts.join(', ') : 'addressType' in item ? `${item.endpoints.length} endpoints` : 'readyCount' in item ? `${item.readyCount} ready / ${item.notReadyCount} not ready${item.truncated ? ' (truncated)' : ''}` : 'ruleSummary' in item ? item.ruleSummary.length + ' rules' : item.default ? 'default class' : '—') },
  ]


  return (
    <ResourcePage title="Network" description="Services, Ingresses, EndpointSlices and loopback-only port-forward sessions.">
      <ResourceTabStrip ariaLabel="Network resource type" panelId="network-panel" active={tab} onChange={(value) => { const next = value as NetworkTab; setTab(next); navigate(`/network/${next}`) }} tabs={[
        { id: 'services', label: 'services' },
        { id: 'endpoints', label: 'endpoints' },
        { id: 'ingresses', label: 'ingresses' },
        { id: 'ingress-classes', label: 'ingress-classes' },
        { id: 'endpoint-slices', label: 'endpoint-slices' },
        { id: 'network-policies', label: 'network-policies' },
        { id: 'port-forwards', label: 'port-forwards' },
      ]} />
      {selection && (tab === 'services' || tab === 'ingresses' || tab === 'endpoint-slices') ? <ResourceLiveUpdates key={`${tab}/${generation}`} generation={generation!} topics={[tab]} queryKeys={[["resources", tab]]} autoStart={activeQuery.isPending} onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}
      {tab !== 'port-forwards' ? <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], search: value } }))} onApply={(interactionId) => { setAppliedLists((current) => ({ ...current, [resourceTab]: bindListInteraction({ ...draft }, interactionId) })) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} onClear={() => { setDrafts((current) => ({ ...current, [resourceTab]: { ...defaultSimpleList } })); setAppliedLists((current) => ({ ...current, [resourceTab]: { ...defaultSimpleList } })) }} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={networkSortOptions[resourceTab]} onSortChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], sort: value } }))} onOrderChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], order: value } }))} /> : null}
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
          </QueryState> : <QueryState pending={activeQuery.isPending && !previewActive} error={!activeQuery.data || collection.authorizationFailed ? activeQuery.error : null} empty={visibleItems.length === 0 && !activeQuery.hasNextPage}>
            <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
              <ColumnVisibilityControl state={networkColumnState} columns={networkColumns} />
              {previewActive ? <p className="px-3 py-1.5 text-xs text-kp-sky" role="status">Receiving {tab} · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
              {activeQuery.isPlaceholderData || activeQuery.isFetching && !activeQuery.isFetchingNextPage ? <p className="px-3 py-1.5 text-xs text-kp-overlay-text" role="status">Refreshing {tab}…</p> : null}
              <DataTable
                caption={`Authorized ${tab} pages`}
                rows={visibleItems}
                onScrollProgress={collection.onScrollProgress}
                getRowKey={(item) => `${'namespace' in item ? `${item.namespace}/` : ''}${item.name}`}
                columns={applyColumnVisibility(networkColumns, networkColumnState)}
                stickyHeader
              />
              {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={activeQuery.data?.pages.length ?? 0} firstPage={activeQuery.data?.pageParams[0] === '' && activeQuery.data.pages.length === 1} hasNextPage={Boolean(activeQuery.hasNextPage)} loading={activeQuery.isFetching} refreshing={activeQuery.isPlaceholderData} nextPageError={activeQuery.isFetchNextPageError} onNext={() => void activeQuery.fetchNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
            </div>
          </QueryState>}
        </SelectionGate>
      </div>
    </ResourcePage>
  )
}

type ConfigTab = 'configmaps' | 'secrets'
type ConfigItem = { metadata: { namespace: string; name: string; uid: string; creationTimestamp: string } } | { namespace: string; name: string; uid: string; creationTimestamp: string }

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

export function ConfigPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const navigate = useNavigate()
  const { tab: tabParam, namespace: paramNamespace, name: paramName } = useParams<{ tab: string; namespace?: string; name?: string }>()
  const generation = selection?.generation
  const [tabState, setTab] = useState<ConfigTab>(() => configTabFromParams(tabParam ?? '') ?? 'configmaps')
  const tab = useMemo(() => configTabFromParams(tabParam ?? '') ?? tabState, [tabParam, tabState])
  const [drafts, setDrafts] = useState<Record<ConfigTab, SimpleListState>>(() => structuredClone(defaultConfigLists))
  const [appliedLists, setAppliedLists] = useState<Record<ConfigTab, SimpleListState>>(() => structuredClone(defaultConfigLists))
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!paramNamespace || !paramName || !generation || !tab) return
    workspace.openFromRoute({ collection: tab, namespace: paramNamespace, name: paramName })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tabParam, paramNamespace, paramName, generation])

  const draft = drafts[tab]
  const applied = appliedLists[tab]
  const collection = useInfiniteCollection<ConfigItem>({
    identity: ['resources', tab, selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value],
    filters: applied,
    enabled: Boolean(selection),
    fetchPage: (cursor, signal) => {
      const options = { limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, continueToken: cursor || undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), namespaces: effectiveNamespaces(globalNamespace.value, []) }
      return tab === 'configmaps' ? getConfigMapsSafe(options, signal, generation) : getSecretsSafe(options, signal, generation)
    },
  })

  const configColumnState = usePreferenceColumnVisibility(`config/${tab}`)
  const configColumns: DataTableColumn<ConfigItem>[] = [
    { key: 'name', header: 'Namespace / name', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return <TableLink aria-label={`Open ${tab === 'secrets' ? 'Secret' : 'ConfigMap'} ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} secondary={value.namespace} /> } },
    { key: 'uid', header: 'UID', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return <span className="mono text-xs">{value.uid}</span> } },
    { key: 'created', header: 'Created', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return dateTime(value.creationTimestamp) } },
  ]
  const activeQuery = collection.query
  const preview = useResourceStreamPreview<ConfigMapResource>({ identity: [tab, selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value], topic: 'configmaps', namespace: globalNamespace.value, isItem: isConfigMapPreview, itemKey: namedPreviewKey })
  const previewActive = tab === 'configmaps' && activeQuery.isPending && !collection.authorizationFailed && sameListState(applied, defaultSimpleList) && Boolean(preview.preview?.items.length)
  const visibleItems: ConfigItem[] = previewActive ? preview.preview!.items : collection.items

  return (
    <ResourcePage title="Configuration" description="ConfigMaps are fetched on detail; Secrets remain metadata-only and never expose values or YAML.">
      <ResourceTabStrip ariaLabel="Configuration resource type" panelId="config-panel" active={tab} onChange={(value) => { const next = value as ConfigTab; setTab(next); navigate(`/config/${next}`) }} tabs={[
        { id: 'configmaps', label: 'configmaps' },
        { id: 'secrets', label: 'secrets' },
      ]} />
      {selection && tab === 'configmaps' ? <ResourceLiveUpdates key={`configmaps/${generation}`} generation={generation!} topics={['configmaps']} queryKeys={[["resources", "configmaps"]]} autoStart={activeQuery.isPending} onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], search: value } }))} onApply={(interactionId) => { setAppliedLists((current) => ({ ...current, [tab]: bindListInteraction({ ...draft }, interactionId) })) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} onClear={() => { setDrafts((current) => ({ ...current, [tab]: { ...defaultSimpleList } })); setAppliedLists((current) => ({ ...current, [tab]: { ...defaultSimpleList } })) }} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={configSortOptions} onSortChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], sort: value } }))} onOrderChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], order: value } }))} />
      <div id="config-panel" role="tabpanel">
        <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
          <QueryState pending={activeQuery.isPending && !previewActive} error={!activeQuery.data || collection.authorizationFailed ? activeQuery.error : null} empty={visibleItems.length === 0 && !activeQuery.hasNextPage}>
            <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
              <ColumnVisibilityControl state={configColumnState} columns={configColumns} />
              {previewActive ? <p className="px-3 py-1.5 text-xs text-kp-sky" role="status">Receiving ConfigMaps · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
              {activeQuery.isPlaceholderData || activeQuery.isFetching && !activeQuery.isFetchingNextPage ? <p className="px-3 py-1.5 text-xs text-kp-overlay-text" role="status">Refreshing {tab}…</p> : null}
              <DataTable
                caption={`Authorized ${tab} metadata pages`}
                rows={visibleItems}
                onScrollProgress={collection.onScrollProgress}
                getRowKey={(item) => { const value = 'metadata' in item ? item.metadata : item; return `${value.namespace}/${value.name}` }}
                columns={applyColumnVisibility(configColumns, configColumnState)}
                stickyHeader
              />
              {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={activeQuery.data?.pages.length ?? 0} firstPage={activeQuery.data?.pageParams[0] === '' && activeQuery.data.pages.length === 1} hasNextPage={Boolean(activeQuery.hasNextPage)} loading={activeQuery.isFetching} refreshing={activeQuery.isPlaceholderData} nextPageError={activeQuery.isFetchNextPageError} onNext={() => void activeQuery.fetchNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
            </div>
          </QueryState>
        </SelectionGate>
      </div>
    </ResourcePage>
  )
}

// Small wrappers keep the client import list honest for pages that only need
// two collection endpoints each.
async function getConfigMapsSafe(options: Parameters<typeof getPods>[0], signal: AbortSignal | undefined, generation: string | undefined) {
  const mod = await import('../api/client')
  return mod.getConfigMaps(options, signal, generation)
}
async function getSecretsSafe(options: Parameters<typeof getPods>[0], signal: AbortSignal | undefined, generation: string | undefined) {
  const mod = await import('../api/client')
  return mod.getSecrets(options, signal, generation)
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
  const workspace = useResourceWorkspace()
  const { name } = useParams<{ name: string }>()
  const [params] = useSearchParams()
  const generation = selection?.generation
  const [draft, setDraft] = useState<NodeListState>(() => nodeStateFromParams(params))
  const [applied, setApplied] = useState<NodeListState>(() => nodeStateFromParams(params))
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!name || !generation) return
    workspace.openFromRoute({ collection: 'nodes', name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [name, generation])

  const collection = useInfiniteCollection({
    identity: ['resources', 'nodes', selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, ''],
    filters: applied,
    fetchPage: (cursor: string, signal: AbortSignal) => getNodes({ limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, statuses: applied.nodeStatus ? [applied.nodeStatus] : undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, generation),
    enabled: Boolean(selection),
  })
  const list = collection.query

  return (
    <ResourcePage
      title="Nodes"
      description="Cluster nodes with readiness, roles, capacity and taints; a selected context is enough and no namespace filter applies."
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'nodes'] })} onClear={() => { setDraft({ ...defaultNodeList }); setApplied({ ...defaultNodeList }) }} activeFilters={[
        ...activeFilter('status', 'Status', applied.nodeStatus),
      ]} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={!sameListState(draft, applied)} sortOptions={nodeSortOptions} onSortChange={(value) => setDraft((current) => ({ ...current, sort: value }))} onOrderChange={(value) => setDraft((current) => ({ ...current, order: value }))}>
        <Select aria-label="Status" className="!h-7 !w-auto max-w-[9rem] pr-6 text-sm" value={draft.nodeStatus} onChange={(event) => setDraft((current) => ({ ...current, nodeStatus: event.target.value }))}><option value="">All statuses</option>{nodeStatuses.map((value) => <option key={value}>{value}</option>)}</Select>
      </ResourceListControls>
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={!list.data || collection.authorizationFailed ? list.error : null} empty={collection.items.length === 0 && !list.hasNextPage}>
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            {list.isPlaceholderData || list.isFetching && !list.isFetchingNextPage ? <p className="px-3 py-1.5 text-xs text-kp-overlay-text" role="status">Refreshing nodes…</p> : null}
            <DataTable
              caption="Authorized node pages"
              rows={collection.items}
              onScrollProgress={collection.onScrollProgress}
              getRowKey={(item) => item.name}
              columns={[
                { key: 'name', header: 'Node', cell: (item) => <TableLink aria-label={`Open Node ${item.name}`} onClick={() => workspace.openResource({ collection: 'nodes', name: item.name })} primary={item.name} secondary={item.roles.join(', ') || 'no role'} /> },
                { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
                { key: 'version', header: 'Version', cell: (item) => item.kubeletVersion || '—' },
                { key: 'internal-ip', header: 'Internal IP', cell: (item) => item.internalIP ?? '—' },
                { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
              ]}
              stickyHeader
            />
            {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={list.data?.pages.length ?? 0} firstPage={list.data?.pageParams[0] === '' && list.data.pages.length === 1} hasNextPage={Boolean(list.hasNextPage)} loading={list.isFetching} refreshing={list.isPlaceholderData} nextPageError={list.isFetchNextPageError} onNext={() => void list.fetchNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}
