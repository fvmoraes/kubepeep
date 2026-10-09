import { LoadingState } from './ui/LoadingState'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { bindListInteraction, listInteractionFor } from '../observability/uxMetrics'
import { useInfiniteCollection } from './resource/useInfiniteCollection'
import { ResourceCollectionTable } from './resource/ResourceCollectionTable'
import { useQuery } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router'

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
  CSIDriver,
  CSINode,
  Lease,
  PersistentVolume,
  PersistentVolumeClaim,
  StorageClass,
  VolumeAttachment,
} from '../api/types'
import { Badge, StatusBadge, type DataTableColumn } from './ui'
import { ResourceListControls } from './ResourceListControls'
import { ResourceLiveUpdates } from './ResourceLiveUpdates'
import type { ListSortOrder } from './ResourceListControls'
import { SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { usePreferenceColumnVisibility } from './resource/columns'
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

interface FamilyListProps<T> {
  caption: string
  collection: ReturnType<typeof useInfiniteCollection<T>>
  rowKey: (row: T) => string
  columns: DataTableColumn<T>[]
  columnVisibility?: import('./resource/columns').ColumnVisibilityState
  gatePending: boolean
  gateError: unknown
  gateSelected: boolean
  draft: SimpleListState
  applied: SimpleListState

  onDraft: (next: SimpleListState) => void
  onApply: (interactionId: string) => void

}

// FamilyList is the shared list/table/footer block of a family page. It keeps
// every family on the same visual structure (spec §16/§30): identical toolbar,
// table chrome, footer states. Details open in the Resource Workspace.
function FamilyList<T>(props: FamilyListProps<T>) {
  const { draft, applied } = props
  return (
    <>
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => props.onDraft({ ...draft, search: value })} onApply={props.onApply} />
      <SelectionGate pending={props.gatePending} error={props.gateError} selected={props.gateSelected}>
        <ResourceCollectionTable key={props.caption} collection={props.collection} caption={props.caption} columns={props.columns} getRowKey={props.rowKey} columnVisibility={props.columnVisibility} />
      </SelectionGate>
    </>
  )
}

