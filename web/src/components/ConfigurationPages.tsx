import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import {
  getHPAs,
  getLimitRanges,
  getPDBs,
  getResourceQuotas,
  getServiceAccounts,
  getStatus,
} from '../api/client'
import type {
  CollectionResult,
  HorizontalPodAutoscaler,
  LimitRange,
  PodDisruptionBudget,
  ResourceQuota,
  ServiceAccount,
} from '../api/types'
import { Badge, DataTable, type DataTableColumn } from './ui'
import { ResourceListControls } from './ResourceListControls'
import { CollectionFooter, QueryState, SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import type { ListSortOption } from './ResourceListControls'
import { TableLink } from './resource/TableLink'
import { age } from './resource/format'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { useResourceWorkspace } from './workspace/ResourceWorkspaceProvider'

function useActiveSelection() {
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  return { status, selection: status.data?.selection ?? null }
}

function quantitySummary(values: Record<string, string> | null): string {
  if (!values) return 'none'
  const entries = Object.entries(values)
  return entries.slice(0, 4).map(([key, value]) => `${key} ${value}`).join(' · ') + (entries.length > 4 ? ` · +${entries.length - 4} more` : '')
}

type ConfigurationTab = 'resource-quotas' | 'limit-ranges' | 'hpas' | 'pdbs'

const configurationTabs: ConfigurationTab[] = ['resource-quotas', 'limit-ranges', 'hpas', 'pdbs']
const configurationSortOptions: readonly ListSortOption[] = [
  { value: 'identity', label: 'Namespace and name' },
  { value: 'name', label: 'Name' },
]
interface ListState {
  search: string
  sort: string
  order: 'asc' | 'desc'
}
const initialListState: ListState = { search: '', sort: 'identity', order: 'asc' }

function configurationTabFromParams(tab: string): ConfigurationTab | null {
  return (configurationTabs as string[]).includes(tab) ? (tab as ConfigurationTab) : null
}

export function ConfigurationPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const { tab: tabParam, namespace, name } = useParams<{ tab?: string; namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const generation = selection?.generation
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const tab = useMemo(() => configurationTabFromParams(tabParam ?? '') ?? 'resource-quotas', [tabParam])
  const [cursor, setCursor] = useState('')
  const options = { limit: 100, search: applied.search || undefined, continueToken: cursor || undefined, namespaces: effectiveNamespaces(globalNamespace.value, []), sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  // Deep links (/configuration/:tab/:ns/:name) open the Resource Workspace.
  useEffect(() => {
    if (!tab || !namespace || !name || !generation) return
    workspace.openFromRoute({ collection: tab, namespace, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tab, namespace, name, generation])

  const quotas = useQuery({ queryKey: ['resources', 'resource-quotas', generation, applied, cursor], queryFn: ({ signal }) => getResourceQuotas(options, signal, generation), enabled: Boolean(selection && tab === 'resource-quotas') })
  const limitRanges = useQuery({ queryKey: ['resources', 'limit-ranges', generation, applied, cursor], queryFn: ({ signal }) => getLimitRanges(options, signal, generation), enabled: Boolean(selection && tab === 'limit-ranges') })
  const hpas = useQuery({ queryKey: ['resources', 'hpas', generation, applied, cursor], queryFn: ({ signal }) => getHPAs(options, signal, generation), enabled: Boolean(selection && tab === 'hpas') })
  const pdbs = useQuery({ queryKey: ['resources', 'pdbs', generation, applied, cursor], queryFn: ({ signal }) => getPDBs(options, signal, generation), enabled: Boolean(selection && tab === 'pdbs') })

  const active: CollectionResult<unknown> | undefined =
    tab === 'resource-quotas' ? quotas.data : tab === 'limit-ranges' ? limitRanges.data : tab === 'hpas' ? hpas.data : pdbs.data
  const activeQuery = tab === 'resource-quotas' ? quotas : tab === 'limit-ranges' ? limitRanges : tab === 'hpas' ? hpas : pdbs

  const columns: DataTableColumn<unknown>[] = (() => {
    switch (tab) {
      case 'resource-quotas':
        return [
          { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = item as ResourceQuota; return <TableLink aria-label={`Open quota ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} secondary={value.namespace} /> } },
          { key: 'hard', header: 'Hard', cell: (item) => quantitySummary((item as ResourceQuota).hard) },
          { key: 'used', header: 'Used', cell: (item) => quantitySummary((item as ResourceQuota).used) },
        ]
      case 'limit-ranges':
        return [
          { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = item as LimitRange; return <TableLink aria-label={`Open limit range ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} secondary={value.namespace} /> } },
          { key: 'items', header: 'Items', cell: (item) => (item as LimitRange).items.length },
          { key: 'types', header: 'Types', cell: (item) => [...new Set((item as LimitRange).items.map((limit) => limit.type))].join(', ') || 'none' },
        ]
      case 'hpas':
        return [
          { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = item as HorizontalPodAutoscaler; return <TableLink aria-label={`Open autoscaler ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} secondary={value.namespace} /> } },
          { key: 'target', header: 'Target', cell: (item) => `${(item as HorizontalPodAutoscaler).targetKind}/${(item as HorizontalPodAutoscaler).targetName}` },
          { key: 'minmax', header: 'Min / Max', cell: (item) => `${(item as HorizontalPodAutoscaler).minReplicas ?? '—'} / ${(item as HorizontalPodAutoscaler).maxReplicas}` },
          { key: 'replicas', header: 'Current / Desired', cell: (item) => `${(item as HorizontalPodAutoscaler).currentReplicas} / ${(item as HorizontalPodAutoscaler).desiredReplicas}` },
          { key: 'age', header: 'Age', cell: (item) => age((item as HorizontalPodAutoscaler).ageSeconds) },
        ]
      case 'pdbs':
        return [
          { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = item as PodDisruptionBudget; return <TableLink aria-label={`Open budget ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} secondary={value.namespace} /> } },
          { key: 'allowed', header: 'Disruptions allowed', cell: (item) => { const allowed = (item as PodDisruptionBudget).disruptionsAllowed; return allowed > 0 ? <Badge variant="healthy">{allowed}</Badge> : <Badge variant={allowed === 0 ? 'warning' : 'unknown'}>{allowed}</Badge> } },
          { key: 'healthy', header: 'Healthy / Desired', cell: (item) => `${(item as PodDisruptionBudget).currentHealthy} / ${(item as PodDisruptionBudget).desiredHealthy}` },
          { key: 'age', header: 'Age', cell: (item) => age((item as PodDisruptionBudget).ageSeconds) },
        ]
    }
  })()

  return (
    <ResourcePage title="Configuration" description="Quotas, limits, autoscalers and disruption budgets in the active scope; absence and unknown stay distinct from zero.">
      <ResourceTabStrip ariaLabel="Configuration resource type" panelId="configuration-panel" active={tab} onChange={(value) => { setDraft(initialListState); setApplied(initialListState); setCursor(''); navigate(`/configuration/${value}`) }} tabs={configurationTabs.map((id) => ({ id, label: id }))} />
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft({ ...draft, search: value })} onApply={() => { setApplied(draft); setCursor('') }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} onClear={() => { setDraft(initialListState); setApplied(initialListState); setCursor('') }} activeFilters={applied.search ? [{ id: 'search', label: 'Search', value: applied.search }] : []} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={draft.search !== applied.search || draft.sort !== applied.sort || draft.order !== applied.order} sortOptions={configurationSortOptions} onSortChange={(value) => setDraft({ ...draft, sort: value })} onOrderChange={(value) => setDraft({ ...draft, order: value })} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={activeQuery.isPending} error={activeQuery.error} empty={active?.items.length === 0}>
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            <DataTable caption={`Authorized ${tab} page`} rows={active?.items ?? []} getRowKey={(item: unknown) => { const value = item as { namespace: string; name: string }; return `${value.namespace}/${value.name}` }} columns={columns} stickyHeader />
            {active ? <CollectionFooter result={active} onNext={setCursor} onRestart={() => setCursor('')} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}

export function ServiceAccountsPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const { namespace, name } = useParams<{ namespace?: string; name?: string }>()
  const queryClient = useQueryClient()
  const generation = selection?.generation
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const [cursor, setCursor] = useState('')

  useEffect(() => {
    if (!namespace || !name || !generation) return
    workspace.openFromRoute({ collection: 'service-accounts', namespace, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [namespace, name, generation])

  const list = useQuery({ queryKey: ['resources', 'service-accounts', generation, applied, cursor], queryFn: ({ signal }) => getServiceAccounts({ limit: 100, search: applied.search || undefined, continueToken: cursor || undefined, namespaces: effectiveNamespaces(globalNamespace.value, []), sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }, signal, generation), enabled: Boolean(selection) })

  return (
    <ResourcePage title="ServiceAccounts" description="Namespace ServiceAccounts as metadata only: no tokens, no Secret references and no arbitrary annotations.">
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft({ ...draft, search: value })} onApply={() => { setApplied(draft); setCursor('') }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'service-accounts'] })} onClear={() => { setDraft(initialListState); setApplied(initialListState); setCursor('') }} activeFilters={applied.search ? [{ id: 'search', label: 'Search', value: applied.search }] : []} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={draft.search !== applied.search || draft.sort !== applied.sort || draft.order !== applied.order} sortOptions={configurationSortOptions} onSortChange={(value) => setDraft({ ...draft, sort: value })} onOrderChange={(value) => setDraft({ ...draft, order: value })} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}>
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            <DataTable
              caption="Authorized ServiceAccount page"
              rows={list.data?.items ?? []}
              getRowKey={(item) => `${item.namespace}/${item.name}`}
              columns={[
                { key: 'namespace', header: 'Namespace / name', cell: (item) => <TableLink aria-label={`Open ServiceAccount ${item.name} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'service-accounts', namespace: item.namespace, name: item.name })} primary={item.name} secondary={item.namespace} /> },
                { key: 'uid', header: 'UID', cell: (item) => <span className="mono text-xs">{item.uid}</span> },
                { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
              ] as DataTableColumn<ServiceAccount>[]}
              stickyHeader
            />
            {list.data ? <CollectionFooter result={list.data} onNext={setCursor} onRestart={() => setCursor('')} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}
