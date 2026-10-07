import { ResourceUsage } from './resource/ResourceUsage'
import { useResourceMetrics, useResourceHPAs, podHPA, podResourceUsage } from './resource/resourceMetrics'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router'
import { RotateCcw, ScrollText, Trash2 } from 'lucide-react'

import {
  closePortForward,
  createIdempotencyKey,
  getEndpointsList,
  getEndpointSlices,
  getEvents,
  getIngressClasses,
  getIngresses,
  getPermissions,
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
  restartWorkload,
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
  CapabilityMatrix,
} from '../api/types'
import { Badge, Button, DataTable, StatusBadge, type DataTableColumn } from './ui'
import { ConfirmDialog } from './ui/ConfirmDialog'
import { useToast } from './ui/Toast'
import { csrfForGeneration } from '../actions/csrf'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { bindListInteraction, listInteractionFor } from '../observability/uxMetrics'
import { ResourceListControls } from './ResourceListControls'
import type { ListSortOrder } from './ResourceListControls'
import { ResourceLiveUpdates } from './ResourceLiveUpdates'

import { CollectionCoverage, InfiniteCollectionFooter, QueryState, SelectionGate } from './resource/states'
import { collectionGcTime, collectionStaleTime, useInfiniteCollection } from './resource/useInfiniteCollection'
import { podPreviewKey } from './resource/podPreview'
import { useSelectionBoundKeys } from './resource/useSelectionBoundKeys'
import { useResourceStreamPreview } from './resource/useResourceStreamPreview'
import { ResourcePage } from './resource/ResourcePage'
import { TableLink } from './resource/TableLink'
import { usePreferenceColumnVisibility } from './resource/columns'
import { age, dateTime } from './resource/format'
import { eventBadgeVariant, statusBadgeVariant } from './resource/status'
import { workloadKindPath } from '../navigation/paths'
import { useResourceWorkspace } from './workspace/ResourceWorkspaceProvider'
import { ResourceDetailPortal } from './workspace/ResourceSplitView'

const PodLogsPanel = lazy(() => import('./workspace/PodLogsPanel').then((module) => ({ default: module.PodLogsPanel })))
function podGroup(pod: Pod) { return pod.owner?.kind === 'Job' || pod.owner?.kind === 'CronJob' ? 1 : 0 }

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

function namespaceValues(value: string): string[] {
  return [...new Set(value.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean))]
}

// Mirrors the backend collection limit for one list request (MaximumNamespaces).

function optionalSort(sort: string, order: ListSortOrder, defaultSort: string, defaultOrder: ListSortOrder): { sort?: string; order?: ListSortOrder } {
  return sort === defaultSort && order === defaultOrder ? {} : { sort, order }
}

const workloadKinds = ['deployments', 'statefulsets', 'daemonsets', 'jobs', 'cronjobs', 'replicasets'] as const
const workloadStatuses = ['Healthy', 'Progressing', 'Degraded', 'Suspended', 'Completed', 'Failed', 'Unknown'] as const
const podStatuses = ['Running', 'Pending', 'Succeeded', 'Failed', 'Unknown'] as const
const restartFilters = ['any', 'gt0', 'gte3', 'gte10'] as const
const eventTypes = ['Normal', 'Warning', 'Unknown'] as const

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
  succeeded: number
  failed: Array<{ name: string; reason: string }>
}

const bulkRestartCapabilities: Partial<Record<Workload['kind'], string>> = {
  Deployment: 'deployments.restart',
  StatefulSet: 'statefulsets.restart',
  DaemonSet: 'daemonsets.restart',
}

const bulkDeleteCapabilities: Record<Workload['kind'], string> = {
  Deployment: 'deployments.delete',
  StatefulSet: 'statefulsets.delete',
  DaemonSet: 'daemonsets.delete',
  Job: 'jobs.delete',
  CronJob: 'cronjobs.delete',
  ReplicaSet: 'replicasets.delete',
}

function allows(matrix: CapabilityMatrix | undefined, capabilityId: string, namespace: string, resourceName: string): boolean {
  return matrix?.decisions.some((item) => item.capabilityId === capabilityId && item.namespace === namespace && item.resourceName === resourceName && item.decision === 'allowed') === true
}

function mutationError(error: unknown): string {
  if (error instanceof APIError) return `${error.code}: ${error.message}`
  return error instanceof Error ? error.message : 'The action could not be completed.'
}

