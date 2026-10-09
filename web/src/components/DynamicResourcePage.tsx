import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { ArrowLeft, ArrowRight, MoreHorizontal, Pin, Plus, X } from 'lucide-react'
import { getDynamicResources, getPreferences, getResourceDiscovery, getStatus } from '../api/client'
import type { CustomViewContext, DynamicResource, DynamicRow, SelectionSummary } from '../api/types'
import { mutatePreferences } from '../api/preferences'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { dynamicCollection, dynamicColumnKey, dynamicListPath, parseDynamicCollection } from '../navigation/dynamic'
import { Button, Input, type DataTableColumn } from './ui'
import { TableMenu } from './ui/TableMenu'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceCollectionTable } from './resource/ResourceCollectionTable'
import { ResourceListControls } from './ResourceListControls'
import { SelectionGate } from './resource/states'
import { useInfiniteCollection } from './resource/useInfiniteCollection'
import { usePreferenceColumnVisibility } from './resource/columns'
import { TableLink } from './resource/TableLink'
import { age } from './resource/format'
import { errorMessage } from './resource/errors'
import { useResourceWorkspace } from './workspace/ResourceWorkspaceProvider'

function sameContext(value: CustomViewContext, selection: SelectionSummary): boolean {
  return value.clusterProfileId === selection.clusterProfileId && value.context === selection.context && value.cluster === selection.cluster
}
type ViewChange = { selection: SelectionSummary; resource: DynamicResource; action: 'add' | 'remove' | 'left' | 'right' }

export function DynamicResourcePage() {
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  const selection = status.data?.selection
  return <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
    {selection ? <ContextResourceViews key={`${selection.clusterProfileId}/${selection.context}/${selection.cluster}`} selection={selection} /> : null}
  </SelectionGate>
}

function ContextResourceViews({ selection }: { selection: SelectionSummary }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const params = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal), staleTime: 60_000 })
  const views = preferences.data?.customViews?.find(value => sameContext(value, selection))?.items ?? []
  const routeResource = params.resource ? parseDynamicCollection(`dynamic:${params.group}:${params.version}:${params.resource}:${params.scope}`) : null
  const selected = (routeResource && views.find(value => dynamicCollection(value) === dynamicCollection(routeResource))) || routeResource || views[0]
  const save = useMutation({
    mutationFn: ({ selection, resource, action }: ViewChange) => mutatePreferences(current => {
      const contexts = current.customViews ?? []
      const items = [...(contexts.find(value => sameContext(value, selection))?.items ?? [])]
      const index = items.findIndex(value => dynamicCollection(value) === dynamicCollection(resource))
      if (action === 'add' && index < 0) items.push(resource)
      if (action === 'remove' && index >= 0) items.splice(index, 1)
      const offset = action === 'left' ? -1 : action === 'right' ? 1 : 0
      if (offset && index >= 0 && index + offset >= 0 && index + offset < items.length) [items[index], items[index + offset]] = [items[index + offset], items[index]]
      const next = contexts.filter(value => !sameContext(value, selection))
      if (items.length) next.push({ clusterProfileId: selection.clusterProfileId, context: selection.context, cluster: selection.cluster, items })
      return { ...current, customViews: next }
    }),
    onSuccess: saved => queryClient.setQueryData(['preferences'], saved),
  })
  const closeDialog = () => setSearchParams(current => { const next = new URLSearchParams(current); next.delete('add'); return next }, { replace: true })
  async function add(resource: DynamicResource) {
    await save.mutateAsync({ selection, resource, action: 'add' })
    navigate(dynamicListPath(resource))
  }
  async function change(resource: DynamicResource, action: ViewChange['action']) {
    try {
      await save.mutateAsync({ selection, resource, action })
      if (action === 'remove' && selected && dynamicCollection(resource) === dynamicCollection(selected)) navigate('/workloads/custom')
    } catch { /* The mutation error remains visible next to the views. */ }
  }
  return <ResourcePage title="Custom resources">
    <div className="flex shrink-0 flex-wrap items-center gap-2">
      <p className="m-0 min-w-0 flex-1 truncate text-content text-kp-overlay-text" title={selection.context}>Views saved for {selection.context}</p>
      <Button variant="secondary" onClick={() => setSearchParams({ add: '1' })} disabled={!preferences.data || views.length >= 32}><Plus size={14} aria-hidden="true" /> Add custom resource</Button>
    </div>
    {preferences.isError ? <p role="alert" className="text-content text-kp-red">{errorMessage(preferences.error)} <Button onClick={() => void preferences.refetch()}>Retry preferences</Button></p> : null}
    {save.isError ? <p role="alert" className="text-content text-kp-red">Could not save these views. {errorMessage(save.error)}</p> : null}
    {views.length > 0 ? <nav aria-label="Pinned resource views" className="flex shrink-0 gap-2 overflow-x-auto border-b border-kp-overlay-0 pb-2">
      {views.map((resource, index) => <div className="flex shrink-0 items-center" key={dynamicCollection(resource)}>
        <Link to={dynamicListPath(resource)} aria-current={selected && dynamicCollection(selected) === dynamicCollection(resource) ? 'page' : undefined} className={`control px-2 py-1 text-content ${selected && dynamicCollection(selected) === dynamicCollection(resource) ? 'bg-kp-surface-2 text-kp-text' : 'text-kp-overlay-text'}`} title={`${resource.group || 'core'}/${resource.version}`}>{resource.kind} <span className="text-kp-overlay-text">{resource.group || 'core'}</span></Link>
        <TableMenu label={`Manage ${resource.kind} ${resource.group || 'core'} ${resource.version}`} icon={<MoreHorizontal size={14} />}>
          <button type="button" disabled={save.isPending || index === 0} onClick={() => void change(resource, 'left')}><ArrowLeft size={14} aria-hidden="true" /> Move left</button>
          <button type="button" disabled={save.isPending || index === views.length - 1} onClick={() => void change(resource, 'right')}><ArrowRight size={14} aria-hidden="true" /> Move right</button>
          <button type="button" disabled={save.isPending} onClick={() => void change(resource, 'remove')}>Remove view</button>
        </TableMenu>
      </div>)}
    </nav> : null}
    {selected ? <DynamicInventory key={dynamicCollection(selected)} resource={selected} generation={selection.generation} /> : <div className="grid flex-1 content-center justify-items-center gap-3 p-6 text-center">
      <Pin size={24} className="text-kp-overlay-text" aria-hidden="true" />
      <p className="m-0 text-heading text-kp-text">Your resource views, in this context</p>
      <p className="m-0 max-w-lg text-content text-kp-overlay-text">Add a Kubernetes resource or CRD to browse it here. Search by name, kind, short name or API group.</p>
    </div>}
    {searchParams.get('add') === '1' ? <ResourceDiscoveryDialog key={selection.generation} selection={selection} views={views} busy={save.isPending} onClose={closeDialog} onAdd={add} /> : null}
  </ResourcePage>
}

