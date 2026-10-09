import { useInfiniteCollection } from './resource/useInfiniteCollection'
import { ResourceCollectionTable } from './resource/ResourceCollectionTable'
import { QuantityUsage, QuotaUsage } from './resource/QuantityUsage'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router'

import {
  getHPAs,
  getLimitRanges,
  getPDBs,
  getResourceQuotas,
  getServiceAccounts,
  getStatus,
} from '../api/client'
import type {
  HorizontalPodAutoscaler,
  LimitRange,
  PodDisruptionBudget,
  ResourceQuota,
  ServiceAccount,
} from '../api/types'
import { Badge, type DataTableColumn } from './ui'
import { ResourceListControls } from './ResourceListControls'
import { SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'

import { usePreferenceColumnVisibility } from './resource/columns'
import { TableLink } from './resource/TableLink'
import { age } from './resource/format'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { bindListInteraction, listInteractionFor } from '../observability/uxMetrics'
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

  const generation = selection?.generation
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const tab = useMemo(() => configurationTabFromParams(tabParam ?? '') ?? 'resource-quotas', [tabParam])
  const options = { limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, namespaces: effectiveNamespaces(globalNamespace.value, []), sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  // Deep links (/configuration/:tab/:ns/:name) open the Resource Workspace.
  useEffect(() => {
    if (!tab || !namespace || !name || !generation) return
    workspace.openFromRoute({ collection: tab, namespace, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tab, namespace, name, generation])

  const fetchList = { 'resource-quotas': getResourceQuotas, 'limit-ranges': getLimitRanges, hpas: getHPAs, pdbs: getPDBs }[tab]
  const collection = useInfiniteCollection<unknown>({
    identity: ['resources', tab, generation, globalNamespace.value],
    filters: applied,
    enabled: Boolean(selection),
    fetchPage: (cursor, signal, prefetch, focus) => fetchList({ ...options, ...focus, continueToken: cursor || undefined, prefetch }, signal, generation),
  })

  const columnVisibility = usePreferenceColumnVisibility(tab)
  const columns: DataTableColumn<unknown>[] = (() => {
    switch (tab) {
      case 'resource-quotas':
        return [
          { key: 'namespace', header: 'Namespace', cell: (item) => (item as ResourceQuota).namespace },
          { key: 'name', header: 'Name', cell: (item) => { const value = item as ResourceQuota; return <TableLink aria-label={`Open quota ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} /> } },
          { key: 'hard', header: 'Hard', cell: (item) => quantitySummary((item as ResourceQuota).hard) },
          { key: 'used', header: 'Used', cell: (item) => quantitySummary((item as ResourceQuota).used) },
          { key: 'usage', header: 'Usage', cell: (item) => <QuotaUsage quota={item as ResourceQuota} /> },
        ]
      case 'limit-ranges':
        return [
          { key: 'namespace', header: 'Namespace', cell: (item) => (item as LimitRange).namespace },
          { key: 'name', header: 'Name', cell: (item) => { const value = item as LimitRange; return <TableLink aria-label={`Open limit range ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} /> } },
          { key: 'items', header: 'Items', cell: (item) => (item as LimitRange).items.length },
          { key: 'types', header: 'Types', cell: (item) => [...new Set((item as LimitRange).items.map((limit) => limit.type))].join(', ') || 'none' },
        ]
      case 'hpas':
        return [
          { key: 'namespace', header: 'Namespace', cell: (item) => (item as HorizontalPodAutoscaler).namespace },
          { key: 'name', header: 'Name', cell: (item) => { const value = item as HorizontalPodAutoscaler; return <TableLink aria-label={`Open autoscaler ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} /> } },
          { key: 'target', header: 'Target', cell: (item) => `${(item as HorizontalPodAutoscaler).targetKind}/${(item as HorizontalPodAutoscaler).targetName}` },
          { key: 'minmax', header: 'Min / Max', cell: (item) => `${(item as HorizontalPodAutoscaler).minReplicas ?? '—'} / ${(item as HorizontalPodAutoscaler).maxReplicas}` },
          { key: 'replicas', header: 'Current / Desired', cell: (item) => `${(item as HorizontalPodAutoscaler).currentReplicas} / ${(item as HorizontalPodAutoscaler).desiredReplicas}` },
          { key: 'capacity', header: 'Replica capacity', value: (item) => (item as HorizontalPodAutoscaler).currentReplicas / (item as HorizontalPodAutoscaler).maxReplicas, cell: (item) => <QuantityUsage label="HPA replica capacity" current={(item as HorizontalPodAutoscaler).currentReplicas} total={(item as HorizontalPodAutoscaler).maxReplicas} /> },
          { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age((item as HorizontalPodAutoscaler).ageSeconds) },
        ]
      case 'pdbs':
        return [
          { key: 'namespace', header: 'Namespace', cell: (item) => (item as PodDisruptionBudget).namespace },
          { key: 'name', header: 'Name', cell: (item) => { const value = item as PodDisruptionBudget; return <TableLink aria-label={`Open budget ${value.name} in ${value.namespace}`} onClick={() => workspace.openResource({ collection: tab, namespace: value.namespace, name: value.name })} primary={value.name} /> } },
          { key: 'allowed', header: 'Disruptions allowed', cell: (item) => { const allowed = (item as PodDisruptionBudget).disruptionsAllowed; return allowed > 0 ? <Badge variant="healthy">{allowed}</Badge> : <Badge variant={allowed === 0 ? 'warning' : 'unknown'}>{allowed}</Badge> } },
          { key: 'healthy', header: 'Healthy / Desired', value: (item) => (item as PodDisruptionBudget).currentHealthy, cell: (item) => <QuantityUsage label="PDB healthy pods" current={(item as PodDisruptionBudget).currentHealthy} total={(item as PodDisruptionBudget).desiredHealthy} completion failed={(item as PodDisruptionBudget).currentHealthy < (item as PodDisruptionBudget).desiredHealthy} /> },
          { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age((item as PodDisruptionBudget).ageSeconds) },
        ]
    }
  })()

  return (
    <ResourcePage title="Configuration" description="Quotas, limits, autoscalers and disruption budgets in the active scope; absence and unknown stay distinct from zero.">
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft({ ...draft, search: value })} onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <ResourceCollectionTable key={tab} collection={collection} columnVisibility={columnVisibility} caption={`Authorized ${tab} page`} columns={columns} getRowKey={(item: unknown) => { const value = item as { namespace: string; name: string }; return `${value.namespace}/${value.name}` }} />
      </SelectionGate>
    </ResourcePage>
  )
}

export function ServiceAccountsPage() {
  const { status, selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const { namespace, name } = useParams<{ namespace?: string; name?: string }>()

  const generation = selection?.generation
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)

  useEffect(() => {
    if (!namespace || !name || !generation) return
    workspace.openFromRoute({ collection: 'service-accounts', namespace, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [namespace, name, generation])

  const columnVisibility = usePreferenceColumnVisibility('service-accounts')
  const collection = useInfiniteCollection<ServiceAccount>({
    identity: ['resources', 'service-accounts', generation, globalNamespace.value],
    filters: applied,
    enabled: Boolean(selection),
    fetchPage: (cursor, signal, prefetch, focus) => getServiceAccounts({ ...focus, limit: 100, prefetch, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, continueToken: cursor || undefined, namespaces: effectiveNamespaces(globalNamespace.value, []) }, signal, generation),
  })

  return (
    <ResourcePage title="ServiceAccounts" description="Namespace ServiceAccounts as metadata only: no tokens, no Secret references and no arbitrary annotations.">
      <ResourceListControls search={draft.search} appliedSearch={applied.search} onSearchChange={(value) => setDraft({ ...draft, search: value })} onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <ResourceCollectionTable
              caption="Authorized ServiceAccount page"
              collection={collection}
              columnVisibility={columnVisibility}
              getRowKey={(item) => `${item.namespace}/${item.name}`}
              columns={[
                { key: 'namespace', header: 'Namespace', cell: (item) => item.namespace },
                { key: 'name', header: 'Name', cell: (item) => <TableLink aria-label={`Open ServiceAccount ${item.name} in ${item.namespace}`} onClick={() => workspace.openResource({ collection: 'service-accounts', namespace: item.namespace, name: item.name })} primary={item.name} /> },
                { key: 'uid', header: 'UID', cell: (item) => <span className="text-content">{item.uid}</span> },
                { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age(item.ageSeconds) },
              ] as DataTableColumn<ServiceAccount>[]}
            />
      </SelectionGate>
    </ResourcePage>
  )
}