export function LeasesPage() {
  const globalNamespace = useGlobalNamespace()
  const { status, selection } = useActiveSelection()
  const workspace = useResourceWorkspace()
  const { namespace, name } = useParams<{ namespace?: string; name?: string }>()
  const [params] = useSearchParamsShim()
  const generation = selection?.generation
  const [draft, setDraft] = useState<SimpleListState>(() => listStateFromParams(params, []))
  const [applied, setApplied] = useState<SimpleListState>(() => listStateFromParams(params, []))

  useEffect(() => {
    if (!namespace || !name || !generation) return
    workspace.openFromRoute({ collection: 'leases', namespace, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [namespace, name, generation])

  const collection = useInfiniteCollection({
    identity: ['resources', 'leases', generation, globalNamespace.value],
    filters: applied,
    fetchPage: (cursor, signal, prefetch, focus) => getLeases({ ...focus, limit: 100, prefetch, uxInteractionId: listInteractionFor(applied), namespaces: effectiveNamespaces(globalNamespace.value, []), search: applied.search || undefined, ...sortParams(applied), continueToken: cursor || undefined }, signal, generation),
    enabled: Boolean(selection),
  })

  const leaseColumnState = usePreferenceColumnVisibility('leases')
  const columns: DataTableColumn<Lease>[] = [
    { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
    { key: 'name', header: 'Name', cell: (item) => (
      <TableLink aria-label={`Open Lease ${item.name} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'leases', namespace: item.namespace, name: item.name })} primary={item.name} />
    ) },
    { key: 'holder', header: 'Holder', cell: (item) => item.holderName || '—' },
    { key: 'duration', header: 'Duration', cell: (item) => `${item.durationSeconds}s` },
    { key: 'renew', header: 'Renew time', cell: (item) => dateTime(item.renewTime) },
    { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
  ]

  return (
    <ResourcePage title="Leases" description="coordination.k8s.io leases with holders and renewal timing in the active scope.">

      <FamilyList<Lease>
        caption="Authorized lease page"
        collection={collection} rowKey={(item) => `${item.namespace}/${item.name}`}
        columns={columns} columnVisibility={leaseColumnState}
        gatePending={status.isPending} gateError={status.error} gateSelected={Boolean(selection)}
        draft={draft} applied={applied}

        onDraft={setDraft}
        onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }}

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

function storageTabFromParams(tab: string): StorageTab | null {
  return (storageTabs as string[]).includes(tab) ? (tab as StorageTab) : null
}

export function StoragePage() {
  const globalNamespace = useGlobalNamespace()
  const { status, selection } = useActiveSelection()
  const workspace = useResourceWorkspace()
  const { tab: tabParam, namespace, name } = useParams<{ tab?: string; namespace?: string; name?: string }>()
  const generation = selection?.generation
  const tab = useMemo(() => storageTabFromParams(tabParam ?? '') ?? 'persistent-volumes', [tabParam])
  const [drafts, setDrafts] = useState<Record<StorageTab, SimpleListState>>(() => structuredClone(Object.fromEntries(storageTabs.map((key) => [key, { ...defaultSimpleList }])) as Record<StorageTab, SimpleListState>))
  const [appliedLists, setAppliedLists] = useState<Record<StorageTab, SimpleListState>>(() => structuredClone(Object.fromEntries(storageTabs.map((key) => [key, { ...defaultSimpleList }])) as Record<StorageTab, SimpleListState>))

  const draft = drafts[tab]
  const applied = appliedLists[tab]

  // Deep links (/storage/:tab/:name or /storage/:tab/:ns/:name) open the workspace.
  useEffect(() => {
    if (!name || !generation) return
    workspace.openFromRoute({ collection: tab, namespace: namespace ?? null, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tab, namespace, name, generation])

  const fetchList = { 'persistent-volumes': getPersistentVolumes, 'persistent-volume-claims': getPersistentVolumeClaims, 'volume-attachments': getVolumeAttachments, 'storage-classes': getStorageClasses, 'csi-nodes': getCSINodes, 'csi-drivers': getCSIDrivers }[tab]
  const collection = useInfiniteCollection<StorageRow>({
    identity: ['resources', tab, generation, tab === 'persistent-volume-claims' ? globalNamespace.value : ''],
    filters: applied,
    enabled: Boolean(selection),
    fetchPage: (cursor, signal, prefetch, focus) => fetchList({ ...focus, limit: 100, prefetch, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, statuses: applied.status ? [applied.status] : undefined, namespaces: tab === 'persistent-volume-claims' ? effectiveNamespaces(globalNamespace.value, []) : undefined, continueToken: cursor || undefined, ...sortParams(applied) }, signal, generation),
  })

  function setDraft(next: SimpleListState) {
    setDrafts((current) => ({ ...current, [tab]: next }))
  }
  const openDetail = useCallback((item: { namespace?: string; name: string }) => {
    workspace.openResource({ collection: tab, namespace: item.namespace ?? null, name: item.name })
  }, [tab, workspace])

  const storageColumnState = usePreferenceColumnVisibility(tab)
  const allColumns = buildStorageColumns(tab, openDetail)
  const columns = allColumns

  return (
    <ResourcePage
		title="Storage"
		description="PersistentVolumes, claims, attachments, classes and CSI objects; claim inspection respects the active scope."
		actions={selection && tab === 'persistent-volume-claims' ? <ResourceLiveUpdates key={`persistent-volume-claims/${generation}`} generation={generation!} topics={['persistent-volume-claims']} queryKeys={[['resources', 'persistent-volume-claims']]} /> : undefined}
	>

      <FamilyList<StorageRow>
        caption={`Authorized ${tab} page`}
        collection={collection}
        rowKey={(item) => `${(item as { namespace?: string }).namespace ?? ''}/${item.name}`}
        columns={columns} columnVisibility={storageColumnState}
        gatePending={status.isPending} gateError={status.error} gateSelected={Boolean(selection)}
        draft={draft} applied={applied}

        onDraft={setDraft}
        onApply={(interactionId) => { setAppliedLists((current) => ({ ...current, [tab]: bindListInteraction({ ...draft }, interactionId) })) }}

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
    return <TableLink aria-label={`Open ${value.name}`} onClick={() => open(value)} primary={value.name} />
  } })
  switch (tab) {
    case 'persistent-volumes':
      return widen<PersistentVolume>([
        nameCell('Volume'),
        { key: 'status', header: 'Phase', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
        { key: 'capacity', header: 'Capacity', cell: (item) => item.capacity || '—' },
        { key: 'class', header: 'Class', cell: (item) => item.storageClass || '—' },
        { key: 'claim', header: 'Claim', cell: (item) => (item.claim ? `${item.claim.namespace}/${item.claim.name}` : '—') },
        { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'persistent-volume-claims':
      return widen<PersistentVolumeClaim>([
        { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
        { key: 'name', header: 'Name', cell: (item) => <TableLink aria-label={`Open claim ${item.name} in ${item.namespace}`} onClick={() => open(item)} primary={item.name} /> },
        { key: 'status', header: 'Phase', cell: (item) => <StatusBadge variant={statusBadgeVariant(item.status)}>{item.status}</StatusBadge> },
        { key: 'volume', header: 'Volume', cell: (item) => item.volumeName || '—' },
        { key: 'capacity', header: 'Capacity', cell: (item) => item.capacity ?? 'not measured' },
        { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'volume-attachments':
      return widen<VolumeAttachment>([
        nameCell('Attachment'),
        { key: 'node', header: 'Node', cell: (item) => item.nodeName },
        { key: 'attacher', header: 'Attacher', cell: (item) => item.attacher },
        { key: 'attached', header: 'Attached', cell: (item) => item.attached ? <Badge variant="healthy">attached</Badge> : <Badge variant="unknown">not attached</Badge> },
        { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'storage-classes':
      return widen<StorageClass>([
        nameCell('Class'),
        { key: 'provisioner', header: 'Provisioner', cell: (item) => item.provisioner },
        { key: 'default', header: 'Default', cell: (item) => item.default ? <Badge variant="healthy">default</Badge> : '—' },
        { key: 'binding', header: 'Binding', cell: (item) => item.volumeBindingMode || '—' },
        { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'csi-nodes':
      return widen<CSINode>([
        nameCell('CSI node'),
        { key: 'drivers', header: 'Drivers', cell: (item) => item.driverCount },
        { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
      ])
    case 'csi-drivers':
      return widen<CSIDriver>([
        nameCell('Driver'),
        { key: 'attach', header: 'Attach required', cell: (item) => item.attachRequired ? 'yes' : 'no' },
        { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
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
        {detail.isPending ? <LoadingState label="Loading namespace…" /> : detail.isError ? <p className="text-content text-kp-red" role="alert">{errorMessage(detail.error)}</p> : detail.data ? (
          <div className="grid max-w-[900px] gap-4">
            <Facts facts={[
              { label: 'Phase', value: detail.data.status || 'Unknown' },
              { label: 'UID', value: detail.data.metadata.uid },
              { label: 'Created', value: dateTime(detail.data.metadata.creationTimestamp) },
              { label: 'Labels', value: Object.entries(detail.data.metadata.labels ?? {}).map(([key, value]) => `${key}=${value}`).join(', ') || 'none' },
            ]} />
            {detail.data.conditions.length ? (
              <div className="overflow-x-auto rounded-lg border border-kp-overlay-0">
                <table className="w-full border-collapse text-left text-content">
                  <thead><tr className="border-b border-kp-overlay-0 text-column uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-bold">Condition</th><th className="px-2.5 py-1.5 font-bold">Status</th><th className="px-2.5 py-1.5 font-bold">Since</th></tr></thead>
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
            <p className="m-0 text-content text-kp-overlay-text">
              Manage this namespace in a local scope? Open the <Link to="/namespaces" className="text-kp-mauve underline">namespace scope editor</Link>; bulk registration stays available without Namespace permissions.
            </p>
          </div>
        ) : null}
      </SelectionGate>
    </ResourcePage>
  )
}