function DynamicInventory({ resource, generation }: { resource: DynamicResource; generation: string }) {
  const params = useParams()
  const namespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const [search, setSearch] = useState('')
  const [applied, setApplied] = useState('')
  const collectionId = dynamicCollection(resource)
  useEffect(() => {
    if (params.name) workspace.openFromRoute({ collection: collectionId, kind: resource.kind, namespace: resource.namespaced ? params.namespace : null, name: params.name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only explicit route identity should open the overlay
  }, [collectionId, generation, params.name, params.namespace, resource.kind, resource.namespaced])
  const collection = useInfiniteCollection<DynamicRow>({ identity: ['resources', collectionId, generation, resource.namespaced ? namespace.value : 'cluster'], filters: applied, enabled: Boolean(generation) && (!resource.namespaced || namespace.ready), fetchPage: (cursor, signal, prefetch, focus) => getDynamicResources(resource, { ...focus, limit: 100, search: applied || undefined, continueToken: cursor || undefined, prefetch, namespaces: resource.namespaced ? effectiveNamespaces(namespace.value, []) : undefined }, signal, generation) })
  const visibility = usePreferenceColumnVisibility(collectionId)
  const columns = useMemo<DataTableColumn<DynamicRow>[]>(() => {
    const definitions = collection.items.find(item => item.columns?.length)?.columns ?? []
    return [
      ...(resource.namespaced ? [{ key: 'namespace', header: 'Namespace', cell: (item: DynamicRow) => item.namespace }] : []),
      { key: 'name', header: 'Name', cell: item => <TableLink primary={item.name} aria-label={`Open ${item.kind} ${item.name}`} onClick={() => workspace.openResource({ collection: collectionId, kind: item.kind, namespace: item.namespace || null, name: item.name })} /> },
      ...definitions.flatMap((column): DataTableColumn<DynamicRow>[] => {
        if (['name', 'namespace', 'age'].includes(column.name.toLowerCase()) || column.format === 'name') return []
        const key = dynamicColumnKey(column)
        const cell = (item: DynamicRow) => item.cells[item.columns.findIndex(candidate => dynamicColumnKey(candidate) === key)]
        return [{ key, header: column.name, initialRole: column.name.toLowerCase(), defaultHidden: (column.priority ?? 0) > 0, cell: item => cell(item) ?? '—', value: item => ['integer', 'number'].includes(column.type) && cell(item) !== undefined && Number.isFinite(Number(cell(item))) ? Number(cell(item)) : cell(item) }]
      }),
      { key: 'kind', header: 'Type', cell: item => item.kind },
      { key: 'age', header: 'Age', cell: item => age(item.ageSeconds) },
    ]
  }, [collection.items, resource.namespaced, collectionId, workspace])
  return <>
    <p className="m-0 shrink-0 text-content text-kp-overlay-text">{resource.group || 'core'}/{resource.version} · {resource.resource} · {resource.namespaced ? 'Namespaced' : 'Cluster'}</p>
    <ResourceListControls search={search} appliedSearch={applied} onSearchChange={setSearch} onApply={() => setApplied(search)} />
    <ResourceCollectionTable collection={collection} columnVisibility={visibility} caption={`${resource.kind} resources`} columns={columns} getRowKey={item => `${item.namespace || ''}/${item.name}`} />
  </>
}

function ResourceDiscoveryDialog({ selection, views, busy, onClose, onAdd }: { selection: SelectionSummary; views: DynamicResource[]; busy: boolean; onClose: () => void; onAdd: (resource: DynamicResource) => Promise<void> }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(100)
  const [saveError, setSaveError] = useState('')
  const queryClient = useQueryClient()
  const queryKey = ['resource-discovery', selection.generation]
  const catalog = useQuery({ queryKey, queryFn: ({ signal }) => getResourceDiscovery(false, signal, selection.generation), staleTime: 300_000 })
  const refresh = useMutation({ mutationFn: () => getResourceDiscovery(true, undefined, selection.generation), onSuccess: value => queryClient.setQueryData(queryKey, value) })
  useEffect(() => { const element = dialog.current; element?.showModal(); return () => element?.close() }, [])
  const needle = search.trim().toLowerCase()
  const matches = (catalog.data?.resources ?? []).filter(resource => [resource.resource, resource.kind, resource.group, resource.version, ...(resource.shortNames ?? [])].some(value => value.toLowerCase().includes(needle)))
  async function pin(resource: DynamicResource) { setSaveError(''); try { await onAdd(resource) } catch (error) { setSaveError(errorMessage(error)) } }
  return <dialog ref={dialog} className="resource-discovery-dialog" aria-labelledby="resource-discovery-title" onCancel={event => { event.preventDefault(); onClose() }}>
    <header className="flex items-center justify-between gap-3"><h2 id="resource-discovery-title" className="m-0 text-title">Add custom resource</h2><Button variant="icon" aria-label="Close resource discovery" onClick={onClose}><X size={16} aria-hidden="true" /></Button></header>
    <p className="m-0 text-content text-kp-overlay-text">Choose a resource served by {selection.context}. Adding a view saves a shortcut for this context.</p>
    <Input autoFocus type="search" aria-label="Search available resources" placeholder="Name, kind, short name or API group…" value={search} onChange={event => { setSearch(event.target.value); setLimit(100) }} />
    <div className="flex items-center justify-between gap-3 text-content"><span role="status">{catalog.isFetching || refresh.isPending ? 'Discovering resources…' : `${matches.length} ${matches.length === 1 ? 'resource' : 'resources'} available`}</span><Button variant="secondary" disabled={catalog.isFetching || refresh.isPending} onClick={() => refresh.mutate()}>Refresh catalog</Button></div>
    {catalog.isError || refresh.isError || saveError ? <p role="alert" className="m-0 text-content text-kp-red">{saveError || errorMessage(refresh.error || catalog.error)}</p> : null}
    {catalog.data?.failures.length ? <details className="text-content text-kp-yellow"><summary>Some API groups could not be discovered ({catalog.data.failures.length})</summary>{catalog.data.failures.map((failure, index) => <p key={index}>{failure.groupVersion}: {failure.message} ({failure.code})</p>)}</details> : null}
    {catalog.data?.truncated ? <p role="note" className="m-0 text-content text-kp-yellow">The discovery limit was reached. Only the first resources are shown.</p> : null}
    <ul className="resource-discovery-results">
      {matches.slice(0, limit).map(resource => { const pinned = views.some(view => dynamicCollection(view) === dynamicCollection(resource)); return <li key={dynamicCollection(resource)}>
        <div className="min-w-0"><strong className="block break-words text-content text-kp-text">{resource.kind} <span className="font-normal text-kp-subtext">{resource.resource}</span></strong><span className="block break-all text-content text-kp-overlay-text">{resource.group || 'core'}/{resource.version} · {resource.namespaced ? 'Namespaced' : 'Cluster'}{resource.shortNames?.length ? ` · ${resource.shortNames.join(', ')}` : ''}</span></div>
        <Button variant="secondary" disabled={pinned || busy || views.length >= 32} aria-label={`Pin ${resource.kind} ${resource.group || 'core'} ${resource.version}`} onClick={() => void pin(resource)}><Pin size={14} aria-hidden="true" />{pinned ? 'Pinned' : 'Pin'}</Button>
      </li> })}
      {!catalog.isPending && !catalog.isError && matches.length === 0 ? <li className="text-content text-kp-overlay-text">No matching resources. Try another name or refresh the catalog.</li> : null}
    </ul>
    {matches.length > limit ? <Button variant="secondary" onClick={() => setLimit(value => value + 100)}>Show more resources</Button> : null}
  </dialog>
}