function BulkToolbar({ count, children }: { count: number; children: React.ReactNode }) {
  return (
    <div className="mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-kp-accent-border bg-kp-accent-bg/50 px-3 py-2" role="toolbar" aria-label="Bulk actions">
      <strong className="text-content text-kp-text">{count} selected</strong>
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
  const kindPreset = useMemo(() => (kindParam && (workloadKinds as readonly string[]).includes(kindParam) ? kindParam : ''), [kindParam])
  const [draft, setDraft] = useState<WorkloadListState>(() => ({ ...workloadsStateFromParams(params), kind: kindPreset }))
  const [applied, setApplied] = useState<WorkloadListState>(() => ({ ...workloadsStateFromParams(params), kind: kindPreset }))
  const [previousKindPreset, setPreviousKindPreset] = useState(kindPreset)
  if (previousKindPreset !== kindPreset) {
    setPreviousKindPreset(kindPreset)
    setDraft((current) => ({ ...current, kind: kindPreset }))
    setApplied((current) => ({ ...current, kind: kindPreset }))
  }
  const queryClient = useQueryClient()
  const [selectedKeys, setSelectedKeys] = useSelectionBoundKeys([selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value])
  const [bulkAction, setBulkAction] = useState<'delete' | 'restart' | null>(null)

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
    { key: 'name', header: 'Name', cell: (item) => <TableLink aria-label={`Open ${item.kind} ${item.name} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'workloads', kind: item.kind, namespace: item.namespace, name: item.name })} primary={item.name} /> },
    { key: 'kind', header: 'Type', cell: (item) => item.kind },
    { key: 'ready', header: 'Ready', cell: (item) => `${item.ready ?? '—'} / ${item.desired ?? '—'}` },
    { key: 'available', header: 'Available', cell: (item) => item.available ?? '—' },
    { key: 'updated', header: 'Updated', cell: (item) => item.updated ?? '—' },
    { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
    { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
  ]
  const rowKey = useCallback((item: Workload) => `${item.kind}/${item.namespace}/${item.name}`, [])

  const collection = useInfiniteCollection<Workload>({
    identity: ['resources', 'workloads', selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value, applied.namespace],
    filters: applied,
    fetchPage: (cursor, signal, prefetch) => getWorkloads({ limit: 100, prefetch, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, namespaces: effectiveNamespaces(globalNamespace.value, namespaceValues(applied.namespace)), kinds: applied.kind ? [applied.kind] : undefined, statuses: applied.workloadStatus ? [applied.workloadStatus] : undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, generation),
    enabled: Boolean(selection),
  })
  const list = collection.query
  const preview = useResourceStreamPreview<Workload>({ identity: [selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value], topic: 'workloads', namespace: globalNamespace.value, isItem: isWorkloadPreview, itemKey: workloadPreviewKey })
  const previewActive = list.isPending && !collection.authorizationFailed && sameListState(applied, defaultWorkloadList) && Boolean(preview.preview?.items.length)
  const visibleItems = previewActive ? preview.preview!.items : collection.items
  useEffect(() => { if (collection.authorizationFailed) setSelectedKeys(new Set()) }, [collection.authorizationFailed, setSelectedKeys])
  const selectedItems = useMemo(() => collection.items.filter((item) => selectedKeys.has(rowKey(item))), [collection.items, selectedKeys, rowKey])
  const selectedCapabilityIDs = useMemo(() => Array.from(new Set(selectedItems.flatMap((item) => [bulkDeleteCapabilities[item.kind], bulkRestartCapabilities[item.kind]].filter((value): value is string => Boolean(value))))), [selectedItems])
  const bulkPermissions = useQuery({
    queryKey: ['bulk-action-permissions', generation, selectedItems.map(rowKey).join('|'), selectedCapabilityIDs.join('|')],
    queryFn: ({ signal }) => getPermissions({ namespaces: Array.from(new Set(selectedItems.map((item) => item.namespace))), capabilityIds: selectedCapabilityIDs, resourceNames: selectedItems.map((item) => item.name) }, signal, generation),
    enabled: Boolean(generation && selectedItems.length),
    staleTime: 15_000,
  })
  const canBulkDelete = selectedItems.length > 0 && selectedItems.every((item) => allows(bulkPermissions.data, bulkDeleteCapabilities[item.kind], item.namespace, item.name))
  const canBulkRestart = selectedItems.length > 0 && selectedItems.every((item) => {
    const capability = bulkRestartCapabilities[item.kind]
    return Boolean(capability && allows(bulkPermissions.data, capability, item.namespace, item.name))
  })

  const bulkDelete = useMutation({
    mutationFn: async (): Promise<BulkOutcome> => {
      const csrfToken = await csrfForGeneration(generation!)
      const outcome: BulkOutcome = { succeeded: 0, failed: [] }
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
          outcome.succeeded += 1
        } catch (error) {
          outcome.failed.push({ name: `${item.kind}/${item.name}`, reason: mutationError(error) })
        }
      }
      return outcome
    },
    onSuccess: (outcome) => {
      toast.success(`Deleted ${outcome.succeeded} workload${outcome.succeeded === 1 ? '' : 's'}`, outcome.failed.length ? `${outcome.failed.length} failed: ${outcome.failed.map((item) => item.name).join(', ')}` : 'Every selected workload was removed.')
      setBulkAction(null)
      setSelectedKeys(new Set())
      void queryClient.invalidateQueries({ queryKey: ['resources', 'workloads'] })
    },
    onError: (error) => {
      toast.error('Bulk delete failed', mutationError(error))
      setBulkAction(null)
    },
  })

  const bulkRestart = useMutation({
    mutationFn: async (): Promise<BulkOutcome> => {
      const csrfToken = await csrfForGeneration(generation!)
      const outcome: BulkOutcome = { succeeded: 0, failed: [] }
      for (const item of selectedItems) {
        const kindPath = workloadKindPath(item.kind)
        if (!kindPath || !bulkRestartCapabilities[item.kind]) {
          outcome.failed.push({ name: `${item.kind}/${item.name}`, reason: 'This workload kind does not support rollout restart.' })
          continue
        }
        try {
          const detail = await getWorkload(kindPath, item.namespace, item.name, undefined, generation)
          await restartWorkload(kindPath, item.namespace, item.name, {
            confirmed: true,
            action: 'restart',
            consequenceCode: 'RECREATE_WORKLOAD_PODS',
            target: { clusterProfileId: selection!.clusterProfileId, context: selection!.context, namespace: item.namespace, kind: item.kind, name: item.name },
            expectedGeneration: generation!,
            expectedResourceVersion: detail.metadata.resourceVersion,
          }, csrfToken, createIdempotencyKey())
          outcome.succeeded += 1
        } catch (error) {
          outcome.failed.push({ name: `${item.kind}/${item.name}`, reason: mutationError(error) })
        }
      }
      return outcome
    },
    onSuccess: (outcome) => {
      toast.success(`Restarted ${outcome.succeeded} workload${outcome.succeeded === 1 ? '' : 's'}`, outcome.failed.length ? `${outcome.failed.length} failed: ${outcome.failed.map((item) => item.name).join(', ')}` : 'Every selected controller accepted a rollout restart.')
      setBulkAction(null)
      setSelectedKeys(new Set())
      void queryClient.invalidateQueries({ queryKey: ['resources', 'workloads'] })
    },
    onError: (error) => {
      toast.error('Bulk restart failed', mutationError(error))
      setBulkAction(null)
    },
  })

  return (
    <ResourcePage
      title="Workloads"
      description="Deployments, StatefulSets, DaemonSets, Jobs and CronJobs in the active scope."
      actions={selection ? <ResourceLiveUpdates key={`workloads/${generation}`} generation={generation!} topics={['workloads']} queryKeys={[["resources", "workloads"]]} autoStart onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }} />

      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending && !previewActive} error={!list.data || collection.authorizationFailed ? list.error : null} empty={visibleItems.length === 0 && !list.hasNextPage}>
          {selectedItems.length > 0 ? (
            <BulkToolbar count={selectedItems.length}>
              <Button variant="warning" disabled={!canBulkRestart || bulkPermissions.isPending} disabledReason="Every selected workload must support rollout restart and be authorized." onClick={() => setBulkAction('restart')}><RotateCcw size={12} aria-hidden="true" /> Restart selected</Button>
              <Button variant="danger" disabled={!canBulkDelete || bulkPermissions.isPending} disabledReason="Delete must be authorized for every selected workload." onClick={() => setBulkAction('delete')}><Trash2 size={12} aria-hidden="true" /> Delete selected</Button>
              <Button variant="ghost" onClick={() => setSelectedKeys(new Set())}>Clear selection</Button>
            </BulkToolbar>
          ) : null}
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">

            {previewActive ? <p className="px-3 py-1.5 text-content text-kp-sky" role="status">Receiving workloads · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
            {list.isPlaceholderData || list.isFetching && !list.isFetchingNextPage ? <p className="sr-only" role="status">Refreshing workloads…</p> : null}
            <DataTable
              caption="Authorized workload pages"
              rows={visibleItems}
              onScrollProgress={collection.onScrollProgress}
              getRowKey={rowKey}
              columns={workloadColumns} columnVisibility={workloadColumnState}
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
            {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={list.data?.pages.length ?? 0} firstPage={list.data?.pageParams[0] === '' && list.data.pages.length === 1} hasNextPage={Boolean(list.hasNextPage)} loading={list.isFetching} refreshing={list.isPlaceholderData} nextPageError={collection.nextPageError} onNext={() => void collection.loadNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
      <ConfirmDialog
        open={bulkAction !== null}
        severity={bulkAction === 'restart' ? 'warning' : 'danger'}
        title={`${bulkAction === 'restart' ? 'Restart' : 'Delete'} ${selectedItems.length} workload${selectedItems.length === 1 ? '' : 's'}`}
        description={`Each ${bulkAction === 'restart' ? 'restart' : 'delete'} is authorized and re-validated by Kubernetes individually before it executes.`}
        resources={selectedItems.map((item) => ({ kind: item.kind, namespace: item.namespace, name: item.name }))}
        consequenceNote={bulkAction === 'restart' ? 'Each controller updates its Pod template and Kubernetes replaces the managed Pods.' : 'Dependent ReplicaSets, Pods and Jobs are garbage-collected by Kubernetes after deletion. This action cannot be undone.'}
        confirmLabel={bulkAction === 'restart' ? 'Restart selected' : 'Delete selected'}
        pendingLabel={bulkAction === 'restart' ? 'Restarting…' : 'Deleting…'}
        pending={bulkDelete.isPending || bulkRestart.isPending}
        onConfirm={() => bulkAction === 'restart' ? bulkRestart.mutate() : bulkDelete.mutate()}
        onCancel={() => setBulkAction(null)}
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
  const [aggregateTargets, setAggregateTargets] = useState<{ generation: string; pods: Pod[] } | null>(null)
  const generation = selection?.generation
  const [draft, setDraft] = useState<PodListState>(() => podsStateFromParams(params))
  const [applied, setApplied] = useState<PodListState>(() => podsStateFromParams(params))
  const queryClient = useQueryClient()
  const [selectedKeys, setSelectedKeys] = useSelectionBoundKeys([selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value])
  const [bulkAction, setBulkAction] = useState<'delete' | 'restart' | null>(null)
  const preview = useResourceStreamPreview<Pod>({ identity: [selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value], topic: 'pods', namespace: globalNamespace.value, isItem: isPodPreview, itemKey: namedPreviewKey })
  const previewNamespace = globalNamespace.value || globalNamespace.options[0] || ''
  const seedPreview = useQuery({
    queryKey: selection && previewNamespace ? podPreviewKey(selection, globalNamespace.value, previewNamespace) : ['pod-preview', 'unavailable'],
    queryFn: ({ signal }) => getPods({ limit: 20, namespaces: [previewNamespace] }, signal, generation),
    enabled: false,
    staleTime: collectionStaleTime,
    gcTime: collectionGcTime,
  })

  useEffect(() => {
    if (!paramNamespace || !paramName || !generation) return
    workspace.openFromRoute({ collection: 'pods', namespace: paramNamespace, name: paramName })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [paramNamespace, paramName, generation])

  const rowKey = useCallback((item: Pod) => `${item.namespace}/${item.name}`, [])
  const collection = useInfiniteCollection<Pod>({
    identity: ['resources', 'pods', selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value, applied.namespace],
    filters: applied,
    fetchPage: (cursor, signal, prefetch) => getPods({ limit: 100, prefetch, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, namespaces: effectiveNamespaces(globalNamespace.value, namespaceValues(applied.namespace)), statuses: applied.podStatus ? [applied.podStatus] : undefined, workload: applied.workload || undefined, node: applied.node || undefined, restarts: applied.restarts as 'any' | 'gt0' | 'gte3' | 'gte10', problematic: applied.problematic === '' ? undefined : applied.problematic === 'true', ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, generation),
    enabled: Boolean(selection),
  })
  const list = collection.query
  const listKey = collection.queryKey
  const listData = collection.lastPage
  const listItems = collection.items
  const snapshotRenewed = collection.snapshotRenewed
  const authorizationFailed = collection.authorizationFailed
  useEffect(() => { if (authorizationFailed) setSelectedKeys(new Set()) }, [authorizationFailed, setSelectedKeys])
  const seedItems = seedPreview.data?.items.filter((item) => item.namespace === previewNamespace) ?? []
  const previewActive = list.isPending && !authorizationFailed && sameListState(applied, defaultPodList) && Boolean(preview.preview?.items.length || seedItems.length)
  const visibleItems = previewActive ? preview.preview?.items.length ? preview.preview.items : seedItems : listItems
  const selectedItems = useMemo(() => listItems.filter((item) => selectedKeys.has(rowKey(item))), [listItems, selectedKeys, rowKey])
  const bulkPermissions = useQuery({
    queryKey: ['bulk-action-permissions', generation, 'pods.delete', selectedItems.map(rowKey).join('|')],
    queryFn: ({ signal }) => getPermissions({ namespaces: Array.from(new Set(selectedItems.map((item) => item.namespace))), capabilityIds: ['pods.delete'], resourceNames: selectedItems.map((item) => item.name) }, signal, generation),
    enabled: Boolean(generation && selectedItems.length),
    staleTime: 15_000,
  })
  const canBulkDelete = selectedItems.length > 0 && selectedItems.every((item) => allows(bulkPermissions.data, 'pods.delete', item.namespace, item.name))
  const canBulkRestart = canBulkDelete && selectedItems.every((item) => item.owner !== null)

  // Metrics are requested independently of the cached health badge.
  const metrics = useResourceMetrics(generation)
  const hpaCatalog = useResourceHPAs(generation, globalNamespace.value || undefined)
  const metricsByPod = useMemo(() => {
    const map = new Map<string, import('../api/types').PodMetric>()
    for (const entry of metrics.data?.block.value.pods ?? []) {
      map.set(`${entry.namespace}/${entry.pod}`, entry)
    }
    return map
  }, [metrics.data])

  const podColumnState = usePreferenceColumnVisibility('pods')
  const podColumns: DataTableColumn<Pod>[] = [
    { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
    { key: 'name', header: 'Pod', cell: (item) => <TableLink aria-label={`Open Pod ${item.name} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'pods', namespace: item.namespace, name: item.name })} primary={<>{item.name}{item.problematic ? <Badge variant="danger" className="ml-2">problem</Badge> : null}</>} /> },
    { key: 'type', header: 'Type', value: (item) => podGroup(item) ? 'Job' : 'Pod', cell: (item) => podGroup(item) ? 'Job' : 'Pod' },
    { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
    { key: 'ready', header: 'Ready', cell: (item) => `${item.ready.current}/${item.ready.desired}` },
    { key: 'restarts', header: 'Restarts', cell: (item) => item.restarts },
    { key: 'cpu', header: 'CPU', value: (item) => metricsByPod.get(rowKey(item))?.cpuMillicores, cell: (item) => { const value = metricsByPod.get(rowKey(item)); return <ResourceUsage {...podResourceUsage(item, value, 'cpu', podHPA(item, hpaCatalog))} /> } },
    { key: 'memory', header: 'Memory', value: (item) => metricsByPod.get(rowKey(item))?.memoryBytes, cell: (item) => { const value = metricsByPod.get(rowKey(item)); return <ResourceUsage {...podResourceUsage(item, value, 'memory', podHPA(item, hpaCatalog))} /> } },
    { key: 'node', header: 'Node', cell: (item) => item.node ?? '—' },
    { key: 'owner', header: 'Owner', cell: (item) => item.owner ? <span className="text-content">{item.owner.kind}/{item.owner.name}</span> : 'standalone' },
    { key: 'ip', header: 'IP', cell: (item) => item.ip ?? '—' },
    { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
  ]

  const bulkPodAction = useMutation({
    mutationFn: async (): Promise<BulkOutcome> => {
      const csrfToken = await csrfForGeneration(generation!)
      const outcome: BulkOutcome = { succeeded: 0, failed: [] }
      const restarting = bulkAction === 'restart'
      for (const item of selectedItems) {
        if (restarting && !item.owner) {
          outcome.failed.push({ name: `${item.namespace}/${item.name}`, reason: 'Standalone Pods cannot be restarted because no controller will recreate them.' })
          continue
        }
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
          outcome.succeeded += 1
        } catch (error) {
          outcome.failed.push({ name: `${item.namespace}/${item.name}`, reason: mutationError(error) })
        }
      }
      return outcome
    },
    onSuccess: (outcome) => {
      const restarted = bulkAction === 'restart'
      toast.success(`${restarted ? 'Restarted' : 'Deleted'} ${outcome.succeeded} Pod${outcome.succeeded === 1 ? '' : 's'}`, outcome.failed.length ? `${outcome.failed.length} failed: ${outcome.failed.map((item) => item.name).join(', ')}` : restarted ? 'Every selected Pod will be recreated by its controller.' : 'The selected Pods were removed.')
      setBulkAction(null)
      setSelectedKeys(new Set())
      void queryClient.invalidateQueries({ queryKey: ['resources', 'pods'] })
    },
    onError: (error) => {
      toast.error(`Bulk ${bulkAction === 'restart' ? 'restart' : 'delete'} failed`, mutationError(error))
      setBulkAction(null)
    },
  })

  return (
    <ResourcePage
      title="Pods"
      description="Pod inventory with readiness, restarts, owner, metrics and problem evidence in the active scope."
      actions={<div className="flex items-center gap-2">{selection ? <ResourceLiveUpdates key={`pods/${generation}`} generation={generation!} topics={['pods']} queryKeys={[["resources", "pods"]]} autoStart onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}</div>}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={(interactionId) => setApplied(bindListInteraction({ ...draft }, interactionId))} />

      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending && visibleItems.length === 0} error={!list.data || authorizationFailed ? list.error : null} empty={visibleItems.length === 0 && !list.hasNextPage}>
          {selectedItems.length > 0 ? (
            <BulkToolbar count={selectedItems.length}>
              <Button variant="secondary" onClick={() => {
                if (selectedItems.length === 1) { setAggregateTargets(null); workspace.openResource({ collection: 'pods', namespace: selectedItems[0].namespace, name: selectedItems[0].name }, 'logs') }
                else { workspace.reset(); setAggregateTargets({ generation: generation!, pods: selectedItems }) }
              }}><ScrollText size={12} aria-hidden="true" />{selectedItems.length === 1 ? 'View logs' : 'Aggregate logs'}</Button>
              <Button variant="warning" disabled={!canBulkRestart || bulkPermissions.isPending} disabledReason="Every selected Pod must have a controller owner and delete permission." onClick={() => setBulkAction('restart')}><RotateCcw size={12} aria-hidden="true" /> Restart selected</Button>
              <Button variant="danger" disabled={!canBulkDelete || bulkPermissions.isPending} disabledReason="Delete must be authorized for every selected Pod." onClick={() => setBulkAction('delete')}><Trash2 size={12} aria-hidden="true" /> Delete selected</Button>
              <Button variant="ghost" onClick={() => setSelectedKeys(new Set())}>Clear selection</Button>
            </BulkToolbar>
          ) : null}
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">

            {previewActive ? <p className="px-3 py-1.5 text-content text-kp-sky" role="status">Receiving Pods · ✓ {preview.preview?.items.length ? preview.preview.completed : 1}/{preview.preview?.items.length ? preview.preview.requested : selection?.namespaceCount ?? 1} namespaces · partial preview</p> : null}
            {list.isPlaceholderData || list.isFetching && !list.isFetchingNextPage ? <p className="sr-only" role="status">Refreshing Pods…</p> : null}
            <DataTable
              caption="Authorized Pod pages"
              rows={visibleItems}
              rowGroup={podGroup}
              virtualize
              getRowKey={rowKey}
              columns={podColumns} columnVisibility={podColumnState}
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
              onToggleAll={(checked, filteredRows) => {
                setSelectedKeys(checked ? new Set(filteredRows.map(rowKey)) : new Set())
              }}
              onScrollProgress={collection.onScrollProgress}
            />
            {listData ? (
              <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-kp-overlay-0 px-3 py-2.5">
                <div className="text-content text-kp-overlay-text">
                  <span className="block text-kp-subtext">{listItems.length} Pods loaded · {list.data?.pages.length ?? 0} page{list.data?.pages.length === 1 ? '' : 's'} retained</span>
                  <small className="block">{listData.page.complete ? 'Collection complete' : `Bounded ${listData.page.filterScope} result`}</small>
                  {snapshotRenewed ? <small className="block text-kp-yellow" role="status">The list snapshot expired and was renewed from the first page.</small> : null}
                  <CollectionCoverage coverage={listData.coverage} />
                </div>
                <div className="flex gap-2">
                  <Button variant="secondary" disabled={list.data?.pageParams[0] === '' && list.data?.pages.length === 1} disabledReason="Already on the first page." onClick={() => void queryClient.resetQueries({ queryKey: listKey })}>First page</Button>
                  <Button disabled={!list.hasNextPage || list.isFetching || list.isPlaceholderData} disabledReason="The current result has no next page or is refreshing." onClick={() => void collection.loadNextPage()}>{list.isFetchingNextPage ? 'Loading…' : 'Load next page'}</Button>
                </div>
                {collection.nextPageError ? <p className="w-full text-content text-kp-red" role="alert">The next page could not be loaded. The loaded Pods remain available; retry when ready.</p> : null}
              </footer>
            ) : null}
          </div>
          {metrics.isError || metrics.data?.block.errors.length ? <p className="mt-1.5 text-content text-kp-yellow" role="note">Some metrics are unavailable. Check Metrics Server and metrics.k8s.io permissions. Available samples remain visible.</p> : null}
        </QueryState>
      </SelectionGate>
      {aggregateTargets && aggregateTargets.generation === generation && selection && !workspace.open ? <ResourceDetailPortal><section className="workspace-panel" aria-label="Pod aggregate logs">
        <header className="workspace-header"><strong className="col-span-2 text-heading">Selected Pod logs</strong><Button variant="ghost" onClick={() => setAggregateTargets(null)}>Close logs</Button></header>
        <div className="workspace-body"><div className="workspace-tab-content workspace-tab-content--logs">
          <Suspense fallback={<p role="status">Opening logs…</p>}><PodLogsPanel key={`${generation}/${aggregateTargets.pods.map(rowKey).join('|')}`} pods={aggregateTargets.pods} selection={selection} /></Suspense>
        </div></div>
      </section></ResourceDetailPortal> : null}
      <ConfirmDialog
        open={bulkAction !== null}
        severity={bulkAction === 'restart' ? 'warning' : 'danger'}
        title={`${bulkAction === 'restart' ? 'Restart' : 'Delete'} ${selectedItems.length} Pod${selectedItems.length === 1 ? '' : 's'}`}
        description={`Each Pod deletion is authorized and re-validated by Kubernetes individually before it executes.`}
        resources={selectedItems.map((item) => ({ kind: 'Pod', namespace: item.namespace, name: item.name }))}
        consequenceNote={bulkAction === 'restart' ? 'Every selected Pod has an owner. Kubernetes removes each Pod and its controller creates a replacement; ephemeral local state is lost.' : 'Owned Pods may be recreated by their controllers; standalone Pods are gone for good. This action cannot be undone.'}
        confirmLabel={bulkAction === 'restart' ? 'Restart selected' : 'Delete selected'}
        pendingLabel={bulkAction === 'restart' ? 'Restarting…' : 'Deleting…'}
        pending={bulkPodAction.isPending}
        onConfirm={() => bulkPodAction.mutate()}
        onCancel={() => setBulkAction(null)}
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
    { key: 'time', header: 'Time', value: (item) => item.timestamp, cell: (item) => dateTime(item.timestamp) },
    { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
    { key: 'object', header: 'Object', cell: (item) => `${item.objectKind}/${item.objectName}` },
    { key: 'type', header: 'Type / reason', cell: (item) => <><Badge variant={eventBadgeVariant(item.type)}>{item.type}</Badge><small className="mt-0.5 block text-content text-kp-overlay-text">{item.reason}</small></> },
    { key: 'count', header: 'Count', cell: (item) => item.count },
    { key: 'message', header: 'Message', cell: (item) => <span className="block max-w-[480px] break-words text-content leading-snug">{item.message}</span> },
  ]
  const collection = useInfiniteCollection<EventResource>({
    identity: ['resources', 'events', selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value, applied.namespace],
    filters: applied,
    fetchPage: (cursor, signal, prefetch) => getEvents({ limit: 100, prefetch, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, namespaces: effectiveNamespaces(globalNamespace.value, namespaceValues(applied.namespace)), statuses: applied.eventType ? [applied.eventType] : undefined, objectKind: applied.objectKind || undefined, reason: applied.reason || undefined, continueToken: cursor || undefined, ...optionalSort(applied.sort, applied.order, 'timestamp', 'desc') }, signal, generation),
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
      actions={selection ? <ResourceLiveUpdates key={`events/${generation}`} generation={generation!} topics={['events']} queryKeys={[["resources", "events"]]} autoStart onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }} />

      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending && !previewActive} error={!list.data || collection.authorizationFailed ? list.error : null} empty={visibleItems.length === 0 && !list.hasNextPage}>
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">

            {previewActive ? <p className="px-3 py-1.5 text-content text-kp-sky" role="status">Receiving events · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
            {list.isPlaceholderData || list.isFetching && !list.isFetchingNextPage ? <p className="sr-only" role="status">Refreshing events…</p> : null}
            <DataTable
              caption="Authorized event pages"
              rows={visibleItems}
              onScrollProgress={collection.onScrollProgress}
              getRowKey={(item, index) => `${item.namespace}/${item.objectKind}/${item.objectName}/${item.timestamp ?? index}`}
              columns={eventColumns} columnVisibility={eventColumnState}
              stickyHeader
            />
            {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={list.data?.pages.length ?? 0} firstPage={list.data?.pageParams[0] === '' && list.data.pages.length === 1} hasNextPage={Boolean(list.hasNextPage)} loading={list.isFetching} refreshing={list.isPlaceholderData} nextPageError={collection.nextPageError} onNext={() => void collection.loadNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
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
  const toast = useToast()
  const { tab: tabParam, namespace: paramNamespace, name: paramName } = useParams<{ tab: string; namespace?: string; name?: string }>()
  const generation = selection?.generation
  const tab: NetworkTab = tabParam === 'port-forwards' ? 'port-forwards' : networkTabFromParams(tabParam ?? '') ?? 'services'
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
    fetchPage: (cursor, signal, prefetch) => {
      const options = { ...networkOptions(cursor), prefetch }
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
  const [forwardSearch, setForwardSearch] = useState('')
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

  const networkColumnState = usePreferenceColumnVisibility(resourceTab)
  const networkColumns: DataTableColumn<NetworkItem>[] = [
    ...(resourceTab === 'ingress-classes' ? [] : [{ key: 'namespace', header: 'Namespace', cell: (item: NetworkItem) => 'namespace' in item ? item.namespace : '—' }]),
    { key: 'name', header: 'Name', cell: (item) => <TableLink aria-label={`Open ${tab} ${item.name}${'namespace' in item ? ` in ${item.namespace}` : ''}`} onClick={() => workspace.openResource({ collection: networkCollections[resourceTab], namespace: 'namespace' in item ? item.namespace : null, name: item.name })} primary={item.name} /> },
    { key: 'type', header: 'Type', cell: (item) => ('type' in item ? item.type : 'className' in item ? (item.className ?? 'Ingress') : 'addressType' in item ? item.addressType : 'podSelector' in item ? item.podSelector || 'none' : 'controller' in item ? item.controller : '—') },
    { key: 'summary', header: 'Summary', cell: (item) => ('clusterIPs' in item ? item.clusterIPs.join(', ') : 'hosts' in item ? item.hosts.join(', ') : 'addressType' in item ? `${item.endpoints.length} endpoints` : 'readyCount' in item ? `${item.readyCount} ready / ${item.notReadyCount} not ready${item.truncated ? ' (truncated)' : ''}` : 'ruleSummary' in item ? item.ruleSummary.length + ' rules' : item.default ? 'default class' : '—') },
  ]

  return (
    <ResourcePage title="Network" description="Services, Ingresses, EndpointSlices and loopback-only port-forward sessions." actions={selection && (tab === 'services' || tab === 'ingresses' || tab === 'endpoint-slices') ? <ResourceLiveUpdates key={`${tab}/${generation}`} generation={generation!} topics={[tab]} queryKeys={[["resources", tab]]} autoStart onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}>
      {tab !== 'port-forwards' ? <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDrafts((current) => ({ ...current, [resourceTab]: { ...current[resourceTab], search: value } }))} onApply={(interactionId) => { setAppliedLists((current) => ({ ...current, [resourceTab]: bindListInteraction({ ...draft }, interactionId) })) }} /> : <ResourceListControls search={forwardSearch} appliedSearch={forwardSearch} onSearchChange={setForwardSearch} onApply={() => {}} />}
      <div id="network-panel">
        <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
          {tab === 'port-forwards' ? <QueryState pending={forwards.isPending} error={forwards.error ?? close.error} empty={forwards.data?.length === 0}>
            {stopAllState !== 'idle' ? (
              <div role="alertdialog" aria-labelledby="stop-all-title" className="grid gap-2 rounded-lg border border-kp-red-border bg-kp-red-bg/50 p-3.5">
                <strong id="stop-all-title" className="text-content text-kp-text">Close all active loopback sessions?</strong>
                <p className="m-0 text-content text-kp-subtext">Only sessions of the current selection are closed; each close re-verifies authorization and reports per session.</p>
                <div className="flex gap-2">
                  <Button variant="secondary" onClick={() => setStopAllState('idle')}>Cancel</Button>
                  <Button variant="danger" disabled={stopAll.isPending} onClick={() => stopAll.mutate()}>{stopAll.isPending ? 'Closing…' : 'Confirm close all'}</Button>
                </div>
              </div>
            ) : null}
            {stopAllResult ? <p className={stopAllResult.failed === 0 ? 'm-0 text-content text-kp-green' : 'm-0 text-content text-kp-yellow'} role="status">{stopAllResult.closed} session{stopAllResult.closed === 1 ? '' : 's'} closed{stopAllResult.failed ? ` · ${stopAllResult.failed} failed` : ''}.</p> : null}
            {forwards.data?.some((item) => item.status === 'active') && stopAllState === 'idle' ? (
              <Button variant="danger" className="justify-self-start" onClick={() => setStopAllState('confirm')}>Stop all active sessions</Button>
            ) : null}
            <DataTable caption="Loopback sessions" rows={(forwards.data ?? []).filter((item) => [item.namespace, item.pod, item.context, item.localAddress, item.localPort, item.remotePort, item.status].join(' ').toLocaleLowerCase().includes(forwardSearch.toLocaleLowerCase()))} getRowKey={(item) => item.id} columns={[
              { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
              { key: 'name', sortKey: 'pod', header: 'Pod', cell: (item) => <TableLink aria-label={`Open Pod ${item.pod} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'pods', kind: 'Pod', namespace: item.namespace, name: item.pod })} primary={item.pod} /> },
              { key: 'context', header: 'Context', cell: (item) => item.context },
              { key: 'local', header: 'Local address', cell: (item) => `${item.localAddress}:${item.localPort}` },
              { key: 'remotePort', header: 'Remote port', cell: (item) => item.remotePort },
              { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
              { key: 'createdAt', header: 'Created', cell: (item) => dateTime(item.createdAt) },
              { key: 'expiresAt', header: 'Expires', cell: (item) => dateTime(item.expiresAt) },
              { key: 'endedAt', header: 'Ended', cell: (item) => item.endedAt ? dateTime(item.endedAt) : '—' },
              { key: 'endReason', header: 'End reason', cell: (item) => item.endReason ?? '—' },
              { key: 'actions', header: 'Actions', cell: (item) => item.status === 'active' ? <Button variant="danger" onClick={() => close.mutate(item.id)}>Close loopback session</Button> : '—' },
            ]} stickyHeader />
          </QueryState> : <QueryState pending={activeQuery.isPending && !previewActive} error={!activeQuery.data || collection.authorizationFailed ? activeQuery.error : null} empty={visibleItems.length === 0 && !activeQuery.hasNextPage}>
            <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">

              {previewActive ? <p className="px-3 py-1.5 text-content text-kp-sky" role="status">Receiving {tab} · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
              {activeQuery.isPlaceholderData || activeQuery.isFetching && !activeQuery.isFetchingNextPage ? <p className="sr-only" role="status">Refreshing {tab}…</p> : null}
              <DataTable
                caption={`Authorized ${tab} pages`}
                rows={visibleItems}
                onScrollProgress={collection.onScrollProgress}
                getRowKey={(item) => `${'namespace' in item ? `${item.namespace}/` : ''}${item.name}`}
                key={resourceTab} columns={networkColumns} columnVisibility={networkColumnState}
                stickyHeader
              />
              {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={activeQuery.data?.pages.length ?? 0} firstPage={activeQuery.data?.pageParams[0] === '' && activeQuery.data.pages.length === 1} hasNextPage={Boolean(activeQuery.hasNextPage)} loading={activeQuery.isFetching} refreshing={activeQuery.isPlaceholderData} nextPageError={collection.nextPageError} onNext={() => void collection.loadNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
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

const defaultConfigLists: Record<ConfigTab, SimpleListState> = {
  configmaps: { ...defaultSimpleList },
  secrets: { ...defaultSimpleList },
}

export function ConfigPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const { tab: tabParam, namespace: paramNamespace, name: paramName } = useParams<{ tab: string; namespace?: string; name?: string }>()
  const generation = selection?.generation
  const [tabState] = useState<ConfigTab>(() => configTabFromParams(tabParam ?? '') ?? 'configmaps')
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
    fetchPage: (cursor, signal, prefetch) => {
      const options = { limit: 100, prefetch, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, continueToken: cursor || undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), namespaces: effectiveNamespaces(globalNamespace.value, []) }
      return tab === 'configmaps' ? getConfigMapsSafe(options, signal, generation) : getSecretsSafe(options, signal, generation)
    },
  })

  const configColumnState = usePreferenceColumnVisibility(tab)
  const configColumns: DataTableColumn<ConfigItem>[] = [
    { key: 'namespace', header: 'Namespace', cell: (item) => ('metadata' in item ? item.metadata : item).namespace },
    { key: 'name', header: 'Name', value: (item) => ('metadata' in item ? item.metadata : item).name, cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return <TableLink aria-label={`Open ${tab === 'secrets' ? 'Secret' : 'ConfigMap'} ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} /> } },
    { key: 'uid', header: 'UID', cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return <span className="text-content">{value.uid}</span> } },
    { key: 'created', header: 'Created', value: (item) => ('metadata' in item ? item.metadata : item).creationTimestamp, cell: (item) => { const value = 'metadata' in item ? item.metadata : item; return dateTime(value.creationTimestamp) } },
  ]
  const activeQuery = collection.query
  const preview = useResourceStreamPreview<ConfigMapResource>({ identity: [tab, selection?.clusterProfileId, selection?.context, selection?.scopeId, generation, globalNamespace.value], topic: 'configmaps', namespace: globalNamespace.value, isItem: isConfigMapPreview, itemKey: namedPreviewKey })
  const previewActive = tab === 'configmaps' && activeQuery.isPending && !collection.authorizationFailed && sameListState(applied, defaultSimpleList) && Boolean(preview.preview?.items.length)
  const visibleItems: ConfigItem[] = previewActive ? preview.preview!.items : collection.items

  return (
    <ResourcePage title="Configuration" description="ConfigMaps and Secrets in the active scope. Open a resource to inspect its data." actions={selection && tab === 'configmaps' ? <ResourceLiveUpdates key={`configmaps/${generation}`} generation={generation!} topics={['configmaps']} queryKeys={[["resources", "configmaps"]]} autoStart onProgress={preview.onProgress} onPreviewReset={preview.onReset} /> : null}>
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDrafts((current) => ({ ...current, [tab]: { ...current[tab], search: value } }))} onApply={(interactionId) => { setAppliedLists((current) => ({ ...current, [tab]: bindListInteraction({ ...draft }, interactionId) })) }} />
      <div id="config-panel">
        <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
          <QueryState pending={activeQuery.isPending && !previewActive} error={!activeQuery.data || collection.authorizationFailed ? activeQuery.error : null} empty={visibleItems.length === 0 && !activeQuery.hasNextPage}>
            <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">

              {previewActive ? <p className="px-3 py-1.5 text-content text-kp-sky" role="status">Receiving ConfigMaps · ✓ {preview.preview!.completed}/{preview.preview!.requested} namespaces · partial preview</p> : null}
              {activeQuery.isPlaceholderData || activeQuery.isFetching && !activeQuery.isFetchingNextPage ? <p className="sr-only" role="status">Refreshing {tab}…</p> : null}
              <DataTable
                caption={`Authorized ${tab} metadata pages`}
                rows={visibleItems}
                onScrollProgress={collection.onScrollProgress}
                getRowKey={(item) => { const value = 'metadata' in item ? item.metadata : item; return `${value.namespace}/${value.name}` }}
                key={tab} columns={configColumns} columnVisibility={configColumnState}
                stickyHeader
              />
              {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={activeQuery.data?.pages.length ?? 0} firstPage={activeQuery.data?.pageParams[0] === '' && activeQuery.data.pages.length === 1} hasNextPage={Boolean(activeQuery.hasNextPage)} loading={activeQuery.isFetching} refreshing={activeQuery.isPlaceholderData} nextPageError={collection.nextPageError} onNext={() => void collection.loadNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
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
  const columnVisibility = usePreferenceColumnVisibility('nodes')
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
    fetchPage: (cursor: string, signal: AbortSignal, prefetch: boolean) => getNodes({ limit: 100, prefetch, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, statuses: applied.nodeStatus ? [applied.nodeStatus] : undefined, ...optionalSort(applied.sort, applied.order, 'identity', 'asc'), continueToken: cursor || undefined }, signal, generation),
    enabled: Boolean(selection),
  })
  const list = collection.query

  return (
    <ResourcePage
      title="Nodes"
      description="Cluster nodes with readiness, roles, capacity and taints; a selected context is enough and no namespace filter applies."
    >
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft((current) => ({ ...current, search: value }))} onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={!list.data || collection.authorizationFailed ? list.error : null} empty={collection.items.length === 0 && !list.hasNextPage}>
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            {list.isPlaceholderData || list.isFetching && !list.isFetchingNextPage ? <p className="sr-only" role="status">Refreshing nodes…</p> : null}
            <DataTable
              caption="Authorized node pages"
              columnVisibility={columnVisibility}
              rows={collection.items}
              onScrollProgress={collection.onScrollProgress}
              getRowKey={(item) => item.name}
              columns={[
                { key: 'name', header: 'Node', cell: (item) => <TableLink aria-label={`Open Node ${item.name}`} onClick={() => workspace.openResource({ collection: 'nodes', name: item.name })} primary={item.name} secondary={item.roles.join(', ') || 'no role'} /> },
                { key: 'status', header: 'Status', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
                { key: 'version', header: 'Version', cell: (item) => item.kubeletVersion || '—' },
                { key: 'internal-ip', header: 'Internal IP', cell: (item) => item.internalIP ?? '—' },
                { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
              ]}
              stickyHeader
            />
            {collection.lastPage ? <InfiniteCollectionFooter result={collection.lastPage} itemCount={collection.items.length} pageCount={list.data?.pages.length ?? 0} firstPage={list.data?.pageParams[0] === '' && list.data.pages.length === 1} hasNextPage={Boolean(list.hasNextPage)} loading={list.isFetching} refreshing={list.isPlaceholderData} nextPageError={collection.nextPageError} onNext={() => void collection.loadNextPage()} onRestart={() => void queryClient.resetQueries({ queryKey: collection.queryKey })} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}
