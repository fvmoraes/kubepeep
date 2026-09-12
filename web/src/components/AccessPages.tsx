import { useGenerationCursor } from './resource/useListCursor'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import {
  getClusterRoleBindings,
  getClusterRoles,
  getCustomResourceDefinitions,
  getMutatingWebhookConfigurations,
  getPriorityClasses,
  getRoleBindings,
  getRoles,
  getRuntimeClasses,
  getStatus,
  getValidatingWebhookConfigurations,
} from '../api/client'
import type {
  Binding,
  CollectionResult,
  CustomResourceDefinition,
  PriorityClass,
  Role,
  RuntimeClass,
  WebhookConfiguration,
} from '../api/types'
import { DataTable, type DataTableColumn } from './ui'
import { ResourceListControls } from './ResourceListControls'
import type { ListSortOption } from './ResourceListControls'
import { CollectionFooter, QueryState, SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import { TableLink } from './resource/TableLink'
import { age } from './resource/format'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { bindListInteraction, listInteractionFor } from '../observability/uxMetrics'
import { useResourceWorkspace } from './workspace/ResourceWorkspaceProvider'

interface ListState {
  search: string
  sort: string
  order: 'asc' | 'desc'
}

const initialListState: ListState = { search: '', sort: 'identity', order: 'asc' }
const identityNameSorts: readonly ListSortOption[] = [
  { value: 'identity', label: 'Name' },
  { value: 'name', label: 'Name (natural)' },
]

function useActiveSelection() {
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  return { status, selection: status.data?.selection ?? null }
}

interface TabbedFamilyProps {
  title: string
  description: string
  ariaLabel: string
  tabs: readonly string[]
  tab: string
  queryKeys: string[]
  listQuery: { isPending: boolean; error: unknown }
  result: CollectionResult<unknown> | undefined
  columns: DataTableColumn<unknown>[]
  rowKey: (row: unknown) => string
  onTabChange: (tab: string) => void
  currentCursor: string
  onCursor: (value: string) => void
  onApply: (interactionId: string) => void
  onClear: () => void
  draft: ListState
  applied: ListState
  onDraft: (next: ListState) => void
}

// TabbedFamilyPage renders one group of read-only families on the shared
// resource framework: identical toolbar, table and footer. Details open in the
// Resource Workspace, so no drawer lives here anymore.
function TabbedFamilyPage(props: TabbedFamilyProps) {
  const { status, selection } = useActiveSelection()
  const queryClient = useQueryClient()
  return (
    <ResourcePage title={props.title} description={props.description}>
      <ResourceTabStrip ariaLabel={props.ariaLabel} panelId={`${props.ariaLabel}-panel`} active={props.tab} onChange={props.onTabChange} tabs={props.tabs.map((id) => ({ id, label: id }))} />
      <ResourceListControls search={props.draft.search} appliedSearch={props.applied.search} onSearchChange={(value) => props.onDraft({ ...props.draft, search: value })} onApply={props.onApply} onRefresh={() => queryClient.invalidateQueries({ queryKey: props.queryKeys })} onClear={props.onClear} sort={props.draft.sort} order={props.draft.order} appliedSort={props.applied.sort} appliedOrder={props.applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={props.draft.search !== props.applied.search || props.draft.sort !== props.applied.sort || props.draft.order !== props.applied.order} sortOptions={identityNameSorts} onSortChange={(value) => props.onDraft({ ...props.draft, sort: value })} onOrderChange={(value) => props.onDraft({ ...props.draft, order: value })} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={props.listQuery.isPending} error={props.listQuery.error} empty={props.result?.items.length === 0}>
          <div className="min-w-0 overflow-x-auto rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
            <DataTable caption={`Authorized ${props.tab} page`} rows={props.result?.items ?? []} getRowKey={props.rowKey} columns={props.columns} stickyHeader />
            {props.result ? <CollectionFooter result={props.result} currentCursor={props.currentCursor} onNext={props.onCursor} onRestart={() => props.onCursor('')} /> : null}
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}

type AccessTab = 'roles' | 'role-bindings' | 'cluster-roles' | 'cluster-role-bindings'
const accessTabs: AccessTab[] = ['roles', 'role-bindings', 'cluster-roles', 'cluster-role-bindings']

function accessTabFromParams(tab: string): AccessTab | null {
  return (accessTabs as string[]).includes(tab) ? (tab as AccessTab) : null
}

export function AccessControlPage() {
  const { selection } = useActiveSelection()
  const globalNamespace = useGlobalNamespace()
  const workspace = useResourceWorkspace()
  const { tab: tabParam, namespace, name } = useParams<{ tab?: string; namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const generation = selection?.generation
  // BUG FIX: the active tab comes from the :tab route segment. Previously the
  // namespace segment was read here, which made ClusterRoles and
  // ClusterRoleBindings unreachable from the sidebar.
  const tab = useMemo(() => accessTabFromParams(tabParam ?? '') ?? 'roles', [tabParam])
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const namespacedTab = tab === 'roles' || tab === 'role-bindings'
  const [cursor, setCursor] = useGenerationCursor(generation, JSON.stringify([tab, namespacedTab ? globalNamespace.value : '']))
  const options = { limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, continueToken: cursor || undefined, namespaces: namespacedTab ? effectiveNamespaces(globalNamespace.value, []) : undefined, sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  // Deep links open the Resource Workspace (cluster tabs use 3 segments,
  // namespaced tabs 4).
  useEffect(() => {
    if (!name || !generation || !tab) return
    workspace.openFromRoute({ collection: tab, namespace: namespacedTab ? namespace ?? null : null, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tab, namespace, name, generation])

  const roles = useQuery({ queryKey: ['resources', 'roles', generation, globalNamespace.value, applied, cursor], queryFn: ({ signal }) => getRoles(options, signal, generation), enabled: Boolean(selection && tab === 'roles') })
  const roleBindings = useQuery({ queryKey: ['resources', 'role-bindings', generation, globalNamespace.value, applied, cursor], queryFn: ({ signal }) => getRoleBindings(options, signal, generation), enabled: Boolean(selection && tab === 'role-bindings') })
  const clusterRoles = useQuery({ queryKey: ['resources', 'cluster-roles', generation, applied, cursor], queryFn: ({ signal }) => getClusterRoles(options, signal, generation), enabled: Boolean(selection && tab === 'cluster-roles') })
  const clusterRoleBindings = useQuery({ queryKey: ['resources', 'cluster-role-bindings', generation, applied, cursor], queryFn: ({ signal }) => getClusterRoleBindings(options, signal, generation), enabled: Boolean(selection && tab === 'cluster-role-bindings') })

  const active: CollectionResult<unknown> | undefined =
    tab === 'roles' ? roles.data : tab === 'role-bindings' ? roleBindings.data : tab === 'cluster-roles' ? clusterRoles.data : clusterRoleBindings.data
  const activeQuery = tab === 'roles' ? roles : tab === 'role-bindings' ? roleBindings : tab === 'cluster-roles' ? clusterRoles : clusterRoleBindings

  const columns: DataTableColumn<unknown>[] = tab === 'roles' || tab === 'cluster-roles' ? [
    { key: 'name', header: 'Role', cell: (item) => { const value = item as Role; return <TableLink aria-label={`Open Role ${value.name}`} onClick={() => workspace.openResource({ collection: tab, namespace: namespacedTab ? value.namespace : null, name: value.name })} primary={value.name} secondary={value.namespace ?? 'cluster'} /> } },
    { key: 'rules', header: 'Rules', cell: (item) => (item as Role).ruleCount },
    { key: 'age', header: 'Age', cell: (item) => age((item as Role).ageSeconds) },
  ] : [
    { key: 'name', header: 'Binding', cell: (item) => { const value = item as Binding; return <TableLink aria-label={`Open Binding ${value.name}`} onClick={() => workspace.openResource({ collection: tab, namespace: namespacedTab ? value.namespace : null, name: value.name })} primary={value.name} secondary={value.namespace ?? 'cluster'} /> } },
    { key: 'roleRef', header: 'Role ref', cell: (item) => { const value = item as Binding; return `${value.roleRefKind}/${value.roleRefName}` } },
    { key: 'subjects', header: 'Subjects', cell: (item) => (item as Binding).subjects.length },
    { key: 'age', header: 'Age', cell: (item) => age((item as Binding).ageSeconds) },
  ]

  return (
    <TabbedFamilyPage
      title="Access Control" description="Roles and bindings as stored RBAC data; listing rules never calculates effective permissions."
      ariaLabel="Access Control resource type" tabs={accessTabs} tab={tab}
      queryKeys={['resources', tab]} listQuery={activeQuery} result={active}
      columns={columns} rowKey={(row) => { const value = row as { namespace?: string; name: string }; return `${value.namespace ?? ''}/${value.name}` }}
      onTabChange={(value) => { setDraft(initialListState); setApplied(initialListState); setCursor(''); navigate(`/access/${value}`) }}
      currentCursor={cursor}
      onCursor={setCursor}
      onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)); setCursor('') }}
      onClear={() => { setDraft(initialListState); setApplied(initialListState); setCursor('') }}
      draft={draft} applied={applied} onDraft={setDraft}
    />
  )
}

type AdministrationTab = 'customresourcedefinitions' | 'priority-classes' | 'runtime-classes' | 'mutating-webhook-configurations' | 'validating-webhook-configurations'
const administrationTabs: AdministrationTab[] = ['customresourcedefinitions', 'priority-classes', 'runtime-classes', 'mutating-webhook-configurations', 'validating-webhook-configurations']

function administrationTabFromParams(tab: string): AdministrationTab | null {
  return (administrationTabs as string[]).includes(tab) ? (tab as AdministrationTab) : null
}

export function AdministrationPage() {
  const { selection } = useActiveSelection()
  const workspace = useResourceWorkspace()
  const { tab: tabParam, name } = useParams<{ tab?: string; name?: string }>()
  const navigate = useNavigate()
  const generation = selection?.generation
  const tab = useMemo(() => administrationTabFromParams(tabParam ?? '') ?? 'customresourcedefinitions', [tabParam])
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const [cursor, setCursor] = useGenerationCursor(generation, tab)
  const options = { limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, continueToken: cursor || undefined, sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  useEffect(() => {
    if (!name || !generation || !tab) return
    workspace.openFromRoute({ collection: tab, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tab, name, generation])

  const crds = useQuery({ queryKey: ['resources', 'customresourcedefinitions', generation, applied, cursor], queryFn: ({ signal }) => getCustomResourceDefinitions(options, signal, generation), enabled: Boolean(selection && tab === 'customresourcedefinitions') })
  const priorityClasses = useQuery({ queryKey: ['resources', 'priority-classes', generation, applied, cursor], queryFn: ({ signal }) => getPriorityClasses(options, signal, generation), enabled: Boolean(selection && tab === 'priority-classes') })
  const runtimeClasses = useQuery({ queryKey: ['resources', 'runtime-classes', generation, applied, cursor], queryFn: ({ signal }) => getRuntimeClasses(options, signal, generation), enabled: Boolean(selection && tab === 'runtime-classes') })
  const mutating = useQuery({ queryKey: ['resources', 'mutating-webhook-configurations', generation, applied, cursor], queryFn: ({ signal }) => getMutatingWebhookConfigurations(options, signal, generation), enabled: Boolean(selection && tab === 'mutating-webhook-configurations') })
  const validating = useQuery({ queryKey: ['resources', 'validating-webhook-configurations', generation, applied, cursor], queryFn: ({ signal }) => getValidatingWebhookConfigurations(options, signal, generation), enabled: Boolean(selection && tab === 'validating-webhook-configurations') })

  const active: CollectionResult<unknown> | undefined =
    tab === 'customresourcedefinitions' ? crds.data : tab === 'priority-classes' ? priorityClasses.data : tab === 'runtime-classes' ? runtimeClasses.data : tab === 'mutating-webhook-configurations' ? mutating.data : validating.data
  const activeQuery = tab === 'customresourcedefinitions' ? crds : tab === 'priority-classes' ? priorityClasses : tab === 'runtime-classes' ? runtimeClasses : tab === 'mutating-webhook-configurations' ? mutating : validating

  const columns: DataTableColumn<unknown>[] = (() => {
    switch (tab) {
      case 'customresourcedefinitions':
        return [
          { key: 'name', header: 'CRD', cell: (item) => { const value = item as CustomResourceDefinition; return <TableLink aria-label={`Open CRD ${value.name}`} onClick={() => workspace.openResource({ collection: tab, name: value.name })} primary={value.name} secondary={value.group} /> } },
          { key: 'kind', header: 'Kind', cell: (item) => (item as CustomResourceDefinition).kind },
          { key: 'scope', header: 'Scope', cell: (item) => (item as CustomResourceDefinition).scope },
          { key: 'versions', header: 'Versions', cell: (item) => (item as CustomResourceDefinition).versions.map((version) => `${version.name}${version.storage ? '*' : ''}`).join(', ') },
          { key: 'age', header: 'Age', cell: (item) => age((item as CustomResourceDefinition).ageSeconds) },
        ]
      case 'priority-classes':
        return [
          { key: 'name', header: 'Class', cell: (item) => { const value = item as PriorityClass; return <TableLink aria-label={`Open PriorityClass ${value.name}`} onClick={() => workspace.openResource({ collection: tab, name: value.name })} primary={value.name} secondary={value.globalDefault ? 'global default' : ''} /> } },
          { key: 'value', header: 'Priority', cell: (item) => (item as PriorityClass).value },
          { key: 'preemption', header: 'Preemption', cell: (item) => (item as PriorityClass).preemptionPolicy ?? 'unknown' },
          { key: 'age', header: 'Age', cell: (item) => age((item as PriorityClass).ageSeconds) },
        ]
      case 'runtime-classes':
        return [
          { key: 'name', header: 'Class', cell: (item) => { const value = item as RuntimeClass; return <TableLink aria-label={`Open RuntimeClass ${value.name}`} onClick={() => workspace.openResource({ collection: tab, name: value.name })} primary={value.name} secondary={value.handler} /> } },
          { key: 'handler', header: 'Handler', cell: (item) => (item as RuntimeClass).handler },
          { key: 'age', header: 'Age', cell: (item) => age((item as RuntimeClass).ageSeconds) },
        ]
      case 'mutating-webhook-configurations':
      case 'validating-webhook-configurations':
        return [
          { key: 'name', header: 'Configuration', cell: (item) => { const value = item as WebhookConfiguration; return <TableLink aria-label={`Open webhook configuration ${value.name}`} onClick={() => workspace.openResource({ collection: tab, name: value.name })} primary={value.name} secondary={`${value.webhookCount} webhook${value.webhookCount === 1 ? '' : 's'}`} /> } },
          { key: 'count', header: 'Webhooks', cell: (item) => (item as WebhookConfiguration).webhookCount },
          { key: 'age', header: 'Age', cell: (item) => age((item as WebhookConfiguration).ageSeconds) },
        ]
    }
  })()

  return (
    <TabbedFamilyPage
      title="Administration" description="Cluster administration objects; CRD discovery never implies access to custom resource instances."
      ariaLabel="Administration resource type" tabs={administrationTabs} tab={tab}
      queryKeys={['resources', tab]} listQuery={activeQuery} result={active}
      columns={columns} rowKey={(row) => (row as { name: string }).name}
      onTabChange={(value) => { setDraft(initialListState); setApplied(initialListState); setCursor(''); navigate(`/administration/${value}`) }}
      currentCursor={cursor}
      onCursor={setCursor}
      onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)); setCursor('') }}
      onClear={() => { setDraft(initialListState); setApplied(initialListState); setCursor('') }}
      draft={draft} applied={applied} onDraft={setDraft}
    />
  )
}
