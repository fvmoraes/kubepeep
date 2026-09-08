import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { useGenerationCursor, useGenerationCursorMap } from './resource/useListCursor'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'

import {
  getCSIDrivers,
  getCSINodes,
  getLeases,
  getNamespaceObject,
  getPersistentVolumeClaims,
  getPersistentVolumes,
  getStorageClasses,
  getStatus,
  getVolumeAttachments,
} from '../api/client'
import type {
  CollectionResult,
  CSIDriver,
  CSINode,
  Lease,
  PersistentVolume,
  PersistentVolumeClaim,
  StorageClass,
  VolumeAttachment,
} from '../api/types'
import { Badge, DataTable, Select, StatusBadge, type DataTableColumn } from './ui'
import { ResourceListControls } from './ResourceListControls'
import type { ActiveListFilter, ListSortOrder, ListSortOption } from './ResourceListControls'
import { CollectionFooter, QueryState, SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import { applyColumnVisibility, ColumnVisibilityControl, usePreferenceColumnVisibility } from './resource/columns'
import { TableLink } from './resource/TableLink'
import { Facts } from './resource/Facts'
import { errorMessage } from './resource/errors'
import { age, dateTime } from './resource/format'
import { statusBadgeVariant } from './resource/status'
import { useResourceWorkspace } from './workspace/ResourceWorkspaceProvider'

function useActiveSelection() {
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  return { status, selection: status.data?.selection ?? null }
}



interface SimpleListState {
  search: string
  status: string
  sort: string
  order: ListSortOrder
}

const defaultSimpleList: SimpleListState = { search: '', status: '', sort: 'identity', order: 'asc' }

function listStateFromParams(params: URLSearchParams, statuses: readonly string[]): SimpleListState {
  const status = params.get('status') ?? ''
  return { ...defaultSimpleList, search: params.get('search') ?? '', status: statuses.includes(status) ? status : '' }
}

function sameListState<T extends object>(left: T, right: T): boolean {
  return (Object.keys(left) as Array<keyof T>).every((key) => left[key] === right[key])
}

function activeStatusFilter(status: string): ActiveListFilter[] {
  return status === '' ? [] : [{ id: 'status', label: 'Status', value: status }]
}

interface FamilyListProps<T> {
  caption: string
  rows: T[]
  rowKey: (row: T) => string
  columns: DataTableColumn<T>[]
  gatePending: boolean
  gateError: unknown
  gateSelected: boolean
  queryPending: boolean
  queryError: unknown
  result: CollectionResult<T> | undefined
  draft: SimpleListState
  applied: SimpleListState
  statuses: readonly string[]
  sortOptions: readonly ListSortOption[]
  defaultSort: string
  defaultOrder: ListSortOrder
  onDraft: (next: SimpleListState) => void
  onApply: () => void
  onRefresh: () => void
  onClear: () => void
  onSort: (value: string) => void
  onOrder: (value: ListSortOrder) => void
  onNext: (cursor: string) => void
  onRestart: () => void
}

// FamilyList is the shared list/table/footer block of a family page. It keeps
// every family on the same visual structure (spec §16/§30): identical toolbar,
// table chrome, footer states. Details open in the Resource Workspace.
function FamilyList<T>(props: FamilyListProps<T>) {
  const { draft, applied } = props
  return (
    <>
      <ResourceListControls
        search={draft.search} appliedSearch={applied.search}
        onSearchChange={(value) => props.onDraft({ ...draft, search: value })}
        onApply={props.onApply}
        onRefresh={props.onRefresh}
        onClear={props.onClear}
        activeFilters={activeStatusFilter(applied.status)}
        sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order}
        defaultSort={props.defaultSort} defaultOrder={props.defaultOrder}
        hasPendingChanges={!sameListState(draft, applied)}
        sortOptions={props.sortOptions}
        onSortChange={props.onSort}
        onOrderChange={props.onOrder}
      >
        {props.statuses.length ? (
          <Select aria-label="Status" className="!h-7 !w-auto max-w-[9rem] pr-6 text-sm" value={draft.status} onChange={(event) => props.onDraft({ ...draft, status: event.target.value })}><option value="">All statuses</option>{props.statuses.map((value) => <option key={value}>{value}</option>)}</Select>
        ) : null}
      </ResourceListControls>
      <SelectionGate pending={props.gatePending} error={props.gateError} selected={props.gateSelected}>
        <QueryState pending={props.queryPending} error={props.queryError} empty={props.result?.items.length === 0}>
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            <DataTable caption={props.caption} rows={props.rows} getRowKey={props.rowKey} columns={props.columns} stickyHeader />
            {props.result ? <CollectionFooter result={props.result} onNext={props.onNext} onRestart={props.onRestart} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </>
  )
}

const leaseSortOptions: readonly ListSortOption[] = [
  { value: 'identity', label: 'Namespace and name' },
  { value: 'name', label: 'Name' },
  { value: 'age', label: 'Age' },
]

export function LeasesPage() {
  const globalNamespace = useGlobalNamespace()
  const { status, selection } = useActiveSelection()
  const workspace = useResourceWorkspace()
  const { namespace, name } = useParams<{ namespace?: string; name?: string }>()
  const [params] = useSearchParamsShim()
  const generation = selection?.generation
  const [draft, setDraft] = useState<SimpleListState>(() => listStateFromParams(params, ['']))
  const [applied, setApplied] = useState<SimpleListState>(() => listStateFromParams(params, ['']))
  const [cursor, setCursor] = useGenerationCursor(generation, globalNamespace.value)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!namespace || !name || !generation) return
    workspace.openFromRoute({ collection: 'leases', namespace, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [namespace, name, generation])

  const list = useQuery({
    queryKey: ['resources', 'leases', generation, globalNamespace.value, applied, cursor],
    queryFn: ({ signal }) => getLeases({ limit: 100, namespaces: effectiveNamespaces(globalNamespace.value, []), search: applied.search || undefined, ...sortParams(applied), continueToken: cursor || undefined }, signal, generation),
    enabled: Boolean(selection),
  })

  const leaseColumnState = usePreferenceColumnVisibility('leases')
  const columns: DataTableColumn<Lease>[] = [
    { key: 'namespace', header: 'Namespace / name', cell: (item) => (
      <TableLink aria-label={`Open Lease ${item.name} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'leases', namespace: item.namespace, name: item.name })} primary={item.name} secondary={item.namespace} />
    ) },
    { key: 'holder', header: 'Holder', cell: (item) => item.holderName || '—' },
    { key: 'duration', header: 'Duration', cell: (item) => `${item.durationSeconds}s` },
    { key: 'renew', header: 'Renew time', cell: (item) => dateTime(item.renewTime) },
    { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
  ]

  return (
    <ResourcePage title="Leases" description="coordination.k8s.io leases with holders and renewal timing in the active scope.">
      <ColumnVisibilityControl state={leaseColumnState} columns={columns} />
      <FamilyList<Lease>
        caption="Authorized lease page"
        rows={list.data?.items ?? []} rowKey={(item) => `${item.namespace}/${item.name}`}
        columns={applyColumnVisibility(columns, leaseColumnState)}
        gatePending={status.isPending} gateError={status.error} gateSelected={Boolean(selection)}
        queryPending={list.isPending} queryError={list.error} result={list.data}
        draft={draft} applied={applied} statuses={[]}
        sortOptions={leaseSortOptions} defaultSort="identity" defaultOrder="asc"
        onDraft={setDraft}
        onApply={() => { setApplied(draft); setCursor('') }}
        onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'leases'] })}
        onClear={() => { setDraft({ ...defaultSimpleList }); setApplied({ ...defaultSimpleList }); setCursor('') }}
        onSort={(value) => setDraft((current) => ({ ...current, sort: value }))}
        onOrder={(value) => setDraft((current) => ({ ...current, order: value }))}
        onNext={setCursor}
        onRestart={() => setCursor('')}
      />
    </ResourcePage>
  )
}

function useSearchParamsShim(): [URLSearchParams] {
  // Minimal window-based params; avoids a router dependency inside the shared
  // family config while deep links still prefill the applied filters.
  return [new URLSearchParams(window.location.search)]
}

function sortParams(applied: SimpleListState) {
  return applied.sort === 'identity' && applied.order === 'asc' ? { sort: undefined, order: undefined } : { sort: applied.sort, order: applied.order }
}

type StorageTab = 'persistent-volumes' | 'persistent-volume-claims' | 'volume-attachments' | 'storage-classes' | 'csi-nodes' | 'csi-drivers'

// StorageRow is the union of list DTOs across the storage tabs; each tab maps
// to one concrete shape and the columns only project its own fields.
type StorageRow = PersistentVolume | PersistentVolumeClaim | VolumeAttachment | StorageClass | CSINode | CSIDriver

const storageTabs: StorageTab[] = ['persistent-volumes', 'persistent-volume-claims', 'volume-attachments', 'storage-classes', 'csi-nodes', 'csi-drivers']
const volumeStatuses = ['Available', 'Bound', 'Released', 'Failed', 'Pending'] as const
const claimStatuses = ['Bound', 'Pending', 'Lost'] as const
const storageSortOptions: readonly ListSortOption[] = [
  { value: 'identity', label: 'Name' },
  { value: 'name', label: 'Name (natural)' },
  { value: 'age', label: 'Age' },
]
const volumeSortOptions: readonly ListSortOption[] = [
  ...storageSortOptions,
  { value: 'status', label: 'Status' },
]

function storageTabFromParams(tab: string): StorageTab | null {
  return (storageTabs as string[]).includes(tab) ? (tab as StorageTab) : null
}

export function StoragePage() {
  const globalNamespace = useGlobalNamespace()
  const { status, selection } = useActiveSelection()
  const workspace = useResourceWorkspace()
  const navigate = useNavigate()
  const { tab: tabParam, namespace, name } = useParams<{ tab?: string; namespace?: string; name?: string }>()
  const generation = selection?.generation
  const tab = useMemo(() => storageTabFromParams(tabParam ?? '') ?? 'persistent-volumes', [tabParam])
  const [drafts, setDrafts] = useState<Record<StorageTab, SimpleListState>>(() => structuredClone(Object.fromEntries(storageTabs.map((key) => [key, { ...defaultSimpleList }])) as Record<StorageTab, SimpleListState>))
  const [appliedLists, setAppliedLists] = useState<Record<StorageTab, SimpleListState>>(() => structuredClone(Object.fromEntries(storageTabs.map((key) => [key, { ...defaultSimpleList }])) as Record<StorageTab, SimpleListState>))
  const [cursors, setCursorValue] = useGenerationCursorMap(generation, Object.fromEntries(storageTabs.map((key) => [key, ''])) as Record<StorageTab, string>)
  const [claimCursor, setClaimCursor] = useGenerationCursor(generation, globalNamespace.value)
  const queryClient = useQueryClient()
  const statuses = tab === 'persistent-volumes' ? volumeStatuses : tab === 'persistent-volume-claims' ? claimStatuses : ['']
  const draft = drafts[tab]
  const applied = appliedLists[tab]

  // Deep links (/storage/:tab/:name or /storage/:tab/:ns/:name) open the workspace.
  useEffect(() => {
    if (!name || !generation) return
    workspace.openFromRoute({ collection: tab, namespace: namespace ?? null, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tab, namespace, name, generation])

  const options = (value: StorageTab) => ({ limit: 100, search: appliedLists[value].search || undefined, statuses: appliedLists[value].status ? [appliedLists[value].status] : undefined, continueToken: cursors[value] || undefined, ...sortParams(appliedLists[value]) })
  const persistentVolumes = useQuery({ queryKey: ['resources', 'persistent-volumes', generation, appliedLists['persistent-volumes'], cursors['persistent-volumes']], queryFn: ({ signal }) => getPersistentVolumes(options('persistent-volumes'), signal, generation), enabled: Boolean(selection && tab === 'persistent-volumes') })
  const claims = useQuery({ queryKey: ['resources', 'persistent-volume-claims', generation, globalNamespace.value, appliedLists['persistent-volume-claims'], claimCursor], queryFn: ({ signal }) => getPersistentVolumeClaims({ ...options('persistent-volume-claims'), namespaces: effectiveNamespaces(globalNamespace.value, []), continueToken: claimCursor || undefined }, signal, generation), enabled: Boolean(selection && tab === 'persistent-volume-claims') })
  const volumeAttachments = useQuery({ queryKey: ['resources', 'volume-attachments', generation, appliedLists['volume-attachments'], cursors['volume-attachments']], queryFn: ({ signal }) => getVolumeAttachments(options('volume-attachments'), signal, generation), enabled: Boolean(selection && tab === 'volume-attachments') })
  const storageClasses = useQuery({ queryKey: ['resources', 'storage-classes', generation, appliedLists['storage-classes'], cursors['storage-classes']], queryFn: ({ signal }) => getStorageClasses(options('storage-classes'), signal, generation), enabled: Boolean(selection && tab === 'storage-classes') })
  const csiNodes = useQuery({ queryKey: ['resources', 'csi-nodes', generation, appliedLists['csi-nodes'], cursors['csi-nodes']], queryFn: ({ signal }) => getCSINodes(options('csi-nodes'), signal, generation), enabled: Boolean(selection && tab === 'csi-nodes') })
  const csiDrivers = useQuery({ queryKey: ['resources', 'csi-drivers', generation, appliedLists['csi-drivers'], cursors['csi-drivers']], queryFn: ({ signal }) => getCSIDrivers(options('csi-drivers'), signal, generation), enabled: Boolean(selection && tab === 'csi-drivers') })

  const active: CollectionResult<StorageRow> | undefined =
    tab === 'persistent-volumes' ? (persistentVolumes.data as CollectionResult<StorageRow> | undefined) :
    tab === 'persistent-volume-claims' ? (claims.data as CollectionResult<StorageRow> | undefined) :
    tab === 'volume-attachments' ? (volumeAttachments.data as CollectionResult<StorageRow> | undefined) :
    tab === 'storage-classes' ? (storageClasses.data as CollectionResult<StorageRow> | undefined) :
    tab === 'csi-nodes' ? (csiNodes.data as CollectionResult<StorageRow> | undefined) : (csiDrivers.data as CollectionResult<StorageRow> | undefined)
  const activeQuery = tab === 'persistent-volumes' ? persistentVolumes : tab === 'persistent-volume-claims' ? claims : tab === 'volume-attachments' ? volumeAttachments : tab === 'storage-classes' ? storageClasses : tab === 'csi-nodes' ? csiNodes : csiDrivers

  function setDraft(next: SimpleListState) {
    setDrafts((current) => ({ ...current, [tab]: next }))
  }
  function setCursor(value: string) {
    if (tab === 'persistent-volume-claims') setClaimCursor(value)
    else setCursorValue(tab, value)
  }
  const openDetail = useCallback((item: { namespace?: string; name: string }) => {
    workspace.openResource({ collection: tab, namespace: item.namespace ?? null, name: item.name })
  }, [tab, workspace])

  const storageColumnState = usePreferenceColumnVisibility(`storage/${tab}`)
  const columns = applyColumnVisibility(buildStorageColumns(tab, openDetail), storageColumnState)

  return (
    <ResourcePage title="Storage" description="PersistentVolumes, claims, attachments, classes and CSI objects; claim inspection respects the active scope.">
      <ColumnVisibilityControl state={storageColumnState} columns={columns} />
      <ResourceTabStrip ariaLabel="Storage resource type" panelId="storage-panel" active={tab} onChange={(value) => navigate(`/storage/${value}`)} tabs={storageTabs.map((id) => ({ id, label: id }))} />
      <FamilyList<StorageRow>
        caption={`Authorized ${tab} page`}
        rows={active?.items ?? []}
        rowKey={(item) => `${(item as { namespace?: string }).namespace ?? ''}/${item.name}`}
        columns={columns}
        gatePending={status.isPending} gateError={status.error} gateSelected={Boolean(selection)}
        queryPending={activeQuery.isPending} queryError={activeQuery.error} result={active as CollectionResult<StorageRow> | undefined}
        draft={draft} applied={applied} statuses={[...statuses]}
        sortOptions={tab === 'persistent-volumes' || tab === 'persistent-volume-claims' ? volumeSortOptions : storageSortOptions}
        defaultSort="identity" defaultOrder="asc"
        onDraft={setDraft}
        onApply={() => { setAppliedLists((current) => ({ ...current, [tab]: draft })); setCursor('') }}
        onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })}
        onClear={() => { setDraft({ ...defaultSimpleList }); setAppliedLists((current) => ({ ...current, [tab]: { ...defaultSimpleList } })); setCursor('') }}
        onSort={(value) => setDraft({ ...draft, sort: value })}
        onOrder={(value) => setDraft({ ...draft, order: value })}
        onNext={setCursor}
        onRestart={() => setCursor('')}
      />
    </ResourcePage>
  )
}

function buildStorageColumns(tab: StorageTab, open: (item: { namespace?: string; name: string }) => void): DataTableColumn<StorageRow>[] {
  // Each tab is a concrete DTO at runtime; the shared FamilyList boundary is
  // the only place where the tab-specific column type is widened.
  const widen = <T,>(columns: DataTableColumn<T>[]) => columns as unknown as DataTableColumn<StorageRow>[]
  const nameCell = (header: string) => ({ key: 'name', header, cell: (item: StorageRow) => {
    const value = item as { namespace?: string; name: string }
    return <TableLink aria-label={`Open ${value.name}`} onClick={() => open(value)} primary={value.name} secondary={value.namespace ?? 'cluster'} />
  } })
  switch (tab) {
    case 'persistent-volumes':
      return widen<PersistentVolume>([
        nameCell('Volume'),
        { key: 'status', header: 'Phase', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
        { key: 'capacity', header: 'Capacity', cell: (item) => item.capacity || '—' },
        { key: 'class', header: 'Class', cell: (item) => item.storageClass || '—' },
        { key: 'claim', header: 'Claim', cell: (item) => (item.claim ? `${item.claim.namespace}/${item.claim.name}` : '—') },
        { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'persistent-volume-claims':
      return widen<PersistentVolumeClaim>([
        { key: 'namespace', header: 'Namespace / name', cell: (item) => <TableLink aria-label={`Open claim ${item.name} in ${item.namespace}`} onClick={() => open(item)} primary={item.name} secondary={item.namespace} /> },
        { key: 'status', header: 'Phase', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
        { key: 'volume', header: 'Volume', cell: (item) => item.volumeName || '—' },
        { key: 'capacity', header: 'Capacity', cell: (item) => item.capacity ?? 'not measured' },
        { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'volume-attachments':
      return widen<VolumeAttachment>([
        nameCell('Attachment'),
        { key: 'node', header: 'Node', cell: (item) => item.nodeName },
        { key: 'attacher', header: 'Attacher', cell: (item) => item.attacher },
        { key: 'attached', header: 'Attached', cell: (item) => item.attached ? <Badge variant="healthy">attached</Badge> : <Badge variant="unknown">not attached</Badge> },
        { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'storage-classes':
      return widen<StorageClass>([
        nameCell('Class'),
        { key: 'provisioner', header: 'Provisioner', cell: (item) => item.provisioner },
        { key: 'default', header: 'Default', cell: (item) => item.default ? <Badge variant="healthy">default</Badge> : '—' },
        { key: 'binding', header: 'Binding', cell: (item) => item.volumeBindingMode || '—' },
        { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'csi-nodes':
      return widen<CSINode>([
        nameCell('CSI node'),
        { key: 'drivers', header: 'Drivers', cell: (item) => item.driverCount },
        { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'csi-drivers':
      return widen<CSIDriver>([
        nameCell('Driver'),
        { key: 'attach', header: 'Attach required', cell: (item) => item.attachRequired ? 'yes' : 'no' },
        { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
  }
}

// NamespaceObjectPage is the V2-01 object inspection, deliberately separate
// from the scope editor at /namespaces: it reads the cluster object with its
// own authorization and never creates or edits scopes.
export function NamespaceObjectPage() {
  const { status, selection } = useActiveSelection()
  const { name } = useParams<{ name: string }>()
  const detail = useQuery({
    queryKey: ['resources', 'namespace-object', selection?.generation, name],
    queryFn: ({ signal }) => getNamespaceObject(name!, signal, selection!.generation),
    enabled: Boolean(selection && name),
  })
  return (
    <ResourcePage title={`Namespace ${name}`} description="Cluster Namespace object inspection. Managing local scopes is a separate journey and keeps working without this permission.">
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        {detail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading namespace object…</p> : detail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(detail.error)}</p> : detail.data ? (
          <div className="grid max-w-[900px] gap-4">
            <Facts facts={[
              { label: 'Phase', value: detail.data.status || 'Unknown' },
              { label: 'UID', value: detail.data.metadata.uid },
              { label: 'Created', value: dateTime(detail.data.metadata.creationTimestamp) },
              { label: 'Labels', value: Object.entries(detail.data.metadata.labels ?? {}).map(([key, value]) => `${key}=${value}`).join(', ') || 'none' },
            ]} />
            {detail.data.conditions.length ? (
              <div className="overflow-x-auto rounded-lg border border-kp-overlay-0">
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
            <p className="m-0 text-sm text-kp-overlay-text">
              Manage this namespace in a local scope? Open the <Link to="/namespaces" className="text-kp-mauve underline">namespace scope editor</Link>; bulk registration stays available without Namespace permissions.
            </p>
          </div>
        ) : null}
      </SelectionGate>
    </ResourcePage>
  )
}
