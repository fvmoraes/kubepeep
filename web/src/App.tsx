import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Waypoints } from 'lucide-react'
import { Outlet, Route, Routes, useLocation, useNavigate } from 'react-router'

import { clearRecentTargets, recordPath, recentTargets, subscribeRecentTargets } from './recent/recent'

import { getPreferences, getStatus, type Preferences } from './api/client'
import { Badge } from './components/ui/Badge'
import { CommandCenter, type CommandRoute } from './components/CommandCenter'
import { ContextSelector } from './components/ContextSelector'
import { DashboardPage } from './components/Dashboard'
import { NamespaceScopeEditor } from './components/NamespaceScopeEditor'
import { PermissionsMatrixPage } from './components/PermissionsMatrix'
import { LogsPage } from './components/LogsPage'
import { ConfigPage, EventsPage, NetworkPage, NodesPage, PodsPage, WorkloadsPage } from './components/ResourcePages'
import { LeasesPage, NamespaceObjectPage, StoragePage } from './components/FamilyPages'
import { ConfigurationPage, ServiceAccountsPage } from './components/ConfigurationPages'
import { AccessControlPage, AdministrationPage } from './components/AccessPages'
import { SettingsPage } from './components/SettingsPage'
import { Sidebar } from './components/Sidebar'
import { StatePanel } from './components/StatePanel'
import { useAppVersion } from './hooks/useAppVersion'
import { navGroups, settingsNavItem } from './navigation/tree'

// Command palette catalog: every enabled navigation destination. Group labels
// disambiguate repeated item names (e.g. the Workloads "Overview").
const commandRoutes: CommandRoute[] = [
  ...navGroups.flatMap((group) =>
    group.items
      .filter((item) => Boolean(item.path))
      .map((item) => ({
        path: item.path!,
        label: item.label === 'Overview' && group.id !== 'cluster' ? group.label : item.label,
        description: group.label,
        keywords: [group.label.toLowerCase(), item.label.toLowerCase(), ...(item.keywords ?? [])],
      })),
  ),
  {
    path: settingsNavItem.path!,
    label: settingsNavItem.label,
    description: 'Application',
    keywords: ['application', ...(settingsNavItem.keywords ?? [])],
  },
]

// Global resource index (F7-04): names and namespaces already loaded in this
// session's bounded pages become searchable palette entries. Only identifiers
// are indexed — never resource content, specs, or Secret data.
const maximumCommandResources = 200

function resourceEntryPath(collection: unknown, item: { name?: string; namespace?: string; kind?: string }): string | null {
  if (typeof collection !== 'string' || !item.name) return null
  // Cluster-scoped entries (ADR 0006) resolve by name only; no fake namespace.
  const clusterRoots: Record<string, string> = {
    nodes: '/nodes',
    'persistent-volumes': '/storage/persistent-volumes',
    'storage-classes': '/storage/storage-classes',
    'csi-nodes': '/storage/csi-nodes',
    'csi-drivers': '/storage/csi-drivers',
    'volume-attachments': '/storage/volume-attachments',
    'cluster-roles': '/access/cluster-roles',
    'cluster-role-bindings': '/access/cluster-role-bindings',
    'customresourcedefinitions': '/administration/customresourcedefinitions',
    'priority-classes': '/administration/priority-classes',
    'runtime-classes': '/administration/runtime-classes',
    'mutating-webhook-configurations': '/administration/mutating-webhook-configurations',
    'validating-webhook-configurations': '/administration/validating-webhook-configurations',
    'ingress-classes': '/network/ingress-classes',
  }
  if (collection in clusterRoots) return `${clusterRoots[collection]}/${encodeURIComponent(item.name)}`
  if (!item.namespace) return null
  const namespace = encodeURIComponent(item.namespace)
  const name = encodeURIComponent(item.name)
  switch (collection) {
    case 'pods':
      return `/pods/${namespace}/${name}`
    case 'workloads': {
      const kind = ({ Deployment: 'deployments', StatefulSet: 'statefulsets', DaemonSet: 'daemonsets', Job: 'jobs', CronJob: 'cronjobs', ReplicaSet: 'replicasets' } as Record<string, string>)[item.kind ?? '']
      return kind ? `/workloads/${kind}/${namespace}/${name}` : null
    }
    case 'services':
      return `/network/services/${namespace}/${name}`
    case 'ingresses':
      return `/network/ingresses/${namespace}/${name}`
    case 'endpoint-slices':
      return `/network/endpoint-slices/${namespace}/${name}`
    case 'configmaps':
      return `/config/configmaps/${namespace}/${name}`
    case 'secrets':
      return `/config/secrets/${namespace}/${name}`
    case 'leases':
      return `/leases/${namespace}/${name}`
    case 'persistent-volume-claims':
      return `/storage/persistent-volume-claims/${namespace}/${name}`
    case 'roles':
      return `/access/roles/${namespace}/${name}`
    case 'role-bindings':
      return `/access/role-bindings/${namespace}/${name}`
    case 'network-policies':
      return `/network/network-policies/${namespace}/${name}`
    case 'endpoints':
      return `/network/endpoints/${namespace}/${name}`
    default:
      return null
  }
}

