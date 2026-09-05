import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import {
  getHPA,
  getHPAs,
  getLimitRange,
  getLimitRanges,
  getPDB,
  getPDBs,
  getResourceQuota,
  getResourceQuotas,
  getServiceAccount,
  getServiceAccounts,
  getStatus,
} from '../api/client'
import type {
  CollectionResult,
  HorizontalPodAutoscaler,
  LimitRange,
  LimitRangeItem,
  PodDisruptionBudget,
  ResourceQuota,
  ServiceAccount,
} from '../api/types'
import { Badge, DataTable, type DataTableColumn, Drawer } from './ui'
import { ResourceListControls } from './ResourceListControls'
import { errorMessage } from './resource/errors'
import { CollectionFooter, QueryState, SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import type { ListSortOption } from './ResourceListControls'
import { TableLink } from './resource/TableLink'
import { Facts } from './resource/Facts'
import { age, dateTime } from './resource/format'

function useActiveSelection() {
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  return { status, selection: status.data?.selection ?? null }
}

function detailTitle(title: string) {
  return <h2 className="text-lg text-kp-text break-words">{title}</h2>
}

function quantitySummary(values: Record<string, string> | null): string {
  if (!values) return 'none'
  const entries = Object.entries(values)
  return entries.slice(0, 4).map(([key, value]) => `${key} ${value}`).join(' · ') + (entries.length > 4 ? ` · +${entries.length - 4} more` : '')
}

function intOrStringLabel(value: { isInt: boolean; int: number; string: string } | null): string {
  if (!value) return '—'
  return value.isInt ? String(value.int) : value.string
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

interface ConfigurationSelection {
  namespace: string
  name: string
}

export function ConfigurationPage() {
  const { status, selection } = useActiveSelection()
  const { tab: tabParam, namespace, name } = useParams<{ tab?: string; namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const tab = useMemo(() => configurationTabFromParams(tabParam ?? '') ?? 'resource-quotas', [tabParam])
  const [cursor, setCursor] = useState('')
  const [selected, setSelected] = useState<ConfigurationSelection | null>(null)
  const paramSelection = namespace && name && selection ? { namespace, name } : null
  const activeSelected = paramSelection || selected
  const options = { limit: 100, search: applied.search || undefined, continueToken: cursor || undefined, sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  const quotas = useQuery({ queryKey: ['resources', 'resource-quotas', selection?.generation, applied, cursor], queryFn: ({ signal }) => getResourceQuotas(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'resource-quotas') })
  const limitRanges = useQuery({ queryKey: ['resources', 'limit-ranges', selection?.generation, applied, cursor], queryFn: ({ signal }) => getLimitRanges(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'limit-ranges') })
  const hpas = useQuery({ queryKey: ['resources', 'hpas', selection?.generation, applied, cursor], queryFn: ({ signal }) => getHPAs(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'hpas') })
  const pdbs = useQuery({ queryKey: ['resources', 'pdbs', selection?.generation, applied, cursor], queryFn: ({ signal }) => getPDBs(options, signal, selection?.generation), enabled: Boolean(selection && tab === 'pdbs') })

  const quotaDetail = useQuery({ queryKey: ['resources', 'quota-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getResourceQuota(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'resource-quotas') })
  const limitRangeDetail = useQuery({ queryKey: ['resources', 'limitrange-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getLimitRange(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'limit-ranges') })
  const hpaDetail = useQuery({ queryKey: ['resources', 'hpa-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getHPA(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'hpas') })
  const pdbDetail = useQuery({ queryKey: ['resources', 'pdb-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getPDB(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected && tab === 'pdbs') })

  const active: CollectionResult<unknown> | undefined =
    tab === 'resource-quotas' ? quotas.data : tab === 'limit-ranges' ? limitRanges.data : tab === 'hpas' ? hpas.data : pdbs.data
  const activeQuery = tab === 'resource-quotas' ? quotas : tab === 'limit-ranges' ? limitRanges : tab === 'hpas' ? hpas : pdbs

  function closeDetail() {
    setSelected(null)
    navigate(`/configuration/${tab}`)
  }

  const columns: DataTableColumn<unknown>[] = (() => {
    switch (tab) {
      case 'resource-quotas':
        return [
          { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = item as ResourceQuota; return <TableLink aria-label={`Open quota ${value.name} in ${value.namespace}`} onClick={() => { setSelected(value); navigate(`/configuration/${tab}/${encodeURIComponent(value.namespace)}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.namespace} /> } },
          { key: 'hard', header: 'Hard', cell: (item) => quantitySummary((item as ResourceQuota).hard) },
          { key: 'used', header: 'Used', cell: (item) => quantitySummary((item as ResourceQuota).used) },
        ]
      case 'limit-ranges':
        return [
          { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = item as LimitRange; return <TableLink aria-label={`Open limit range ${value.name} in ${value.namespace}`} onClick={() => { setSelected(value); navigate(`/configuration/${tab}/${encodeURIComponent(value.namespace)}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.namespace} /> } },
          { key: 'items', header: 'Items', cell: (item) => (item as LimitRange).items.length },
          { key: 'types', header: 'Types', cell: (item) => [...new Set((item as LimitRange).items.map((limit) => limit.type))].join(', ') || 'none' },
        ]
      case 'hpas':
        return [
          { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = item as HorizontalPodAutoscaler; return <TableLink aria-label={`Open autoscaler ${value.name} in ${value.namespace}`} onClick={() => { setSelected(value); navigate(`/configuration/${tab}/${encodeURIComponent(value.namespace)}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.namespace} /> } },
          { key: 'target', header: 'Target', cell: (item) => `${(item as HorizontalPodAutoscaler).targetKind}/${(item as HorizontalPodAutoscaler).targetName}` },
          { key: 'minmax', header: 'Min / Max', cell: (item) => `${(item as HorizontalPodAutoscaler).minReplicas ?? '—'} / ${(item as HorizontalPodAutoscaler).maxReplicas}` },
          { key: 'replicas', header: 'Current / Desired', cell: (item) => `${(item as HorizontalPodAutoscaler).currentReplicas} / ${(item as HorizontalPodAutoscaler).desiredReplicas}` },
          { key: 'age', header: 'Age', cell: (item) => age((item as HorizontalPodAutoscaler).ageSeconds) },
        ]
      case 'pdbs':
        return [
          { key: 'namespace', header: 'Namespace / name', cell: (item) => { const value = item as PodDisruptionBudget; return <TableLink aria-label={`Open budget ${value.name} in ${value.namespace}`} onClick={() => { setSelected(value); navigate(`/configuration/${tab}/${encodeURIComponent(value.namespace)}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.namespace} /> } },
          { key: 'allowed', header: 'Disruptions allowed', cell: (item) => { const allowed = (item as PodDisruptionBudget).disruptionsAllowed; return allowed > 0 ? <Badge variant="healthy">{allowed}</Badge> : <Badge variant={allowed === 0 ? 'warning' : 'unknown'}>{allowed}</Badge> } },
          { key: 'healthy', header: 'Healthy / Desired', cell: (item) => `${(item as PodDisruptionBudget).currentHealthy} / ${(item as PodDisruptionBudget).desiredHealthy}` },
          { key: 'age', header: 'Age', cell: (item) => age((item as PodDisruptionBudget).ageSeconds) },
        ]
    }
  })()

  return (
    <ResourcePage title="Configuration" description="Quotas, limits, autoscalers and disruption budgets in the active scope; absence and unknown stay distinct from zero.">
      <ResourceTabStrip ariaLabel="Configuration resource type" panelId="configuration-panel" active={tab} onChange={(value) => { setDraft(initialListState); setApplied(initialListState); setCursor(''); setSelected(null); navigate(`/configuration/${value}`) }} tabs={configurationTabs.map((id) => ({ id, label: id }))} />
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft({ ...draft, search: value })} onApply={() => { setApplied(draft); setCursor(''); setSelected(null) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', tab] })} onClear={() => { setDraft(initialListState); setApplied(initialListState); setCursor(''); setSelected(null) }} activeFilters={applied.search ? [{ id: 'search', label: 'Search', value: applied.search }] : []} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={draft.search !== applied.search || draft.sort !== applied.sort || draft.order !== applied.order} sortOptions={configurationSortOptions} onSortChange={(value) => setDraft({ ...draft, sort: value })} onOrderChange={(value) => setDraft({ ...draft, order: value })} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={activeQuery.isPending} error={activeQuery.error} empty={active?.items.length === 0}>
          <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
            <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
              <DataTable caption={`Authorized ${tab} page`} rows={active?.items ?? []} getRowKey={(item: unknown) => { const value = item as { namespace: string; name: string }; return `${value.namespace}/${value.name}` }} columns={columns} />
              {active ? <CollectionFooter result={active} onNext={(next) => { setCursor(next); setSelected(null) }} onRestart={() => { setCursor(''); setSelected(null) }} /> : null}
            </div>
            <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={detailTitle(activeSelected ? `Configuration ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}>
              {activeSelected ? renderConfigurationDetail(tab, quotaDetail, limitRangeDetail, hpaDetail, pdbDetail) : null}
            </Drawer>
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}

interface DetailQuery { isPending: boolean; error: unknown; data?: unknown }

function renderConfigurationDetail(tab: ConfigurationTab, quotaDetail: DetailQuery, limitRangeDetail: DetailQuery, hpaDetail: DetailQuery, pdbDetail: DetailQuery) {
  const loading = <p className="text-sm text-kp-overlay-text" role="status">Loading detail…</p>
  const failed = (error: unknown) => <p className="text-sm text-kp-red" role="alert">{errorMessage(error)}</p>
  switch (tab) {
    case 'resource-quotas':
      return quotaDetail.isPending ? loading : quotaDetail.error ? failed(quotaDetail.error) : quotaDetail.data ? <Facts facts={[
        { label: 'Hard', value: quantitySummary((quotaDetail.data as ResourceQuota).hard) },
        { label: 'Used', value: quantitySummary((quotaDetail.data as ResourceQuota).used) },
        { label: 'Truncated', value: (quotaDetail.data as ResourceQuota).truncated ? 'yes' : 'no' },
      ]} /> : null
    case 'limit-ranges':
      return limitRangeDetail.isPending ? loading : limitRangeDetail.error ? failed(limitRangeDetail.error) : limitRangeDetail.data ? limitRangeItemsView(limitRangeDetail.data as LimitRange) : null
    case 'hpas':
      return hpaDetail.isPending ? loading : hpaDetail.error ? failed(hpaDetail.error) : hpaDetail.data ? hpaDetailView(hpaDetail.data as HorizontalPodAutoscaler) : null
    case 'pdbs':
      return pdbDetail.isPending ? loading : pdbDetail.error ? failed(pdbDetail.error) : pdbDetail.data ? pdbDetailView(pdbDetail.data as PodDisruptionBudget) : null
  }
}

function limitRangeItemsView(detail: LimitRange) {
  return (
    <div className="grid gap-3">
      <Facts facts={[
        { label: 'UID', value: detail.uid },
        { label: 'Items', value: `${detail.items.length}${detail.truncated ? ' (truncated)' : ''}` },
      ]} />
      <div className="overflow-x-auto rounded-lg border border-kp-overlay-0">
        <table className="w-full border-collapse text-left text-sm">
          <thead><tr className="border-b border-kp-overlay-0 text-2xs uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-medium">Type</th><th className="px-2.5 py-1.5 font-medium">Min</th><th className="px-2.5 py-1.5 font-medium">Max</th><th className="px-2.5 py-1.5 font-medium">Default</th></tr></thead>
          <tbody>
            {detail.items.map((item: LimitRangeItem) => (
              <tr key={item.type} className="border-b border-kp-overlay-0/50 last:border-0">
                <td className="px-2.5 py-1.5 text-kp-text">{item.type}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{Object.entries(item.min ?? {}).map(([key, value]) => `${key} ${value}`).join(', ') || '—'}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{Object.entries(item.max ?? {}).map(([key, value]) => `${key} ${value}`).join(', ') || '—'}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{Object.entries(item.default ?? {}).map(([key, value]) => `${key} ${value}`).join(', ') || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function hpaDetailView(detail: HorizontalPodAutoscaler) {
  return (
    <div className="grid gap-3">
      <Facts facts={[
        { label: 'Target', value: `${detail.targetKind}/${detail.targetName}` },
        { label: 'Replicas', value: `min ${detail.minReplicas ?? '—'} · max ${detail.maxReplicas} · current ${detail.currentReplicas} · desired ${detail.desiredReplicas}` },
        { label: 'Metrics', value: detail.metricNames.join(', ') || 'none declared' },
        { label: 'Created', value: dateTime(null) === '—' ? '—' : age(detail.ageSeconds) },
      ]} />
      {detail.conditions.length ? (
        <div className="grid gap-1">
          {detail.conditions.map((condition) => (
            <div key={condition.type} className="flex items-center gap-2 rounded-md border border-kp-overlay-0 bg-kp-surface-1 px-2.5 py-1.5">
              <Badge variant={condition.status === 'True' ? 'healthy' : condition.status === 'False' ? 'danger' : 'unknown'}>{condition.type}</Badge>
              <small className="text-xs text-kp-overlay-text">{condition.message ?? condition.reason ?? ''}</small>
            </div>
          ))}
        </div>
      ) : (
        <p className="m-0 text-xs text-kp-overlay-text">No conditions reported: autoscaling state is unknown, not healthy.</p>
      )}
    </div>
  )
}

function pdbDetailView(detail: PodDisruptionBudget) {
  return (
    <Facts facts={[
      { label: 'Min available', value: intOrStringLabel(detail.minAvailable) },
      { label: 'Max unavailable', value: intOrStringLabel(detail.maxUnavailable) },
      { label: 'Current healthy', value: String(detail.currentHealthy) },
      { label: 'Desired healthy', value: String(detail.desiredHealthy) },
      { label: 'Disruptions allowed', value: String(detail.disruptionsAllowed) },
      { label: 'Expected pods', value: String(detail.expectedPods) },
      { label: 'Selector', value: Object.entries(detail.selector ?? {}).map(([key, value]) => `${key}=${value}`).join(', ') || 'none' },
      { label: 'Age', value: age(detail.ageSeconds) },
    ]} />
  )
}

export function ServiceAccountsPage() {
  const { status, selection } = useActiveSelection()
  const { namespace, name } = useParams<{ namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const [cursor, setCursor] = useState('')
  const [selected, setSelected] = useState<ConfigurationSelection | null>(null)
  const paramSelection = namespace && name && selection ? { namespace, name } : null
  const activeSelected = paramSelection || selected
  const list = useQuery({ queryKey: ['resources', 'service-accounts', selection?.generation, applied, cursor], queryFn: ({ signal }) => getServiceAccounts({ limit: 100, search: applied.search || undefined, continueToken: cursor || undefined, sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }, signal, selection?.generation), enabled: Boolean(selection) })
  const detail = useQuery({ queryKey: ['resources', 'serviceaccount-detail', selection?.generation, activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getServiceAccount(activeSelected!.namespace, activeSelected!.name, signal, selection!.generation), enabled: Boolean(activeSelected) })

  function closeDetail() {
    setSelected(null)
    navigate('/service-accounts')
  }

  return (
    <ResourcePage title="ServiceAccounts" description="Namespace ServiceAccounts as metadata only: no tokens, no Secret references and no arbitrary annotations.">
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft({ ...draft, search: value })} onApply={() => { setApplied(draft); setCursor(''); setSelected(null) }} onRefresh={() => queryClient.invalidateQueries({ queryKey: ['resources', 'service-accounts'] })} onClear={() => { setDraft(initialListState); setApplied(initialListState); setCursor(''); setSelected(null) }} activeFilters={applied.search ? [{ id: 'search', label: 'Search', value: applied.search }] : []} sort={draft.sort} order={draft.order} appliedSort={applied.sort} appliedOrder={applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={draft.search !== applied.search || draft.sort !== applied.sort || draft.order !== applied.order} sortOptions={configurationSortOptions} onSortChange={(value) => setDraft({ ...draft, sort: value })} onOrderChange={(value) => setDraft({ ...draft, order: value })} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={list.isPending} error={list.error} empty={list.data?.items.length === 0}>
          <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
            <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
              <DataTable
                caption="Authorized ServiceAccount page"
                rows={list.data?.items ?? []}
                getRowKey={(item) => `${item.namespace}/${item.name}`}
                columns={[
                  { key: 'namespace', header: 'Namespace / name', cell: (item) => <TableLink aria-label={`Open ServiceAccount ${item.name} in ${item.namespace}`} onClick={() => { setSelected({ namespace: item.namespace, name: item.name }); navigate(`/service-accounts/${encodeURIComponent(item.namespace)}/${encodeURIComponent(item.name)}`) }} primary={item.name} secondary={item.namespace} /> },
                  { key: 'uid', header: 'UID', cell: (item) => <span className="mono text-xs">{item.uid}</span> },
                  { key: 'age', header: 'Age', cell: (item) => age(item.ageSeconds) },
                ] as DataTableColumn<ServiceAccount>[]}
              />
              {list.data ? <CollectionFooter result={list.data} onNext={(next) => { setCursor(next); setSelected(null) }} onRestart={() => { setCursor(''); setSelected(null) }} /> : null}
            </div>
            <Drawer open={Boolean(activeSelected)} onClose={closeDetail} title={detailTitle(activeSelected ? `ServiceAccount ${activeSelected.namespace}/${activeSelected.name}` : 'Resource detail')}>
              {activeSelected ? detail.isPending ? <p className="text-sm text-kp-overlay-text" role="status">Loading metadata…</p> : detail.isError ? <p className="text-sm text-kp-red" role="alert">{errorMessage(detail.error)}</p> : detail.data ? (
                <>
                  <Facts facts={[
                    { label: 'Namespace', value: detail.data.namespace },
                    { label: 'Name', value: detail.data.name },
                    { label: 'UID', value: detail.data.uid },
                    { label: 'Created', value: age(detail.data.ageSeconds) },
                  ]} />
                  <p className="mt-3 rounded-r-md border-l-2 border-kp-yellow-border bg-kp-yellow-bg px-3 py-2 text-sm text-kp-yellow">Tokens, Secret references, annotations and YAML are intentionally unavailable for ServiceAccounts.</p>
                </>
              ) : null : null}
            </Drawer>
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}
