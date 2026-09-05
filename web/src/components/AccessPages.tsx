import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import {
  getClusterRole,
  getClusterRoleBinding,
  getClusterRoleBindings,
  getClusterRoles,
  getCustomResourceDefinitions,
  getMutatingWebhookConfigurations,
  getPriorityClasses,
  getRole,
  getRoleBinding,
  getRoleBindings,
  getRoles,
  getRuntimeClasses,
  getStatus,
  getValidatingWebhookConfigurations,
} from '../api/client'
import type {
  Binding,
  RBACRule,
  CollectionResult,
  CustomResourceDefinition,
  PriorityClass,
  Role,
  RoleDetail,
  RuntimeClass,
  WebhookConfiguration,
} from '../api/types'
import { DataTable, type DataTableColumn, Drawer } from './ui'
import { ResourceListControls } from './ResourceListControls'
import type { ListSortOption } from './ResourceListControls'
import { errorMessage } from './resource/errors'
import { CollectionFooter, QueryState, SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import { TableLink } from './resource/TableLink'
import { Facts } from './resource/Facts'
import { age } from './resource/format'
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

function detailTitle(title: string) {
  return <h2 className="text-lg text-kp-text break-words">{title}</h2>
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
  clusterScoped: boolean
  columns: DataTableColumn<unknown>[]
  rowKey: (row: unknown) => string
  detailOpen: boolean
  detailTitle: string
  detail: React.ReactNode
  onCloseDetail: () => void
  onTabChange: (tab: string) => void
  onCursor: (value: string) => void
  draft: ListState
  applied: ListState
  onDraft: (next: ListState) => void
}

// TabbedFamilyPage renders one group of read-only families on the shared
// resource framework: identical toolbar, table, footer and drawer (spec §16).
function TabbedFamilyPage(props: TabbedFamilyProps) {
  const { status, selection } = useActiveSelection()
  const queryClient = useQueryClient()
  return (
    <ResourcePage title={props.title} description={props.description}>
      <ResourceTabStrip ariaLabel={props.ariaLabel} panelId={`${props.ariaLabel}-panel`} active={props.tab} onChange={props.onTabChange} tabs={props.tabs.map((id) => ({ id, label: id }))} />
      <ResourceListControls search={props.draft.search} appliedSearch={props.applied.search} onSearchChange={(value) => props.onDraft({ ...props.draft, search: value })} onApply={() => props.onCursor('')} onRefresh={() => queryClient.invalidateQueries({ queryKey: props.queryKeys })} onClear={() => props.onDraft({ ...initialListState })} activeFilters={props.applied.search ? [{ id: 'search', label: 'Search', value: props.applied.search }] : []} sort={props.draft.sort} order={props.draft.order} appliedSort={props.applied.sort} appliedOrder={props.applied.order} defaultSort="identity" defaultOrder="asc" hasPendingChanges={props.draft.search !== props.applied.search || props.draft.sort !== props.applied.sort || props.draft.order !== props.applied.order} sortOptions={identityNameSorts} onSortChange={(value) => props.onDraft({ ...props.draft, sort: value })} onOrderChange={(value) => props.onDraft({ ...props.draft, order: value })} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <QueryState pending={props.listQuery.isPending} error={props.listQuery.error} empty={props.result?.items.length === 0}>
          <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.85fr)]">
            <div className="min-w-0 overflow-x-auto border border-kp-overlay-0 rounded-xl bg-kp-surface-0">
              <DataTable caption={`Authorized ${props.tab} page`} rows={props.result?.items ?? []} getRowKey={props.rowKey} columns={props.columns} />
              {props.result ? <CollectionFooter result={props.result} onNext={props.onCursor} onRestart={() => props.onCursor('')} /> : null}
            </div>
            <Drawer open={props.detailOpen} onClose={props.onCloseDetail} title={detailTitle(props.detailTitle)}>
              {props.detailOpen ? props.detail : null}
            </Drawer>
          </div>
        </QueryState>
      </SelectionGate>
    </ResourcePage>
  )
}

function loading() {
  return <p className="text-sm text-kp-overlay-text" role="status">Loading detail…</p>
}
function failed(error: unknown) {
  return <p className="text-sm text-kp-red" role="alert">{errorMessage(error)}</p>
}