function resourceEntryKeywords(collection: string, item: { kind?: string; namespace?: string }): string[] {
  return [item.kind ?? '', item.namespace ?? '', collection]
}

function favoriteEntryPath(kind: string, namespace: string, name: string): string | null {
  switch (kind) {
    case 'pod':
      return `/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'deployment':
      return `/workloads/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'statefulset':
      return `/workloads/statefulsets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'daemonset':
      return `/workloads/daemonsets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'job':
      return `/workloads/jobs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'cronjob':
      return `/workloads/cronjobs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'service':
      return `/network/services/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'ingress':
      return `/network/ingresses/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'endpointslice':
      return `/network/endpoint-slices/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'configmap':
      return `/config/configmaps/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    case 'secret':
      return `/config/secrets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
    default:
      return null
  }
}

function favoriteEntries(preferences: Preferences | undefined) {
  const entries: CommandRoute[] = []
  for (const item of preferences?.favorites?.items ?? []) {
    const path = favoriteEntryPath(item.kind, item.namespace, item.name)
    if (!path) continue
    entries.push({
      path,
      label: item.name,
      description: `${item.kind} · ${item.namespace}`,
      keywords: [item.kind, item.namespace, 'favorite'],
    })
  }
  return entries
}

function commandResourceEntries(queryClient: ReturnType<typeof useQueryClient>, generation: string | undefined) {
  if (!generation) return []
  const seen = new Set<string>()
  const entries: Array<{ path: string; label: string; description: string; keywords: string[] }> = []
  for (const query of queryClient.getQueryCache().getAll()) {
    const key = query.queryKey
    if (key[0] !== 'resources' || key[2] !== generation) continue
    const collection = typeof key[1] === 'string' ? key[1] : ''
    const data = query.state.data as { items?: Array<{ name?: string; namespace?: string; kind?: string }> } | undefined
    if (!Array.isArray(data?.items)) continue
    for (const item of data.items) {
      const path = resourceEntryPath(collection, item)
      if (!path || seen.has(path)) continue
      seen.add(path)
      entries.push({
        path,
        label: item.name ?? '',
        description: `${item.kind ?? collection} · ${item.namespace ?? 'cluster'}`,
        keywords: resourceEntryKeywords(collection, item),
      })
      if (entries.length >= maximumCommandResources) return entries
    }
  }
  return entries
}
const safeGlobalRefreshRoots = new Set([
  'action-permissions',
  'cluster-profiles',
  'contexts',
  'dashboard',
  'local-status',
  'namespace-scopes',
  'permissions',
  'port-forwards',
  'preferences',
  'resources',
])

function isSafeGlobalRefreshQuery(query: { queryKey: readonly unknown[] }) {
  const root = query.queryKey[0]
  return typeof root === 'string' && safeGlobalRefreshRoots.has(root)
}

function scopeLabel(selection: {
  context: string
  cluster: string
  scopeName: string | null
  scopeMode: string | null
} | null): string {
  if (!selection) return 'No Kubernetes context selected'
  if (selection.scopeName) return `${selection.context} / ${selection.scopeName}`
  if (selection.scopeMode === 'all') return `${selection.context} / All namespaces`
  return `${selection.context} / No namespace scope selected`
}

function StatusBadge() {
  const status = useQuery({
    queryKey: ['local-status'],
    queryFn: ({ signal }) => getStatus(signal),
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })

  if (status.isPending) {
    return <Badge variant="unknown">checking local service</Badge>
  }
  if (status.isError) {
    return <Badge variant="danger">local API unavailable</Badge>
  }
  const local = status.data.components.application.status
  const variant = local === 'healthy' ? 'healthy' : local === 'degraded' ? 'warning' : local === 'unhealthy' ? 'danger' : 'unknown'
  return <Badge variant={variant}>{local}</Badge>
}

function Shell() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const location = useLocation()
  const version = useAppVersion()
  const [compact, setCompact] = useState<boolean>(false)
  const [, setRecentVersion] = useState(0)
  useEffect(() => {
    // V5-12: completed detail navigations become in-memory recents. Secrets
    // and list pages never enter the history.
    recordPath(location.pathname)
  }, [location.pathname])
  useEffect(() => {
    const unsubscribe = subscribeRecentTargets(() => setRecentVersion((value) => value + 1))
    return () => { unsubscribe() }
  }, [])
  const status = useQuery({
    queryKey: ['local-status'],
    queryFn: ({ signal }) => getStatus(signal),
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })
  const selection = status.data?.selection ?? null
  const previousGeneration = useRef<string | null>(null)
  const refreshActiveReads = useCallback(() => queryClient.refetchQueries({ type: 'active', predicate: isSafeGlobalRefreshQuery }), [queryClient])
  const preferences = useQuery({
    queryKey: ['preferences'],
    queryFn: ({ signal }) => getPreferences(signal),
    staleTime: 60_000,
  })

  const toggleCompact = useCallback(() => {
    setCompact((current) => !current)
  }, [])

  useEffect(() => {
    const current = selection?.generation ?? null
    const previous = previousGeneration.current
    if (previous && previous !== current) {
      const belongsToPreviousGeneration = (query: { queryKey: readonly unknown[] }) => query.queryKey.includes(previous)
      void queryClient.cancelQueries({ predicate: belongsToPreviousGeneration })
      queryClient.removeQueries({ predicate: belongsToPreviousGeneration })
    }
    previousGeneration.current = current
  }, [queryClient, selection?.generation])

  return (
    <div className={`app-shell ${compact ? 'app-shell--compact' : ''}`}>
      <a className="skip-link" href="#main-content">Skip to main content</a>
      <Sidebar version={version} compact={compact} onToggleCompact={toggleCompact} />
      <div className="workspace">
        <header className="topbar">
          <div className="topbar-controls">
            <ContextSelector selection={selection} />
            <button
              type="button"
              onClick={() => navigate('/namespaces')}
              data-tip={selection ? `${selection.cluster} · ${selection.namespaceCount} namespace${selection.namespaceCount === 1 ? '' : 's'} in scope` : 'Select a namespace scope to browse resources'}
              className="flex h-8 min-w-0 max-w-[15rem] items-center gap-2 rounded-full border border-kp-overlay-0 bg-kp-surface-0 px-3 text-sm text-kp-subtext hover:border-kp-accent-border hover:text-kp-text"
            >
              <Waypoints size={14} strokeWidth={1.8} className="shrink-0 text-kp-mauve" aria-hidden="true" />
              <span className="truncate">{scopeLabel(selection)}</span>
            </button>
          </div>
          <div className="topbar-controls">
            <StatusBadge />
            <CommandCenter routes={commandRoutes} getFavorites={() => favoriteEntries(preferences.data)} getRecent={() => recentTargets().map((entry) => ({
              path: entry.path,
              label: entry.name,
              description: `recent · ${entry.kind}${entry.namespace ? ` · ${entry.namespace}` : ''}`,
              keywords: [entry.kind, entry.namespace ?? '', 'recent'],
            }))} onClearRecent={() => clearRecentTargets()} getResources={() => commandResourceEntries(queryClient, selection?.generation)} onRefresh={refreshActiveReads} />
          </div>
        </header>
        <main id="main-content"><Outlet /></main>
      </div>
    </div>
  )
}

export function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<DashboardPage />} />
        <Route path="events" element={<EventsPage />} />
        <Route path="nodes" element={<NodesPage />} />
        <Route path="nodes/:name" element={<NodesPage />} />
        <Route path="leases" element={<LeasesPage />} />
        <Route path="leases/:namespace/:name" element={<LeasesPage />} />
        <Route path="namespaces" element={<NamespaceScopeEditor />} />
        <Route path="namespaces/:name" element={<NamespaceObjectPage />} />
        <Route path="storage" element={<StoragePage />} />
        <Route path="storage/:tab" element={<StoragePage />} />
        <Route path="storage/:tab/:namespace/:name" element={<StoragePage />} />
        <Route path="configuration" element={<ConfigurationPage />} />
        <Route path="configuration/:tab" element={<ConfigurationPage />} />
        <Route path="configuration/:tab/:namespace/:name" element={<ConfigurationPage />} />
        <Route path="service-accounts" element={<ServiceAccountsPage />} />
        <Route path="access/:tab" element={<AccessControlPage />} />
        <Route path="access/:tab/:namespace/:name" element={<AccessControlPage />} />
        <Route path="administration" element={<AdministrationPage />} />
        <Route path="administration/:tab" element={<AdministrationPage />} />
        <Route path="administration/:tab/:name" element={<AdministrationPage />} />
        <Route path="permissions" element={<PermissionsMatrixPage />} />
        <Route path="logs" element={<LogsPage />} />
        <Route path="pods" element={<PodsPage />} />
        <Route path="pods/:namespace/:name" element={<PodsPage />} />
        <Route path="workloads" element={<WorkloadsPage />} />
        <Route path="workloads/kind/:kind" element={<WorkloadsPage />} />
        <Route path="workloads/:kind/:namespace/:name" element={<WorkloadsPage />} />
        <Route path="network" element={<NetworkPage />} />
        <Route path="network/:tab" element={<NetworkPage />} />
        <Route path="network/:tab/:namespace/:name" element={<NetworkPage />} />
        <Route path="config" element={<ConfigPage />} />
        <Route path="config/:tab" element={<ConfigPage />} />
        <Route path="config/:tab/:namespace/:name" element={<ConfigPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<StatePanel kind="error" title="Page not found">Return to Overview using the navigation.</StatePanel>} />
      </Route>
    </Routes>
  )
}
