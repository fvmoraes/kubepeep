import { useInfiniteCollection } from './resource/useInfiniteCollection'
import { ResourceCollectionTable } from './resource/ResourceCollectionTable'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router'

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
  CustomResourceDefinition,
  PriorityClass,
  Role,
  RuntimeClass,
  WebhookConfiguration,
} from '../api/types'
import { type DataTableColumn } from './ui'
import { ResourceListControls } from './ResourceListControls'

import { SelectionGate } from './resource/states'
import { ResourcePage } from './resource/ResourcePage'
import { usePreferenceColumnVisibility } from './resource/columns'
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

function useActiveSelection() {
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  return { status, selection: status.data?.selection ?? null }
}

interface TabbedFamilyProps {
  tab: string
  title: string
  description: string
  collection: ReturnType<typeof useInfiniteCollection<unknown>>
  columns: DataTableColumn<unknown>[]
  rowKey: (row: unknown) => string
  onApply: (interactionId: string) => void
  draft: ListState
  applied: ListState
  onDraft: (next: ListState) => void
}

// TabbedFamilyPage renders one group of read-only families on the shared
// resource framework: identical toolbar, table and footer. Details open in the
// Resource Workspace, so no drawer lives here anymore.
function TabbedFamilyPage(props: TabbedFamilyProps) {
  const columnVisibility = usePreferenceColumnVisibility(props.tab)
  const { status, selection } = useActiveSelection()

  return (
    <ResourcePage title={props.title} description={props.description}>
      <ResourceListControls search={props.draft.search} appliedSearch={props.applied.search} onSearchChange={(value) => props.onDraft({ ...props.draft, search: value })} onApply={props.onApply} />
      <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(selection)}>
        <ResourceCollectionTable key={props.tab} collection={props.collection} columnVisibility={columnVisibility} caption={`Authorized ${props.tab} page`} columns={props.columns} getRowKey={props.rowKey} />
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
  const generation = selection?.generation
  // BUG FIX: the active tab comes from the :tab route segment. Previously the
  // namespace segment was read here, which made ClusterRoles and
  // ClusterRoleBindings unreachable from the sidebar.
  const tab = useMemo(() => accessTabFromParams(tabParam ?? '') ?? 'roles', [tabParam])
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const namespacedTab = tab === 'roles' || tab === 'role-bindings'
  const options = { limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, namespaces: namespacedTab ? effectiveNamespaces(globalNamespace.value, []) : undefined, sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  // Deep links open the Resource Workspace (cluster tabs use 3 segments,
  // namespaced tabs 4).
  useEffect(() => {
    if (!name || !generation || !tab) return
    workspace.openFromRoute({ collection: tab, namespace: namespacedTab ? namespace ?? null : null, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tab, namespace, name, generation])

  const fetchList = { roles: getRoles, 'role-bindings': getRoleBindings, 'cluster-roles': getClusterRoles, 'cluster-role-bindings': getClusterRoleBindings }[tab]
  const collection = useInfiniteCollection<unknown>({
    identity: ['resources', tab, generation, namespacedTab ? globalNamespace.value : ''],
    filters: applied,
    enabled: Boolean(selection),
    fetchPage: (cursor, signal, prefetch, focus) => fetchList({ ...options, ...focus, continueToken: cursor || undefined, prefetch }, signal, generation),
  })

  const columns: DataTableColumn<unknown>[] = tab === 'roles' || tab === 'cluster-roles' ? [
    { key: 'name', header: 'Role', cell: (item) => { const value = item as Role; return <TableLink aria-label={`Open Role ${value.name}`} onClick={() => workspace.openResource({ collection: tab, namespace: namespacedTab ? value.namespace : null, name: value.name })} primary={value.name} /> } },
    { key: 'rules', header: 'Rules', cell: (item) => (item as Role).ruleCount },
    { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age((item as Role).ageSeconds) },
  ] : [
    { key: 'name', header: 'Binding', cell: (item) => { const value = item as Binding; return <TableLink aria-label={`Open Binding ${value.name}`} onClick={() => workspace.openResource({ collection: tab, namespace: namespacedTab ? value.namespace : null, name: value.name })} primary={value.name} /> } },
    { key: 'role-ref', header: 'Role ref', cell: (item) => { const value = item as Binding; return `${value.roleRefKind}/${value.roleRefName}` } },
    { key: 'subjects', header: 'Subjects', cell: (item) => (item as Binding).subjects.length },
    { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age((item as Binding).ageSeconds) },
  ]

  if (namespacedTab) columns.unshift({ key: 'namespace', header: 'Namespace', cell: (item) => (item as Role | Binding).namespace ?? '—' })

  return (
    <TabbedFamilyPage
      title="Access Control" description="Roles and bindings as stored RBAC data; listing rules never calculates effective permissions."
      tab={tab} collection={collection}
      columns={columns} rowKey={(row) => { const value = row as { namespace?: string; name: string }; return `${value.namespace ?? ''}/${value.name}` }}
      onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }}
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
  const generation = selection?.generation
  const tab = useMemo(() => administrationTabFromParams(tabParam ?? '') ?? 'customresourcedefinitions', [tabParam])
  const [draft, setDraft] = useState<ListState>(initialListState)
  const [applied, setApplied] = useState<ListState>(initialListState)
  const options = { limit: 100, uxInteractionId: listInteractionFor(applied), search: applied.search || undefined, sort: applied.sort === 'identity' ? undefined : applied.sort, order: applied.sort === 'identity' && applied.order === 'asc' ? undefined : applied.order }

  useEffect(() => {
    if (!name || !generation || !tab) return
    workspace.openFromRoute({ collection: tab, name })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to route param changes
  }, [tab, name, generation])

  const fetchList = { customresourcedefinitions: getCustomResourceDefinitions, 'priority-classes': getPriorityClasses, 'runtime-classes': getRuntimeClasses, 'mutating-webhook-configurations': getMutatingWebhookConfigurations, 'validating-webhook-configurations': getValidatingWebhookConfigurations }[tab]
  const collection = useInfiniteCollection<unknown>({
    identity: ['resources', tab, generation],
    filters: applied,
    enabled: Boolean(selection),
    fetchPage: (cursor, signal, prefetch, focus) => fetchList({ ...options, ...focus, continueToken: cursor || undefined, prefetch }, signal, generation),
  })

  const columns: DataTableColumn<unknown>[] = (() => {
    switch (tab) {
      case 'customresourcedefinitions':
        return [
          { key: 'name', header: 'CRD', cell: (item) => { const value = item as CustomResourceDefinition; return <TableLink aria-label={`Open CRD ${value.name}`} onClick={() => workspace.openResource({ collection: tab, name: value.name })} primary={value.name} secondary={value.group} /> } },
          { key: 'kind', header: 'Kind', cell: (item) => (item as CustomResourceDefinition).kind },
          { key: 'scope', header: 'Scope', cell: (item) => (item as CustomResourceDefinition).scope },
          { key: 'versions', header: 'Versions', cell: (item) => (item as CustomResourceDefinition).versions.map((version) => `${version.name}${version.storage ? '*' : ''}`).join(', ') },
          { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age((item as CustomResourceDefinition).ageSeconds) },
        ]
      case 'priority-classes':
        return [
          { key: 'name', header: 'Class', cell: (item) => { const value = item as PriorityClass; return <TableLink aria-label={`Open PriorityClass ${value.name}`} onClick={() => workspace.openResource({ collection: tab, name: value.name })} primary={value.name} secondary={value.globalDefault ? 'global default' : ''} /> } },
          { key: 'value', header: 'Priority', cell: (item) => (item as PriorityClass).value },
          { key: 'preemption', header: 'Preemption', cell: (item) => (item as PriorityClass).preemptionPolicy ?? 'unknown' },
          { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age((item as PriorityClass).ageSeconds) },
        ]
      case 'runtime-classes':
        return [
          { key: 'name', header: 'Class', cell: (item) => { const value = item as RuntimeClass; return <TableLink aria-label={`Open RuntimeClass ${value.name}`} onClick={() => workspace.openResource({ collection: tab, name: value.name })} primary={value.name} /> } },
          { key: 'handler', header: 'Handler', cell: (item) => (item as RuntimeClass).handler },
          { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age((item as RuntimeClass).ageSeconds) },
        ]
      case 'mutating-webhook-configurations':
      case 'validating-webhook-configurations':
        return [
          { key: 'name', header: 'Configuration', cell: (item) => { const value = item as WebhookConfiguration; return <TableLink aria-label={`Open webhook configuration ${value.name}`} onClick={() => workspace.openResource({ collection: tab, name: value.name })} primary={value.name} /> } },
          { key: 'count', header: 'Webhooks', cell: (item) => (item as WebhookConfiguration).webhookCount },
          { key: 'age', sortKey: 'ageSeconds', header: 'Age', cell: (item) => age((item as WebhookConfiguration).ageSeconds) },
        ]
    }
  })()

  return (
    <TabbedFamilyPage
      title="Administration" description="Cluster administration objects; CRD discovery never implies access to custom resource instances."
      tab={tab} collection={collection}
      columns={columns} rowKey={(row) => (row as { name: string }).name}
      onApply={(interactionId) => { setApplied(bindListInteraction({ ...draft }, interactionId)) }}
      draft={draft} applied={applied} onDraft={setDraft}
    />
  )
}
