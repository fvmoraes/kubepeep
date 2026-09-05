import { useMutation, useQuery, useQueryClient, type UseMutationResult } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'

import {
  getCSIDriver,
  getCSIDrivers,
  getCSINode,
  getCSINodes,
  getLease,
  getLeases,
  getLeaseYAML,
  getNamespaceObject,
  getPersistentVolume,
  getPersistentVolumeClaim,
  getPersistentVolumeClaims,
  getPersistentVolumeClaimYAML,
  getPersistentVolumes,
  getPersistentVolumeYAML,
  getStorageClass,
  getStorageClasses,
  getStorageClassYAML,
  getStatus,
  getVolumeAttachment,
  getVolumeAttachments,
} from '../api/client'
import type {
  CollectionResult,
  CSIDriver,
  CSIDriverDetail,
  CSINode,
  CSINodeDetail,
  Lease,
  PersistentVolume,
  PersistentVolumeClaim,
  PersistentVolumeClaimDetail,
  PersistentVolumeDetail,
  StorageClass,
  StorageClassDetail,
  VolumeAttachment,
  VolumeAttachmentDetail,
} from '../api/types'
import { Badge, DataTable, type DataTableColumn, Drawer, Select, StatusBadge } from './ui'
import { FavoriteButton } from './FavoriteButton'
import { ResourceListControls } from './ResourceListControls'
import type { ActiveListFilter, ListSortOrder, ListSortOption } from './ResourceListControls'
import { YamlViewer } from './YamlViewer'
import { errorMessage } from './resource/errors'
import { CollectionFooter, QueryState, SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import { applyColumnVisibility, ColumnVisibilityControl, usePreferenceColumnVisibility } from './resource/columns'
import { TableLink } from './resource/TableLink'
import { Facts } from './resource/Facts'
import { age, dateTime } from './resource/format'
import { statusBadgeVariant } from './resource/status'

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
  const setValue = (next: string) => setState({ generation, value: next })
  return [value, setValue]
}

function useGenerationCursorMap<K extends string>(generation: string | undefined, empty: Record<K, string>): [Record<K, string>, (key: K, value: string) => void] {
  const [state, setState] = useState<{ generation: string | undefined; values: Record<K, string> }>(() => ({ generation, values: { ...empty } }))
  const values = state.generation === generation ? state.values : empty
  const setValue = (key: K, value: string) => setState((current) => ({
    generation,
    values: { ...(current.generation === generation ? current.values : empty), [key]: value },
  }))
  return [values, setValue]
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
  children?: React.ReactNode
}