type AccessTab = 'roles' | 'role-bindings' | 'cluster-roles' | 'cluster-role-bindings'
const accessTabs: AccessTab[] = ['roles', 'role-bindings', 'cluster-roles', 'cluster-role-bindings']

function accessTabFromParams(tab: string): AccessTab | null {
  return (accessTabs as string[]).includes(tab) ? (tab as AccessTab) : null
}

interface AccessSelection {
  namespace: string | null
  name: string
}

export function AccessControlPage() {
  const { namespace, name } = useParams<{ tab?: string; namespace?: string; name?: string }>()
  const navigate = useNavigate()
  const tab = useMemo(() => accessTabFromParams(namespace ?? '') ?? 'roles', [namespace])
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const [cursor, setCursor] = useState('')
  const [selected, setSelected] = useState<AccessSelection | null>(null)
  const paramSelection = name ? { namespace: namespace ?? null, name } : null
  const activeSelected = paramSelection || selected
  const namespacedTab = tab === 'roles' || tab === 'role-bindings'
  const options = { limit: 100, search: applied.search || undefined, continueToken: cursor || undefined, sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  const roles = useQuery({ queryKey: ['resources', 'roles', applied, cursor], queryFn: ({ signal }) => getRoles(options, signal), enabled: Boolean(tab === 'roles') })
  const roleDetail = useQuery({ queryKey: ['resources', 'role-detail', activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getRole(activeSelected!.namespace!, activeSelected!.name, signal), enabled: Boolean(activeSelected && tab === 'roles') })
  const roleBindingDetail = useQuery({ queryKey: ['resources', 'rolebinding-detail', activeSelected?.namespace, activeSelected?.name], queryFn: ({ signal }) => getRoleBinding(activeSelected!.namespace!, activeSelected!.name, signal), enabled: Boolean(activeSelected && tab === 'role-bindings') })
  const clusterRoleDetail = useQuery({ queryKey: ['resources', 'clusterrole-detail', activeSelected?.name], queryFn: ({ signal }) => getClusterRole(activeSelected!.name, signal), enabled: Boolean(activeSelected && tab === 'cluster-roles') })
  const clusterRoleBindingDetail = useQuery({ queryKey: ['resources', 'clusterrolebinding-detail', activeSelected?.name], queryFn: ({ signal }) => getClusterRoleBinding(activeSelected!.name, signal), enabled: Boolean(activeSelected && tab === 'cluster-role-bindings') })
  const roleBindings = useQuery({ queryKey: ['resources', 'role-bindings', applied, cursor], queryFn: ({ signal }) => getRoleBindings(options, signal), enabled: Boolean(tab === 'role-bindings') })
  const clusterRoles = useQuery({ queryKey: ['resources', 'cluster-roles', applied, cursor], queryFn: ({ signal }) => getClusterRoles(options, signal), enabled: Boolean(tab === 'cluster-roles') })
  const clusterRoleBindings = useQuery({ queryKey: ['resources', 'cluster-role-bindings', applied, cursor], queryFn: ({ signal }) => getClusterRoleBindings(options, signal), enabled: Boolean(tab === 'cluster-role-bindings') })

  const active: CollectionResult<unknown> | undefined =
    tab === 'roles' ? roles.data : tab === 'role-bindings' ? roleBindings.data : tab === 'cluster-roles' ? clusterRoles.data : clusterRoleBindings.data
  const activeQuery = tab === 'roles' ? roles : tab === 'role-bindings' ? roleBindings : tab === 'cluster-roles' ? clusterRoles : clusterRoleBindings

  const columns: DataTableColumn<unknown>[] = tab === 'roles' || tab === 'cluster-roles' ? [
    { key: 'name', header: 'Role', cell: (item) => { const value = item as Role; return <TableLink aria-label={`Open Role ${value.name}`} onClick={() => { setSelected({ namespace: value.namespace ?? null, name: value.name }); navigate(namespacedTab ? `/access/${tab}/${encodeURIComponent(value.namespace)}/${encodeURIComponent(value.name)}` : `/access/${tab}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.namespace ?? 'cluster'} /> } },
    { key: 'rules', header: 'Rules', cell: (item) => (item as Role).ruleCount },
    { key: 'age', header: 'Age', cell: (item) => age((item as Role).ageSeconds) },
  ] : [
    { key: 'name', header: 'Binding', cell: (item) => { const value = item as Binding; return <TableLink aria-label={`Open Binding ${value.name}`} onClick={() => { setSelected({ namespace: value.namespace ?? null, name: value.name }); navigate(namespacedTab ? `/access/${tab}/${encodeURIComponent(value.namespace)}/${encodeURIComponent(value.name)}` : `/access/${tab}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.namespace ?? 'cluster'} /> } },
    { key: 'roleRef', header: 'Role ref', cell: (item) => { const value = item as Binding; return `${value.roleRefKind}/${value.roleRefName}` } },
    { key: 'subjects', header: 'Subjects', cell: (item) => (item as Binding).subjects.length },
    { key: 'age', header: 'Age', cell: (item) => age((item as Binding).ageSeconds) },
  ]

  function closeDetail() {
    setSelected(null)
    navigate(`/access/${tab}`)
  }

  return (
    <TabbedFamilyPage
      title="Access Control" description="Roles and bindings as stored RBAC data; listing rules never calculates effective permissions."
      ariaLabel="Access Control resource type" tabs={accessTabs} tab={tab}
      queryKeys={['resources', tab]} listQuery={activeQuery} result={active}
      clusterScoped={!namespacedTab} columns={columns} rowKey={(row) => { const value = row as { namespace?: string; name: string }; return `${value.namespace ?? ''}/${value.name}` }}
      detailOpen={Boolean(activeSelected)} detailTitle={activeSelected ? `${tab} ${activeSelected.namespace ? `${activeSelected.namespace}/` : ''}${activeSelected.name}` : 'Resource detail'}
      detail={renderAccessDetail(tab, activeSelected, roleDetail, roleBindingDetail, clusterRoleDetail, clusterRoleBindingDetail)}
      onCloseDetail={closeDetail}
      onTabChange={(value) => { setDraft(initialListState); setApplied(initialListState); setCursor(''); setSelected(null); navigate(`/access/${value}`) }}
      onCursor={(value) => { setCursor(value); setSelected(null) }}
      draft={draft} applied={applied} onDraft={setDraft}
    />
  )
}

function renderAccessDetail(tab: AccessTab, target: AccessSelection | null, roles: { isPending: boolean; error: unknown; data?: RoleDetail }, roleBindings: { isPending: boolean; error: unknown; data?: Binding }, clusterRoles: { isPending: boolean; error: unknown; data?: RoleDetail }, clusterRoleBindings: { isPending: boolean; error: unknown; data?: Binding }) {
  if (!target) return null
  if (!target) return null
  const loadingOr = <T,>(query: { isPending: boolean; error: unknown; data?: T }, render: (value: T) => React.ReactNode) =>
    query.isPending ? loading() : query.error ? failed(query.error) : query.data ? render(query.data) : null
  if (tab === 'roles') return loadingOr(roles, (detail: RoleDetail) => rulesView(detail.rules, detail.truncated))
  if (tab === 'cluster-roles') return loadingOr(clusterRoles, (detail: RoleDetail) => rulesView(detail.rules, detail.truncated))
  if (tab === 'role-bindings') return loadingOr(roleBindings, (detail: Binding) => bindingView(detail))
  return loadingOr(clusterRoleBindings, (detail: Binding) => bindingView(detail))
}

function rulesView(rules: RBACRule[], truncated: boolean) {
  return (
    <div className="grid gap-2">
      <div className="overflow-x-auto rounded-lg border border-kp-overlay-0">
        <table className="w-full border-collapse text-left text-sm">
          <thead><tr className="border-b border-kp-overlay-0 text-2xs uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-medium">Groups</th><th className="px-2.5 py-1.5 font-medium">Resources</th><th className="px-2.5 py-1.5 font-medium">Verbs</th></tr></thead>
          <tbody>
            {rules.map((rule, index) => (
              <tr key={index} className="border-b border-kp-overlay-0/50 last:border-0">
                <td className="px-2.5 py-1.5 text-kp-subtext">{rule.apiGroups.join(', ') || 'core'}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{rule.resources.join(', ') || '—'}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{rule.verbs.join(', ') || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {truncated ? <p className="m-0 text-xs text-kp-yellow" role="note">Rule list is truncated; not all rules are visible.</p> : null}
      <p className="m-0 text-xs text-kp-overlay-text">Stored rules never imply effective permissions; check the Permissions screen for SAR decisions.</p>
    </div>
  )
}

function bindingView(detail: Binding) {
  return (
    <div className="grid gap-2">
      <Facts facts={[
        { label: 'Role ref', value: `${detail.roleRefKind}/${detail.roleRefName}` },
        { label: 'Subjects', value: `${detail.subjects.length}${detail.truncated ? ' (truncated)' : ''}` },
        { label: 'Age', value: age(detail.ageSeconds) },
      ]} />
      <div className="overflow-x-auto rounded-lg border border-kp-overlay-0">
        <table className="w-full border-collapse text-left text-sm">
          <thead><tr className="border-b border-kp-overlay-0 text-2xs uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-medium">Kind</th><th className="px-2.5 py-1.5 font-medium">Name</th><th className="px-2.5 py-1.5 font-medium">Namespace</th></tr></thead>
          <tbody>
            {detail.subjects.map((subject) => (
              <tr key={`${subject.kind}/${subject.name}/${subject.namespace ?? ''}`} className="border-b border-kp-overlay-0/50 last:border-0">
                <td className="px-2.5 py-1.5 text-kp-text">{subject.kind}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{subject.name}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{subject.namespace ?? '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="m-0 text-xs text-kp-overlay-text">Bound subjects do not gain UI actions; authorization is always verified per operation.</p>
    </div>
  )
}

type AdministrationTab = 'customresourcedefinitions' | 'priority-classes' | 'runtime-classes' | 'mutating-webhook-configurations' | 'validating-webhook-configurations'
const administrationTabs: AdministrationTab[] = ['customresourcedefinitions', 'priority-classes', 'runtime-classes', 'mutating-webhook-configurations', 'validating-webhook-configurations']

function administrationTabFromParams(tab: string): AdministrationTab | null {
  return (administrationTabs as string[]).includes(tab) ? (tab as AdministrationTab) : null
}

export function AdministrationPage() {
  const { tab: tabParam, name } = useParams<{ tab?: string; name?: string }>()
  const navigate = useNavigate()
  const tab = useMemo(() => administrationTabFromParams(tabParam ?? '') ?? 'customresourcedefinitions', [tabParam])
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const [cursor, setCursor] = useState('')
  const [selected, setSelected] = useState<string | null>(null)
  const paramSelection = name ? { name } : null
  const activeSelected = paramSelection || selected
  const options = { limit: 100, search: applied.search || undefined, continueToken: cursor || undefined, sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  const crds = useQuery({ queryKey: ['resources', 'customresourcedefinitions', applied, cursor], queryFn: ({ signal }) => getCustomResourceDefinitions(options, signal), enabled: Boolean(tab === 'customresourcedefinitions') })
  const priorityClasses = useQuery({ queryKey: ['resources', 'priority-classes', applied, cursor], queryFn: ({ signal }) => getPriorityClasses(options, signal), enabled: Boolean(tab === 'priority-classes') })
  const runtimeClasses = useQuery({ queryKey: ['resources', 'runtime-classes', applied, cursor], queryFn: ({ signal }) => getRuntimeClasses(options, signal), enabled: Boolean(tab === 'runtime-classes') })
  const mutating = useQuery({ queryKey: ['resources', 'mutating-webhook-configurations', applied, cursor], queryFn: ({ signal }) => getMutatingWebhookConfigurations(options, signal), enabled: Boolean(tab === 'mutating-webhook-configurations') })
  const validating = useQuery({ queryKey: ['resources', 'validating-webhook-configurations', applied, cursor], queryFn: ({ signal }) => getValidatingWebhookConfigurations(options, signal), enabled: Boolean(tab === 'validating-webhook-configurations') })

  const active: CollectionResult<unknown> | undefined =
    tab === 'customresourcedefinitions' ? crds.data : tab === 'priority-classes' ? priorityClasses.data : tab === 'runtime-classes' ? runtimeClasses.data : tab === 'mutating-webhook-configurations' ? mutating.data : validating.data
  const activeQuery = tab === 'customresourcedefinitions' ? crds : tab === 'priority-classes' ? priorityClasses : tab === 'runtime-classes' ? runtimeClasses : tab === 'mutating-webhook-configurations' ? mutating : validating

  const columns: DataTableColumn<unknown>[] = (() => {
    switch (tab) {
      case 'customresourcedefinitions':
        return [
          { key: 'name', header: 'CRD', cell: (item) => { const value = item as CustomResourceDefinition; return <TableLink aria-label={`Open CRD ${value.name}`} onClick={() => { setSelected(value.name); navigate(`/administration/${tab}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.group} /> } },
          { key: 'kind', header: 'Kind', cell: (item) => (item as CustomResourceDefinition).kind },
          { key: 'scope', header: 'Scope', cell: (item) => (item as CustomResourceDefinition).scope },
          { key: 'versions', header: 'Versions', cell: (item) => (item as CustomResourceDefinition).versions.map((version) => `${version.name}${version.storage ? '*' : ''}`).join(', ') },
          { key: 'age', header: 'Age', cell: (item) => age((item as CustomResourceDefinition).ageSeconds) },
        ]
      case 'priority-classes':
        return [
          { key: 'name', header: 'Class', cell: (item) => { const value = item as PriorityClass; return <TableLink aria-label={`Open PriorityClass ${value.name}`} onClick={() => { setSelected(value.name); navigate(`/administration/${tab}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.globalDefault ? 'global default' : ''} /> } },
          { key: 'value', header: 'Priority', cell: (item) => (item as PriorityClass).value },
          { key: 'preemption', header: 'Preemption', cell: (item) => (item as PriorityClass).preemptionPolicy ?? 'unknown' },
          { key: 'age', header: 'Age', cell: (item) => age((item as PriorityClass).ageSeconds) },
        ]
      case 'runtime-classes':
        return [
          { key: 'name', header: 'Class', cell: (item) => { const value = item as RuntimeClass; return <TableLink aria-label={`Open RuntimeClass ${value.name}`} onClick={() => { setSelected(value.name); navigate(`/administration/${tab}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={value.handler} /> } },
          { key: 'handler', header: 'Handler', cell: (item) => (item as RuntimeClass).handler },
          { key: 'age', header: 'Age', cell: (item) => age((item as RuntimeClass).ageSeconds) },
        ]
      case 'mutating-webhook-configurations':
      case 'validating-webhook-configurations':
        return [
          { key: 'name', header: 'Configuration', cell: (item) => { const value = item as WebhookConfiguration; return <TableLink aria-label={`Open webhook configuration ${value.name}`} onClick={() => { setSelected(value.name); navigate(`/administration/${tab}/${encodeURIComponent(value.name)}`) }} primary={value.name} secondary={`${value.webhookCount} webhook${value.webhookCount === 1 ? '' : 's'}`} /> } },
          { key: 'count', header: 'Webhooks', cell: (item) => (item as WebhookConfiguration).webhookCount },
          { key: 'age', header: 'Age', cell: (item) => age((item as WebhookConfiguration).ageSeconds) },
        ]
    }
  })()

  function closeDetail() {
    setSelected(null)
    navigate(`/administration/${tab}`)
  }

  const detail = (() => {
    if (!activeSelected) return null
    const selectedName = typeof activeSelected === 'string' ? activeSelected : activeSelected.name
    if (tab === 'customresourcedefinitions') return crds.isPending ? loading() : crds.error ? failed(crds.error) : crds.data ? crdView(crds.data.items.find((item) => (item as CustomResourceDefinition).name === selectedName) as CustomResourceDefinition | undefined) : null
    if (tab === 'priority-classes') return priorityClasses.isPending ? loading() : priorityClasses.error ? failed(priorityClasses.error) : priorityClasses.data ? priorityClassView(priorityClasses.data.items.find((item) => (item as PriorityClass).name === selectedName) as PriorityClass | undefined) : null
    if (tab === 'runtime-classes') return runtimeClasses.isPending ? loading() : runtimeClasses.error ? failed(runtimeClasses.error) : runtimeClasses.data ? runtimeClassView(runtimeClasses.data.items.find((item) => (item as RuntimeClass).name === selectedName) as RuntimeClass | undefined) : null
    if (tab === 'mutating-webhook-configurations') return mutating.isPending ? loading() : mutating.error ? failed(mutating.error) : mutating.data ? webhookView(mutating.data.items.find((item) => (item as WebhookConfiguration).name === selectedName) as WebhookConfiguration | undefined) : null
    return validating.isPending ? loading() : validating.error ? failed(validating.error) : validating.data ? webhookView(validating.data.items.find((item) => (item as WebhookConfiguration).name === selectedName) as WebhookConfiguration | undefined) : null
  })()

  return (
    <TabbedFamilyPage
      title="Administration" description="Cluster administration objects; CRD discovery never implies access to custom resource instances."
      ariaLabel="Administration resource type" tabs={administrationTabs} tab={tab}
      queryKeys={['resources', tab]} listQuery={activeQuery} result={active}
      clusterScoped columns={columns} rowKey={(row) => (row as { name: string }).name}
      detailOpen={Boolean(activeSelected)} detailTitle={activeSelected ? `${tab} ${activeSelected}` : 'Resource detail'}
      detail={detail} onCloseDetail={closeDetail}
      onTabChange={(value) => { setDraft(initialListState); setApplied(initialListState); setCursor(''); setSelected(null); navigate(`/administration/${value}`) }}
      onCursor={(value) => { setCursor(value); setSelected(null) }}
      draft={draft} applied={applied} onDraft={setDraft}
    />
  )
}

function crdView(value: CustomResourceDefinition | undefined) {
  if (!value) return <p className="text-sm text-kp-overlay-text">Detail is not in the current page; open it from the list.</p>
  return (
    <Facts facts={[
      { label: 'Group / Kind', value: `${value.group} / ${value.kind}` },
      { label: 'Scope', value: value.scope },
      { label: 'Versions', value: value.versions.map((version) => `${version.name} (served=${version.served}, storage=${version.storage})`).join(', ') || 'none' },
      { label: 'Conditions', value: value.conditions.map((condition) => `${condition.type}=${condition.status}`).join(', ') || 'unknown' },
      { label: 'Age', value: age(value.ageSeconds) },
    ]} />
  )
}

function priorityClassView(value: PriorityClass | undefined) {
  if (!value) return <p className="text-sm text-kp-overlay-text">Detail is not in the current page; open it from the list.</p>
  return <Facts facts={[
    { label: 'Value', value: String(value.value) },
    { label: 'Global default', value: value.globalDefault ? 'yes' : 'no' },
    { label: 'Preemption policy', value: value.preemptionPolicy ?? 'unknown' },
    { label: 'Age', value: age(value.ageSeconds) },
  ]} />
}

function runtimeClassView(value: RuntimeClass | undefined) {
  if (!value) return <p className="text-sm text-kp-overlay-text">Detail is not in the current page; open it from the list.</p>
  return <Facts facts={[
    { label: 'Handler', value: value.handler },
    { label: 'Overhead', value: Object.entries(value.overhead ?? {}).map(([key, overhead]) => `${key} ${overhead}`).join(', ') || 'not declared' },
    { label: 'Age', value: age(value.ageSeconds) },
  ]} />
}

function webhookView(value: WebhookConfiguration | undefined) {
  if (!value) return <p className="text-sm text-kp-overlay-text">Detail is not in the current page; open it from the list.</p>
  return (
    <div className="grid gap-2">
      <Facts facts={[
        { label: 'Webhooks', value: `${value.webhookCount}${value.truncated ? ' (truncated)' : ''}` },
        { label: 'Age', value: age(value.ageSeconds) },
      ]} />
      <div className="overflow-x-auto rounded-lg border border-kp-overlay-0">
        <table className="w-full border-collapse text-left text-sm">
          <thead><tr className="border-b border-kp-overlay-0 text-2xs uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-medium">Webhook</th><th className="px-2.5 py-1.5 font-medium">Failure policy</th><th className="px-2.5 py-1.5 font-medium">Rules</th></tr></thead>
          <tbody>
            {value.webhooks.map((webhook) => (
              <tr key={webhook.name} className="border-b border-kp-overlay-0/50 last:border-0">
                <td className="px-2.5 py-1.5 text-kp-text">{webhook.name}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{webhook.failurePolicy ?? 'unknown'}</td>
                <td className="px-2.5 py-1.5 text-kp-subtext">{webhook.rules.map((rule) => `${rule.verbs.join('|')} ${rule.apiGroups.join('|')}/${rule.resources.join('|')}`).join('; ') || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="m-0 rounded-r-md border-l-2 border-kp-yellow-border bg-kp-yellow-bg px-3 py-2 text-sm text-kp-yellow">CA bundles, webhook URLs and service references are intentionally unavailable; YAML is not offered for webhook configurations.</p>
    </div>
  )
}
