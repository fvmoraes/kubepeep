import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'

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
import { PodActions, WorkloadActions } from './ResourceActions'
import { ResourceLiveUpdates } from './ResourceLiveUpdates'
import { SavedFilterControls } from './SavedFilterControls'
import { StatePanel } from './StatePanel'

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

function SearchBar({ value, onChange, onApply, onRefresh, children }: { value: string; onChange: (value: string) => void; onApply: () => void; onRefresh: () => void; children?: ReactNode }) {
  return (
    <form className="resource-filters" onSubmit={(event) => { event.preventDefault(); onApply() }}>
      <label className="resource-search">Search this bounded page<input value={value} maxLength={256} onChange={(event) => onChange(event.target.value)} /></label>
      {children}
      <div className="resource-filter-actions">
        <button className="button" type="submit">Apply filters</button>
        <button className="button button--secondary" type="button" onClick={onRefresh}>Refresh</button>
      </div>
    </form>
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
        <button type="button" className="button button--secondary button--compact" onClick={onRestart}>First page</button>
        <button type="button" className="button button--compact" disabled={!result.page.next} onClick={() => onNext(result.page.next)}>Next page</button>
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

function DetailCard({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return (
    <aside className="resource-detail" aria-label={`${title} details`}>
      <div className="resource-detail-heading"><h2>{title}</h2><button type="button" className="button button--secondary button--compact" onClick={onClose}>Close</button></div>
      {children}
    </aside>
  )
}

function YAMLViewer({ value, pending, error, onLoad }: { value?: string; pending: boolean; error: unknown; onLoad: () => void }) {
  return (
    <section className="yaml-viewer" aria-label="Authorized YAML">
      <button type="button" className="button button--secondary button--compact" disabled={pending} onClick={onLoad}>{pending ? 'Loading YAML…' : 'Load authorized YAML'}</button>
      {error ? <p className="field-error">{errorMessage(error)}</p> : null}
      {value !== undefined ? <pre aria-label="YAML document">{value}</pre> : <p>YAML is fetched only after this explicit action and remains in memory.</p>}
    </section>
  )
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

function workloadKindPath(kind: Workload['kind']): string {
  return ({ Deployment: 'deployments', StatefulSet: 'statefulsets', DaemonSet: 'daemonsets', Job: 'jobs', CronJob: 'cronjobs' } as const)[kind]
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

const workloadKinds = ['deployments', 'statefulsets', 'daemonsets', 'jobs', 'cronjobs'] as const
const workloadStatuses = ['Healthy', 'Progressing', 'Degraded', 'Suspended', 'Completed', 'Failed', 'Unknown'] as const
const podStatuses = ['Running', 'Pending', 'Succeeded', 'Failed', 'Unknown'] as const
const restartFilters = ['any', 'gt0', 'gte3', 'gte10'] as const
const eventTypes = ['Normal', 'Warning', 'Unknown'] as const

export function WorkloadsPage() {
  const { status, selection } = useActiveSelection()
  const [search, setSearch] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [namespace, setNamespace] = useState('')
  const [kind, setKind] = useState('')
  const [workloadStatus, setWorkloadStatus] = useState('')
  const [cursor, setCursor] = useState('')
  const [selected, setSelected] = useState<GenerationSelection<Workload> | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const activeSelected = selected && selected.generation === selection?.generation ? selected.item : null
  const list = useQuery({
    queryKey: ['resources', 'workloads', selection?.generation, appliedSearch, namespace, kind, workloadStatus, cursor],
    queryFn: ({ signal }) => getWorkloads({ limit: 100, search: appliedSearch || undefined, namespaces: namespaceValues(namespace), kinds: kind ? [kind] : undefined, statuses: workloadStatus ? [workloadStatus] : undefined, continueToken: cursor || undefined }, signal, selection?.generation),
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
  }

  return (
    <div className="resource-page">
      <PageHeading title="Workloads" description="Deployments, StatefulSets, DaemonSets, Jobs and CronJobs in the active scope." action={selection ? <ResourceLiveUpdates key={`workloads/${selection.generation}`} generation={selection.generation} topics={['workloads']} queryKeys={[["resources", "workloads"]]} /> : null} />
      <SearchBar value={search} onChange={setSearch} onApply={() => { setCursor(''); setAppliedSearch(search); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'workloads'] })}>
        <label>Namespace<input value={namespace} maxLength={256} placeholder="active scope; comma-separated" onChange={(event) => setNamespace(event.target.value)} /></label>
        <label>Kind<select value={kind} onChange={(event) => setKind(event.target.value)}><option value="">All kinds</option><option value="deployments">Deployments</option><option value="statefulsets">StatefulSets</option><option value="daemonsets">DaemonSets</option><option value="jobs">Jobs</option><option value="cronjobs">CronJobs</option></select></label>
        <label>Status<select value={workloadStatus} onChange={(event) => setWorkloadStatus(event.target.value)}><option value="">All statuses</option>{workloadStatuses.map((value) => <option key={value}>{value}</option>)}</select></label>
      </SearchBar>
      {selection ? <SavedFilterControls collection="workloads" generation={selection.generation} currentQuery={compactFilterQuery([
        ['namespace', namespaceValues(namespace)], ['search', search], ['kind', kind ? [kind] : []], ['status', workloadStatus ? [workloadStatus] : []],
      ])} onApply={(query) => {
        const nextSearch = savedString(query, 'search')
        setNamespace(savedNamespaces(query))
        setSearch(nextSearch)
        setAppliedSearch(nextSearch)
        setKind(savedFirst(query, 'kind', workloadKinds))
        setWorkloadStatus(savedFirst(query, 'status', workloadStatuses))
        setCursor('')
        closeDetail()
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}>
          <div className="resource-layout">
            <div className="resource-table-wrap">
              <table className="resource-table"><caption>Authorized workload page</caption><thead><tr><th>Namespace</th><th>Kind / name</th><th>Ready</th><th>Status</th><th>Age</th></tr></thead>
                <tbody>{list.data?.items.map((item) => <tr key={`${item.kind}/${item.namespace}/${item.name}`}><td>{item.namespace}</td><td><button type="button" className="table-link" aria-label={`Open ${item.kind} ${item.name} in ${item.namespace}`} onClick={() => { requests.abortAll(); setSelected({ generation: selection!.generation, item }); yaml.reset() }}><strong>{item.name}</strong><small>{item.kind}</small></button></td><td>{item.ready ?? '—'} / {item.desired ?? '—'}</td><td><span className={`resource-status resource-status--${item.status.toLowerCase()}`}>{item.status}</span></td><td>{age(item.ageSeconds)}</td></tr>)}</tbody>
              </table>
              {list.data ? <CollectionFooter result={list.data} onNext={(next) => { setCursor(next); closeDetail() }} onRestart={() => { setCursor(''); closeDetail() }} /> : null}
            </div>
            {activeSelected ? <DetailCard title={`${activeSelected.kind} ${activeSelected.namespace}/${activeSelected.name}`} onClose={closeDetail}>
              {detail.isPending ? <p>Loading detail…</p> : detail.isError ? <p className="field-error">{errorMessage(detail.error)}</p> : detail.data ? <>
                <dl className="resource-facts"><div><dt>Status</dt><dd>{detail.data.status}</dd></div><div><dt>Resource version</dt><dd>{detail.data.metadata.resourceVersion}</dd></div><div><dt>Containers</dt><dd>{detail.data.containers.map((value) => value.name).join(', ') || 'none'}</dd></div><div><dt>Labels</dt><dd>{Object.entries(detail.data.metadata.labels ?? {}).map(([label, value]) => `${label}=${value}`).join(', ') || 'none'}</dd></div></dl>
                <WorkloadActions key={`${selection!.generation}/${detail.data.metadata.uid}`} detail={detail.data} selection={selection as SelectionSummary} />
              </> : null}
              <YAMLViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} />
            </DetailCard> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </div>
  )
}

export function PodsPage() {
  const { status, selection } = useActiveSelection()
  const [search, setSearch] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [namespace, setNamespace] = useState('')
  const [podStatus, setPodStatus] = useState('')
  const [workload, setWorkload] = useState('')
  const [node, setNode] = useState('')
  const [restarts, setRestarts] = useState('any')
  const [problematic, setProblematic] = useState('')
  const [cursor, setCursor] = useState('')
  const [selected, setSelected] = useState<GenerationSelection<Pod> | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const activeSelected = selected && selected.generation === selection?.generation ? selected.item : null
  const list = useQuery({
    queryKey: ['resources', 'pods', selection?.generation, appliedSearch, namespace, podStatus, workload, node, restarts, problematic, cursor],
    queryFn: ({ signal }) => getPods({ limit: 100, search: appliedSearch || undefined, namespaces: namespaceValues(namespace), statuses: podStatus ? [podStatus] : undefined, workload: workload || undefined, node: node || undefined, restarts: restarts as 'any' | 'gt0' | 'gte3' | 'gte10', problematic: problematic === '' ? undefined : problematic === 'true', continueToken: cursor || undefined }, signal, selection?.generation),
    enabled: Boolean(selection),
  })
  const detail = useQuery({ queryKey: ['resources', 'pod-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getPod(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(selection && activeSelected) })
  const yaml = useMutation({ mutationFn: ({ namespace, name }: Pod) => requests.run((signal) => getPodYAML(namespace, name, signal)) })

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
  }

  return (
    <div className="resource-page">
      <PageHeading title="Pods" description="Bounded Pod inventory with readiness, restarts, owner and problem evidence." action={<div className="resource-header-actions"><Link className="button button--secondary" to="/logs">Open logs</Link>{selection ? <ResourceLiveUpdates key={`pods/${selection.generation}`} generation={selection.generation} topics={['pods']} queryKeys={[["resources", "pods"]]} /> : null}</div>} />
      <SearchBar value={search} onChange={setSearch} onApply={() => { setCursor(''); setAppliedSearch(search); closeDetail() }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'pods'] })}>
        <label>Namespace<input value={namespace} maxLength={256} placeholder="active scope; comma-separated" onChange={(event) => setNamespace(event.target.value)} /></label>
        <label>Workload owner<input value={workload} maxLength={256} placeholder="exact owner name" onChange={(event) => setWorkload(event.target.value)} /></label>
        <label>Node<input value={node} maxLength={256} onChange={(event) => setNode(event.target.value)} /></label>
        <label>Status<select value={podStatus} onChange={(event) => setPodStatus(event.target.value)}><option value="">All statuses</option>{podStatuses.map((value) => <option key={value}>{value}</option>)}</select></label>
        <label>Restarts<select value={restarts} onChange={(event) => setRestarts(event.target.value)}><option value="any">Any</option><option value="gt0">More than 0</option><option value="gte3">At least 3</option><option value="gte10">At least 10</option></select></label>
        <label>Problem evidence<select value={problematic} onChange={(event) => setProblematic(event.target.value)}><option value="">All Pods</option><option value="true">Problematic only</option><option value="false">Without evidence</option></select></label>
      </SearchBar>
      {selection ? <SavedFilterControls collection="pods" generation={selection.generation} currentQuery={compactFilterQuery([
        ['namespace', namespaceValues(namespace)], ['search', search], ['status', podStatus ? [podStatus] : []], ['workload', workload], ['node', node], ['restarts', restarts === 'any' ? '' : restarts], ['problematic', problematic === '' ? undefined : problematic === 'true'],
      ])} onApply={(query) => {
        const nextSearch = savedString(query, 'search')
        const savedRestarts = savedString(query, 'restarts', restartFilters)
        setNamespace(savedNamespaces(query))
        setSearch(nextSearch)
        setAppliedSearch(nextSearch)
        setPodStatus(savedFirst(query, 'status', podStatuses))
        setWorkload(savedString(query, 'workload'))
        setNode(savedString(query, 'node'))
        setRestarts(savedRestarts || 'any')
        setProblematic(typeof query.problematic === 'boolean' ? String(query.problematic) : '')
        setCursor('')
        closeDetail()
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}>
          <div className="resource-layout"><div className="resource-table-wrap"><table className="resource-table"><caption>Authorized Pod page</caption><thead><tr><th>Namespace / Pod</th><th>Status</th><th>Ready</th><th>Restarts</th><th>Node</th><th>Age</th></tr></thead><tbody>{list.data?.items.map((item) => <tr className={item.problematic ? 'resource-row--problem' : ''} key={`${item.namespace}/${item.name}`}><td><button type="button" className="table-link" aria-label={`Open Pod ${item.name} in ${item.namespace}`} onClick={() => { requests.abortAll(); setSelected({ generation: selection!.generation, item }); yaml.reset() }}><strong>{item.name}</strong><small>{item.namespace}</small></button></td><td>{item.status}</td><td>{item.ready.current}/{item.ready.desired}</td><td>{item.restarts}</td><td>{item.node ?? '—'}</td><td>{age(item.ageSeconds)}</td></tr>)}</tbody></table>{list.data ? <CollectionFooter result={list.data} onNext={(next) => { setCursor(next); closeDetail() }} onRestart={() => { setCursor(''); closeDetail() }} /> : null}</div>
            {activeSelected ? <DetailCard title={`Pod ${activeSelected.namespace}/${activeSelected.name}`} onClose={closeDetail}>{detail.isPending ? <p>Loading detail…</p> : detail.isError ? <p className="field-error">{errorMessage(detail.error)}</p> : detail.data ? <><dl className="resource-facts"><div><dt>UID</dt><dd>{detail.data.metadata.uid}</dd></div><div><dt>Owner</dt><dd>{detail.data.summary.owner ? `${detail.data.summary.owner.kind}/${detail.data.summary.owner.name}` : 'standalone'}</dd></div><div><dt>Containers</dt><dd>{detail.data.containers.map((value) => `${value.spec.name} (${value.state})`).join(', ') || 'none'}</dd></div><div><dt>IP</dt><dd>{detail.data.summary.ip ?? '—'}</dd></div></dl><Link className="button button--secondary button--compact" to={`/logs?namespace=${encodeURIComponent(activeSelected.namespace)}&pod=${encodeURIComponent(activeSelected.name)}&container=${encodeURIComponent(detail.data.containers[0]?.spec.name ?? '')}`}>View logs</Link><PodActions key={`${selection!.generation}/${detail.data.metadata.uid}`} detail={detail.data} selection={selection as SelectionSummary} /></> : null}<YAMLViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} /></DetailCard> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </div>
  )
}

export function EventsPage() {
  const { status, selection } = useActiveSelection()
  const [search, setSearch] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [namespace, setNamespace] = useState('')
  const [eventType, setEventType] = useState('')
  const [objectKind, setObjectKind] = useState('')
  const [reason, setReason] = useState('')
  const [cursor, setCursor] = useState('')
  const queryClient = useQueryClient()
  const list = useQuery({ queryKey: ['resources', 'events', selection?.generation, appliedSearch, namespace, eventType, objectKind, reason, cursor], queryFn: ({ signal }) => getEvents({ limit: 100, search: appliedSearch || undefined, namespaces: namespaceValues(namespace), statuses: eventType ? [eventType] : undefined, objectKind: objectKind || undefined, reason: reason || undefined, continueToken: cursor || undefined, sort: 'timestamp', order: 'desc' }, signal, selection?.generation), enabled: Boolean(selection) })
  return (
    <div className="resource-page">
      <PageHeading title="Events" description="Real Kubernetes events ordered within the bounded page; type, source and count are preserved." action={selection ? <ResourceLiveUpdates key={`events/${selection.generation}`} generation={selection.generation} topics={['events']} queryKeys={[["resources", "events"]]} /> : null} />
      <SearchBar value={search} onChange={setSearch} onApply={() => { setCursor(''); setAppliedSearch(search) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'events'] })}>
        <label>Namespace<input value={namespace} maxLength={256} placeholder="active scope; comma-separated" onChange={(event) => setNamespace(event.target.value)} /></label>
        <label>Type<select value={eventType} onChange={(event) => setEventType(event.target.value)}><option value="">All types</option>{eventTypes.map((value) => <option key={value}>{value}</option>)}</select></label>
        <label>Object kind<input value={objectKind} maxLength={256} onChange={(event) => setObjectKind(event.target.value)} /></label>
        <label>Reason<input value={reason} maxLength={256} onChange={(event) => setReason(event.target.value)} /></label>
      </SearchBar>
      {selection ? <SavedFilterControls collection="events" generation={selection.generation} currentQuery={compactFilterQuery([
        ['namespace', namespaceValues(namespace)], ['search', search], ['status', eventType ? [eventType] : []], ['sort', 'timestamp'], ['order', 'desc'], ['objectKind', objectKind], ['reason', reason],
      ])} onApply={(query) => {
        const nextSearch = savedString(query, 'search')
        setNamespace(savedNamespaces(query))
        setSearch(nextSearch)
        setAppliedSearch(nextSearch)
        setEventType(savedFirst(query, 'status', eventTypes))
        setObjectKind(savedString(query, 'objectKind'))
        setReason(savedString(query, 'reason'))
        setCursor('')
      }} /> : null}
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}><QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}><div className="resource-table-wrap"><table className="resource-table"><caption>Authorized event page</caption><thead><tr><th>Time</th><th>Namespace</th><th>Object</th><th>Type / reason</th><th>Count</th><th>Message</th></tr></thead><tbody>{list.data?.items.map((item, index) => <tr key={`${item.namespace}/${item.objectKind}/${item.objectName}/${item.timestamp ?? index}`}><td>{dateTime(item.timestamp)}</td><td>{item.namespace}</td><td>{item.objectKind}/{item.objectName}</td><td><span className={`event-type event-type--${item.type.toLowerCase()}`}>{item.type}</span><small>{item.reason}</small></td><td>{item.count}</td><td className="resource-message">{item.message}</td></tr>)}</tbody></table>{list.data ? <CollectionFooter result={list.data} onNext={setCursor} onRestart={() => setCursor('')} /> : null}</div></QueryState></SelectionGate>
    </div>
  )
}

type NetworkTab = 'services' | 'ingresses' | 'endpoint-slices' | 'port-forwards'
type NetworkItem = ServiceResource | IngressResource | EndpointSliceResource
type NetworkSelection = { generation: string; tab: Exclude<NetworkTab, 'port-forwards'>; namespace: string; name: string }

function NetworkDetailView({ tab, service, ingress, slice }: { tab: NetworkSelection['tab']; service?: ServiceDetail; ingress?: IngressDetail; slice?: EndpointSliceDetail }) {
  if (tab === 'services' && service) return <><dl className="resource-facts"><div><dt>Type</dt><dd>{service.summary.type}</dd></div><div><dt>Cluster IPs</dt><dd>{service.summary.clusterIPs.join(', ') || 'none'}</dd></div><div><dt>Session affinity</dt><dd>{service.sessionAffinity}</dd></div><div><dt>Ports</dt><dd>{service.summary.ports.map((port) => `${port.port}/${port.protocol}`).join(', ') || 'none'}</dd></div></dl></>
  if (tab === 'ingresses' && ingress) return <><dl className="resource-facts"><div><dt>Class</dt><dd>{ingress.summary.className ?? 'default'}</dd></div><div><dt>Hosts</dt><dd>{ingress.summary.hosts.join(', ') || 'none'}</dd></div><div><dt>Paths</dt><dd>{ingress.summary.paths.map((path) => `${path.host}${path.path} → ${path.backend.serviceName}`).join(', ') || 'none'}</dd></div><div><dt>Load balancers</dt><dd>{ingress.loadBalancerAddresses.join(', ') || 'none'}</dd></div></dl></>
  if (tab === 'endpoint-slices' && slice) return <><dl className="resource-facts"><div><dt>Address type</dt><dd>{slice.summary.addressType}</dd></div><div><dt>Endpoints</dt><dd>{slice.summary.endpoints.length}</dd></div><div><dt>Addresses</dt><dd>{slice.summary.endpoints.flatMap((endpoint) => endpoint.addresses).join(', ') || 'none'}</dd></div><div><dt>Ports</dt><dd>{slice.summary.ports.map((port) => port.port ?? 'named').join(', ') || 'none'}</dd></div></dl></>
  return null
}

export function NetworkPage() {
  const { status, selection } = useActiveSelection()
  const [tab, setTab] = useState<NetworkTab>('services')
  const [search, setSearch] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [cursors, setCursors] = useState<Record<Exclude<NetworkTab, 'port-forwards'>, string>>({ services: '', ingresses: '', 'endpoint-slices': '' })
  const [selected, setSelected] = useState<NetworkSelection | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const currentCursor = tab === 'port-forwards' ? '' : cursors[tab]
  const activeSelected = selected?.generation === selection?.generation ? selected : null
  const options = { limit: 100, search: appliedSearch || undefined, continueToken: currentCursor || undefined }
  const services = useQuery({ queryKey: ['resources', 'services', selection?.generation, appliedSearch, cursors.services], queryFn: ({ signal }) => getServices(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'services') })
  const ingresses = useQuery({ queryKey: ['resources', 'ingresses', selection?.generation, appliedSearch, cursors.ingresses], queryFn: ({ signal }) => getIngresses(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'ingresses') })
  const slices = useQuery({ queryKey: ['resources', 'endpoint-slices', selection?.generation, appliedSearch, cursors['endpoint-slices']], queryFn: ({ signal }) => getEndpointSlices(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'endpoint-slices') })
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
  }

  function setCursor(value: string) {
    if (tab === 'port-forwards') return
    setCursors((current) => ({ ...current, [tab]: value }))
    closeDetail()
  }

  return (
    <div className="resource-page">
      <PageHeading title="Network" description="Services, Ingresses, EndpointSlices and loopback-only port-forward sessions." />
      <div className="resource-tabs" role="tablist" aria-label="Network resource type">{(['services', 'ingresses', 'endpoint-slices', 'port-forwards'] as const).map((value) => <button type="button" role="tab" aria-selected={tab === value} aria-controls="network-panel" className={tab === value ? 'active' : ''} key={value} onClick={() => { setTab(value); closeDetail() }}>{value}</button>)}</div>
      {selection && tab !== 'port-forwards' ? <ResourceLiveUpdates key={`${tab}/${selection.generation}`} generation={selection.generation} topics={[tab]} queryKeys={[["resources", tab]]} /> : null}
      {tab !== 'port-forwards' ? <SearchBar value={search} onChange={setSearch} onApply={() => { setCursor(''); setAppliedSearch(search) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} /> : null}
      <div id="network-panel" role="tabpanel">
        <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
          {tab === 'port-forwards' ? <QueryState pending={forwards.isPending} error={forwards.error ?? close.error} empty={forwards.data?.length === 0}><div className="session-grid">{forwards.data?.map((item) => <article key={item.id}><strong>{item.localAddress}:{item.localPort}</strong><span>{item.context} · {item.namespace}/{item.pod} → {item.remotePort}</span><small>{item.status} · created {dateTime(item.createdAt)} · expires {dateTime(item.expiresAt)}</small>{item.endedAt ? <small>ended {dateTime(item.endedAt)} · {item.endReason ?? item.status}</small> : null}{item.status === 'active' ? <button type="button" className="button button--danger button--compact" onClick={() => close.mutate(item.id)}>Close loopback session</button> : null}</article>)}</div></QueryState> : <QueryState pending={activeQuery.isPending} error={activeQuery.error} empty={active?.items.length === 0}><div className="resource-layout"><div className="resource-table-wrap"><table className="resource-table"><caption>Authorized {tab} page</caption><thead><tr><th>Namespace / name</th><th>Type</th><th>Summary</th></tr></thead><tbody>{active?.items.map((item) => <tr key={`${item.namespace}/${item.name}`}><td><button type="button" className="table-link" aria-label={`Open ${tab} ${item.name} in ${item.namespace}`} onClick={() => { closeDetail(); setSelected({ generation: selection!.generation, tab: tab as NetworkSelection['tab'], namespace: item.namespace, name: item.name }) }}><strong>{item.name}</strong><small>{item.namespace}</small></button></td><td>{'type' in item ? item.type : 'className' in item ? (item.className ?? 'Ingress') : item.addressType}</td><td>{'clusterIPs' in item ? item.clusterIPs.join(', ') : 'hosts' in item ? item.hosts.join(', ') : `${item.endpoints.length} endpoints`}</td></tr>)}</tbody></table>{active ? <CollectionFooter result={active} onNext={setCursor} onRestart={() => setCursor('')} /> : null}</div>{activeSelected ? <DetailCard title={`${activeSelected.namespace}/${activeSelected.name}`} onClose={closeDetail}>{currentDetail.isPending ? <p>Loading detail…</p> : currentDetail.isError ? <p className="field-error">{errorMessage(currentDetail.error)}</p> : <NetworkDetailView tab={activeSelected.tab} service={serviceDetail.data} ingress={ingressDetail.data} slice={sliceDetail.data} />}<YAMLViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} /></DetailCard> : null}</div></QueryState>}
        </SelectionGate>
      </div>
    </div>
  )
}

type ConfigTab = 'configmaps' | 'secrets'
type ConfigItem = ConfigMapResource | SecretMetadata
type ConfigSelection = { generation: string; namespace: string; name: string }

function ConfigMapDetailView({ detail }: { detail: ConfigMapDetail }) {
  return <><dl className="resource-facts"><div><dt>Resource version</dt><dd>{detail.metadata.resourceVersion}</dd></div><div><dt>Total bytes</dt><dd>{detail.totalBytes}</dd></div><div><dt>Entries</dt><dd>{detail.entries.length}</dd></div><div><dt>Truncated</dt><dd>{detail.truncated ? 'yes' : 'no'}</dd></div></dl><div className="config-entry-list">{detail.entries.map((entry) => <details key={entry.key}><summary>{entry.key} · {entry.encoding}{entry.truncated ? ' · truncated' : ''}</summary><pre>{entry.value}</pre></details>)}</div></>
}

function SecretMetadataView({ secret }: { secret: SecretMetadata }) {
  return <><dl className="resource-facts"><div><dt>API version</dt><dd>{secret.apiVersion}</dd></div><div><dt>Kind</dt><dd>{secret.kind}</dd></div><div><dt>UID</dt><dd>{secret.metadata.uid}</dd></div><div><dt>Created</dt><dd>{dateTime(secret.metadata.creationTimestamp)}</dd></div>{secret.metadata.deletionTimestamp ? <div><dt>Deleting</dt><dd>{dateTime(secret.metadata.deletionTimestamp)}</dd></div> : null}</dl><p className="permission-notice">Secret values, annotations, managed fields and YAML are intentionally unavailable.</p></>
}

export function ConfigPage() {
  const { status, selection } = useActiveSelection()
  const [tab, setTab] = useState<ConfigTab>('configmaps')
  const [search, setSearch] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [cursors, setCursors] = useState<Record<ConfigTab, string>>({ configmaps: '', secrets: '' })
  const [selected, setSelected] = useState<ConfigSelection | null>(null)
  const queryClient = useQueryClient()
  const requests = useGenerationRequests(selection?.generation)
  const activeSelected = selected?.generation === selection?.generation ? selected : null
  const options = { limit: 100, search: appliedSearch || undefined, continueToken: cursors[tab] || undefined }
  const configMaps = useQuery({ queryKey: ['resources', 'configmaps', selection?.generation, appliedSearch, cursors.configmaps], queryFn: ({ signal }) => getConfigMaps(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'configmaps') })
  const secrets = useQuery({ queryKey: ['resources', 'secrets', selection?.generation, appliedSearch, cursors.secrets], queryFn: ({ signal }) => getSecrets(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'secrets') })
  const configDetail = useQuery({ queryKey: ['resources', 'configmap-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getConfigMap(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'configmaps') })
  const secretDetail = useQuery({ queryKey: ['resources', 'secret-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getSecret(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'secrets') })
  const yaml = useMutation({ mutationFn: (value: ConfigSelection) => requests.run((signal) => getConfigMapYAML(value.namespace, value.name, signal)) })
  const active: CollectionResult<ConfigItem> | undefined = tab === 'configmaps' && configMaps.data ? { ...configMaps.data, items: configMaps.data.items } : tab === 'secrets' && secrets.data ? { ...secrets.data, items: secrets.data.items } : undefined
  const activeQuery = tab === 'configmaps' ? configMaps : secrets

  function closeDetail() {
    requests.abortAll()
    yaml.reset()
    setSelected(null)
  }

  function setCursor(value: string) {
    setCursors((current) => ({ ...current, [tab]: value }))
    closeDetail()
  }

  return (
    <div className="resource-page">
      <PageHeading title="Config" description="ConfigMaps are fetched on detail; Secrets remain metadata-only and never expose values or YAML." />
      <div className="resource-tabs" role="tablist" aria-label="Configuration resource type">{(['configmaps', 'secrets'] as const).map((value) => <button type="button" role="tab" aria-selected={tab === value} aria-controls="config-panel" className={tab === value ? 'active' : ''} key={value} onClick={() => { setTab(value); closeDetail() }}>{value}</button>)}</div>
      {selection && tab === 'configmaps' ? <ResourceLiveUpdates key={`configmaps/${selection.generation}`} generation={selection.generation} topics={['configmaps']} queryKeys={[["resources", "configmaps"]]} /> : null}
      <SearchBar value={search} onChange={setSearch} onApply={() => { setCursor(''); setAppliedSearch(search) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} />
      <div id="config-panel" role="tabpanel"><SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}><QueryState pending={activeQuery.isPending} error={activeQuery.error} empty={active?.items.length === 0}><div className="resource-layout"><div className="resource-table-wrap"><table className="resource-table"><caption>Authorized {tab} metadata page</caption><thead><tr><th>Namespace / name</th><th>UID</th><th>Created</th></tr></thead><tbody>{active?.items.map((item) => { const value = 'metadata' in item ? item.metadata : item; return <tr key={`${value.namespace}/${value.name}`}><td><button type="button" className="table-link" aria-label={`Open ${tab === 'secrets' ? 'Secret' : 'ConfigMap'} ${value.name} in ${value.namespace}`} onClick={() => { closeDetail(); setSelected({ generation: selection!.generation, namespace: value.namespace, name: value.name }) }}><strong>{value.name}</strong><small>{value.namespace}</small></button></td><td>{value.uid}</td><td>{dateTime(value.creationTimestamp)}</td></tr> })}</tbody></table>{active ? <CollectionFooter result={active} onNext={setCursor} onRestart={() => setCursor('')} /> : null}</div>{activeSelected ? <DetailCard title={`${tab === 'secrets' ? 'Secret metadata' : 'ConfigMap'} ${activeSelected.namespace}/${activeSelected.name}`} onClose={closeDetail}>{tab === 'configmaps' ? configDetail.isPending ? <p>Loading authorized entries…</p> : configDetail.isError ? <p className="field-error">{errorMessage(configDetail.error)}</p> : configDetail.data ? <ConfigMapDetailView detail={configDetail.data} /> : null : secretDetail.isPending ? <p>Loading metadata…</p> : secretDetail.isError ? <p className="field-error">{errorMessage(secretDetail.error)}</p> : secretDetail.data ? <SecretMetadataView secret={secretDetail.data} /> : null}{tab === 'configmaps' ? <YAMLViewer value={yaml.data} pending={yaml.isPending} error={yaml.error} onLoad={() => yaml.mutate(activeSelected)} /> : null}</DetailCard> : null}</div></QueryState></SelectionGate></div>
    </div>
  )
}