// FamilyList is the shared list/table/footer block of a family page. It keeps
// every family on the same visual structure (spec §16/§30): identical toolbar,
// table chrome, footer states and drawer layout.
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
          <label>Status<Select value={draft.status} onChange={(event) => props.onDraft({ ...draft, status: event.target.value })}><option value="">All statuses</option>{props.statuses.map((value) => <option key={value}>{value}</option>)}</Select></label>
        ) : null}
      </ResourceListControls>
      <SelectionGate pending={props.gatePending} error={props.gateError} selected={props.gateSelected}>
        <QueryState pending={props.queryPending} error={props.queryError} empty={props.result?.items.length === 0}>
          <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
            <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
              <DataTable caption={props.caption} rows={props.rows} getRowKey={props.rowKey} columns={props.columns} />
              {props.result ? <CollectionFooter result={props.result} onNext={props.onNext} onRestart={props.onRestart} /> : null}
            </div>
            {props.children}
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
  const { status, selection } = useActiveSelection()
  const { namespace, name } = useParams<{ namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const [params] = useSearchParamsShim()
  const [draft, setDraft] = useState<SimpleListState>(() => listStateFromParams(params, ['']))
  const [applied, setApplied] = useState<SimpleListState>(() => listStateFromParams(params, ['']))
  const [cursor, setCursor] = useGenerationCursor(selection?.generation)
  const [selected, setSelected] = useState<{ generation: string; namespace: string; name: string } | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const paramSelection = namespace && name && selection ? { generation: selection.generation, namespace, name } : null
  const activeSelected = paramSelection || (selected?.generation === selection?.generation ? selected : null)
  const list = useQuery({
    queryKey: ['resources', 'leases', selection?.generation, applied, cursor],
    queryFn: ({ signal }) => getLeases({ limit: 100, search: applied.search || undefined, ...sortParams(applied), continueToken: cursor || undefined }, signal, selection?.generation),
    enabled: Boolean(selection),
  })
  const detail = useQuery({
    queryKey: ['resources', 'lease-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name],
    queryFn: ({ signal }) => getLease(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation),
    enabled: Boolean(selection && activeSelected),
  })
  const yaml = useMutation({ mutationFn: (value: { namespace: string; name: string }) => requests.run((signal) => getLeaseYAML(value.namespace, value.name, signal)) })

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
    navigate('/leases')
  }

  const leaseColumnState = usePreferenceColumnVisibility('leases')
  const columns: DataTableColumn<Lease>[] = [
    { key: 'namespace', header: 'Namespace / name', cell: (item) => (
      <TableLink aria-label={`Open Lease ${item.name} in ${item.namespace}`} onClick={() => { requests.abortAll(); setSelected({ generation: selection!.generation, namespace: item.namespace, name: item.name }); yaml.reset(); navigate(`/leases/${encodeURIComponent(item.namespace)}/${encodeURIComponent(item.name)}`) }} primary={item.name} secondary={item.namespace} />
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
        onApply={() => { setApplied(draft); setCursor(''); closeDetail() }}
        onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'leases'] })}
        onClear={() => { setDraft({ ...defaultSimpleList }); setApplied({ ...defaultSimpleList }); setCursor(''); closeDetail() }}
        onSort={(value) => setDraft((current) => ({ ...current, sort: value }))}
        onOrder={(value) => setDraft((current) => ({ ...current, order: value }))}
        onNext={(next) => { setCursor(next); closeDetail() }}
        onRestart={() => { setCursor(''); closeDetail() }}
      >
        <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={detailTitle(activeSelected ? `Lease ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}>
          {activeSelected ? <>
            {detail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading detail…</p> : detail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(detail.error)}</p> : detail.data ? (
              <Facts facts={[
                { label: 'Holder', value: detail.data.holderName || 'none' },
                { label: 'Duration', value: `${detail.data.durationSeconds}s` },
                { label: 'Renew time', value: dateTime(detail.data.renewTime) },
                { label: 'Acquire time', value: dateTime(detail.data.acquireTime) },
                { label: 'UID', value: detail.data.metadata.uid },
              ]} />
            ) : null}
            <YamlViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate({ namespace: activeSelected.namespace, name: activeSelected.name })} />
          </> : null}
        </Drawer>
      </FamilyList>
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

interface StorageSelection {
  generation: string
  namespace: string | null
  name: string
}

// Cluster-scoped storage tabs support metadata-only favorites (V6-03).
const clusterFavoriteKinds: Partial<Record<StorageTab, 'persistentvolume' | 'storageclass'>> = {
  'persistent-volumes': 'persistentvolume',
  'storage-classes': 'storageclass',
}

export function StoragePage() {
  const { status, selection } = useActiveSelection()
  const { tab: tabParam, namespace, name } = useParams<{ tab?: string; namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const tab = useMemo(() => storageTabFromParams(tabParam ?? '') ?? 'persistent-volumes', [tabParam])
  const [drafts, setDrafts] = useState<Record<StorageTab, SimpleListState>>(() => structuredClone(Object.fromEntries(storageTabs.map((key) => [key, { ...defaultSimpleList }])) as Record<StorageTab, SimpleListState>))
  const [appliedLists, setAppliedLists] = useState<Record<StorageTab, SimpleListState>>(() => structuredClone(Object.fromEntries(storageTabs.map((key) => [key, { ...defaultSimpleList }])) as Record<StorageTab, SimpleListState>))
  const [cursors, setCursorValue] = useGenerationCursorMap(selection?.generation, Object.fromEntries(storageTabs.map((key) => [key, ''])) as Record<StorageTab, string>)
  const [selected, setSelected] = useState<StorageSelection | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const statuses = tab === 'persistent-volumes' ? volumeStatuses : tab === 'persistent-volume-claims' ? claimStatuses : ['']
  const draft = drafts[tab]
  const applied = appliedLists[tab]
  const paramSelection = name && selection ? { generation: selection.generation, namespace: namespace ?? null, name } : null
  const activeSelected = paramSelection || (selected?.generation === selection?.generation ? selected : null)

  const options = (value: StorageTab) => ({ limit: 100, search: appliedLists[value].search || undefined, statuses: appliedLists[value].status ? [appliedLists[value].status] : undefined, continueToken: cursors[value] || undefined, ...sortParams(appliedLists[value]) })
  const persistentVolumes = useQuery({ queryKey: ['resources', 'persistent-volumes', selection?.generation, appliedLists['persistent-volumes'], cursors['persistent-volumes']], queryFn: ({ signal }) => getPersistentVolumes(options('persistent-volumes'), signal, selection?.generation), enabled: Boolean(selection && tab === 'persistent-volumes') })
  const claims = useQuery({ queryKey: ['resources', 'persistent-volume-claims', selection?.generation, appliedLists['persistent-volume-claims'], cursors['persistent-volume-claims']], queryFn: ({ signal }) => getPersistentVolumeClaims(options('persistent-volume-claims'), signal, selection?.generation), enabled: Boolean(selection && tab === 'persistent-volume-claims') })
  const volumeAttachments = useQuery({ queryKey: ['resources', 'volume-attachments', selection?.generation, appliedLists['volume-attachments'], cursors['volume-attachments']], queryFn: ({ signal }) => getVolumeAttachments(options('volume-attachments'), signal, selection?.generation), enabled: Boolean(selection && tab === 'volume-attachments') })
  const storageClasses = useQuery({ queryKey: ['resources', 'storage-classes', selection?.generation, appliedLists['storage-classes'], cursors['storage-classes']], queryFn: ({ signal }) => getStorageClasses(options('storage-classes'), signal, selection?.generation), enabled: Boolean(selection && tab === 'storage-classes') })
  const csiNodes = useQuery({ queryKey: ['resources', 'csi-nodes', selection?.generation, appliedLists['csi-nodes'], cursors['csi-nodes']], queryFn: ({ signal }) => getCSINodes(options('csi-nodes'), signal, selection?.generation), enabled: Boolean(selection && tab === 'csi-nodes') })
  const csiDrivers = useQuery({ queryKey: ['resources', 'csi-drivers', selection?.generation, appliedLists['csi-drivers'], cursors['csi-drivers']], queryFn: ({ signal }) => getCSIDrivers(options('csi-drivers'), signal, selection?.generation), enabled: Boolean(selection && tab === 'csi-drivers') })

  const pvDetail = useQuery({ queryKey: ['resources', 'pv-detail', selection?.generation, activeSelected?.name], queryFn: ({ signal }) => getPersistentVolume(activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'persistent-volumes') })
  const claimDetail = useQuery({ queryKey: ['resources', 'pvc-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getPersistentVolumeClaim(activeSelected!.namespace!, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'persistent-volume-claims') })
  const attachmentDetail = useQuery({ queryKey: ['resources', 'va-detail', selection?.generation, activeSelected?.name], queryFn: ({ signal }) => getVolumeAttachment(activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'volume-attachments') })
  const classDetail = useQuery({ queryKey: ['resources', 'sc-detail', selection?.generation, activeSelected?.name], queryFn: ({ signal }) => getStorageClass(activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'storage-classes') })
  const csiNodeDetail = useQuery({ queryKey: ['resources', 'csinode-detail', selection?.generation, activeSelected?.name], queryFn: ({ signal }) => getCSINode(activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'csi-nodes') })
  const csiDriverDetail = useQuery({ queryKey: ['resources', 'csidriver-detail', selection?.generation, activeSelected?.name], queryFn: ({ signal }) => getCSIDriver(activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'csi-drivers') })

  const pvYaml = useMutation({ mutationFn: (value: StorageSelection) => requests.run((signal) => getPersistentVolumeYAML(value.name, signal)) })
  const claimYaml = useMutation({ mutationFn: (value: StorageSelection) => requests.run((signal) => getPersistentVolumeClaimYAML(value.namespace!, value.name, signal)) })
  const classYaml = useMutation({ mutationFn: (value: StorageSelection) => requests.run((signal) => getStorageClassYAML(value.name, signal)) })

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
  function closeDetail() {
    requests.abortAll()
    pvYaml.reset()
    claimYaml.reset()
    classYaml.reset()
    setSelected(null)
    navigate(`/storage/${tab}`)
  }
  function setCursor(value: string) {
    setCursorValue(tab, value)
    closeDetail()
  }
  function openDetail(item: { namespace?: string; name: string }) {
    requests.abortAll()
    pvYaml.reset()
    claimYaml.reset()
    classYaml.reset()
    setSelected({ generation: selection!.generation, namespace: item.namespace ?? null, name: item.name })
    const encodedName = encodeURIComponent(item.name)
    const encodedNamespace = item.namespace ? `${encodeURIComponent(item.namespace)}/` : ''
    navigate(`/storage/${tab}/${encodedNamespace}${encodedName}`)
  }

  const storageColumnState = usePreferenceColumnVisibility(`storage/${tab}`)
  const columns = applyColumnVisibility(buildStorageColumns(tab, openDetail), storageColumnState)

  return (
    <ResourcePage title="Storage" description="PersistentVolumes, claims, attachments, classes and CSI objects; claim inspection respects the active scope.">
      <ColumnVisibilityControl state={storageColumnState} columns={columns} />
      <ResourceTabStrip ariaLabel="Storage resource type" panelId="storage-panel" active={tab} onChange={(value) => {
        requests.abortAll()
        pvYaml.reset()
        claimYaml.reset()
        classYaml.reset()
        setSelected(null)
        navigate(`/storage/${value}`)
      }} tabs={storageTabs.map((id) => ({ id, label: id }))} />
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
        onApply={() => { setAppliedLists((current) => ({ ...current, [tab]: draft })); setCursor(''); closeDetail() }}
        onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })}
        onClear={() => { setDraft({ ...defaultSimpleList }); setAppliedLists((current) => ({ ...current, [tab]: { ...defaultSimpleList } })); setCursor(''); closeDetail() }}
        onSort={(value) => setDraft({ ...draft, sort: value })}
        onOrder={(value) => setDraft({ ...draft, order: value })}
        onNext={setCursor}
        onRestart={() => setCursor('')}
      >
        <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={<span className="flex items-center gap-2">{detailTitle(activeSelected ? `Storage ${activeSelected.namespace ? `${activeSelected.namespace}/` : ''}${activeSelected.name}` : 'Resource detail')}{activeSelected && clusterFavoriteKinds[tab] ? <FavoriteButton kind={clusterFavoriteKinds[tab]} name={activeSelected.name} generation={selection?.generation} label={tab} /> : null}</span>}>
          {activeSelected ? renderStorageDetail(tab, activeSelected, {
            pending: pvDetail.isPending, error: pvDetail.error, data: pvDetail.data as PersistentVolumeDetail | undefined,
            claimPending: claimDetail.isPending, claimError: claimDetail.error, claimData: claimDetail.data as PersistentVolumeClaimDetail | undefined,
            attachmentPending: attachmentDetail.isPending, attachmentError: attachmentDetail.error, attachmentData: attachmentDetail.data as VolumeAttachmentDetail | undefined,
            classPending: classDetail.isPending, classError: classDetail.error, classData: classDetail.data as StorageClassDetail | undefined,
            csiNodePending: csiNodeDetail.isPending, csiNodeError: csiNodeDetail.error, csiNodeData: csiNodeDetail.data as CSINodeDetail | undefined,
            csiDriverPending: csiDriverDetail.isPending, csiDriverError: csiDriverDetail.error, csiDriverData: csiDriverDetail.data as CSIDriverDetail | undefined,
          }, { pvYaml, claimYaml, classYaml }) : null}
        </Drawer>
      </FamilyList>
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

interface StorageDetailProps {
  pending: boolean; error: unknown; data: PersistentVolumeDetail | undefined
  claimPending: boolean; claimError: unknown; claimData: PersistentVolumeClaimDetail | undefined
  attachmentPending: boolean; attachmentError: unknown; attachmentData: VolumeAttachmentDetail | undefined
  classPending: boolean; classError: unknown; classData: StorageClassDetail | undefined
  csiNodePending: boolean; csiNodeError: unknown; csiNodeData: CSINodeDetail | undefined
  csiDriverPending: boolean; csiDriverError: unknown; csiDriverData: CSIDriverDetail | undefined
}

function renderStorageDetail(tab: StorageTab, target: StorageSelection, props: StorageDetailProps, yaml: { pvYaml: MutationState; claimYaml: MutationState; classYaml: MutationState }) {
  const loading = <p className="text-sm text-kp-overlay-text" role="status">Loading detail…</p>
  switch (tab) {
    case 'persistent-volumes':
      return <>
        {props.pending ? loading : props.error ? <p className="text-sm text-kp-red" role="alert">{errorMessage(props.error)}</p> : props.data ? <Facts facts={[
          { label: 'Phase', value: props.data.status },
          { label: 'Reclaim policy', value: props.data.reclaimPolicy || 'unknown' },
          { label: 'Volume mode', value: props.data.volumeMode || 'unknown' },
          { label: 'Claim', value: props.data.claim ? `${props.data.claim.namespace}/${props.data.claim.name}` : 'none' },
          { label: 'Reason', value: props.data.reason ?? '—' },
        ]} /> : null}
        {props.data ? <YamlViewer value={yaml.pvYaml.data} pending={yaml.pvYaml.isPending} error={yaml.pvYaml.error} onLoad={() => yaml.pvYaml.mutate(target)} /> : null}
      </>
    case 'persistent-volume-claims':
      return <>
        {props.claimPending ? loading : props.claimError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(props.claimError)}</p> : props.claimData ? <Facts facts={[
          { label: 'Phase', value: props.claimData.status },
          { label: 'Volume', value: props.claimData.volumeName || 'unbound' },
          { label: 'Storage class', value: props.claimData.storageClass ?? '—' },
          { label: 'Access modes', value: props.claimData.accessModes.join(', ') || 'none' },
          { label: 'Capacity', value: props.claimData.capacity ? Object.values(props.claimData.capacity).join(', ') : 'not measured' },
        ]} /> : null}
        {props.claimData ? <YamlViewer value={yaml.claimYaml.data} pending={yaml.claimYaml.isPending} error={yaml.claimYaml.error} onLoad={() => yaml.claimYaml.mutate(target)} /> : null}
      </>
    case 'volume-attachments':
      return props.attachmentPending ? loading : props.attachmentError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(props.attachmentError)}</p> : props.attachmentData ? (
        <>
          <Facts facts={[
            { label: 'Node', value: props.attachmentData.nodeName },
            { label: 'Attacher', value: props.attachmentData.attacher },
            { label: 'Persistent volume', value: props.attachmentData.persistentVolumeName || 'inline spec' },
            { label: 'Attached', value: props.attachmentData.attached ? 'yes' : 'no' },
          ]} />
          <p className="mt-3 rounded-r-md border-l-2 border-kp-yellow-border bg-kp-yellow-bg px-3 py-2 text-sm text-kp-yellow">Attachment metadata, raw driver errors and inline volume specs are intentionally unavailable.</p>
        </>
      ) : null
    case 'storage-classes':
      return <>
        {props.classPending ? loading : props.classError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(props.classError)}</p> : props.classData ? <Facts facts={[
          { label: 'Provisioner', value: props.classData.provisioner },
          { label: 'Reclaim policy', value: props.classData.reclaimPolicy || 'unknown' },
          { label: 'Binding mode', value: props.classData.volumeBindingMode || 'unknown' },
          { label: 'Volume expansion', value: props.classData.allowVolumeExpansion ? 'allowed' : 'not allowed' },
          { label: 'Omitted', value: props.classData.omitted.join(', ') },
        ]} /> : null}
        {props.classData ? <YamlViewer value={yaml.classYaml.data} pending={yaml.classYaml.isPending} error={yaml.classYaml.error} onLoad={() => yaml.classYaml.mutate(target)} /> : null}
      </>
    case 'csi-nodes':
      return props.csiNodePending ? loading : props.csiNodeError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(props.csiNodeError)}</p> : props.csiNodeData ? (
        <Facts facts={[
          { label: 'Registered drivers', value: String(props.csiNodeData.driverCount) },
          { label: 'Driver names', value: props.csiNodeData.drivers.map((driver) => driver.name).join(', ') || 'none' },
          { label: 'Truncated', value: props.csiNodeData.truncated ? 'yes' : 'no' },
        ]} />
      ) : null
    case 'csi-drivers':
      return props.csiDriverPending ? loading : props.csiDriverError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(props.csiDriverError)}</p> : props.csiDriverData ? (
        <Facts facts={[
          { label: 'Attach required', value: props.csiDriverData.attachRequired ? 'yes' : 'no' },
          { label: 'Pod info on mount', value: props.csiDriverData.podInfoOnMount ? 'yes' : 'no' },
          { label: 'Storage capacity', value: props.csiDriverData.storageCapacity ? 'yes' : 'no' },
          { label: 'FSGroup policy', value: props.csiDriverData.fsGroupPolicy || 'unknown' },
        ]} />
      ) : null
  }
}

type MutationState = UseMutationResult<string, Error, StorageSelection>

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
