import { ResourceListFocus } from './components/resource/ResourceListFocus'
import { WarmInventoryCache } from './components/resource/WarmInventoryCache'
import { AutoRefreshProvider, AutoRefreshToggle } from './components/resource/AutoRefreshProvider'
import { ResourceFamilyNav } from './components/resource/ResourceFamilyNav'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { Waypoints } from 'lucide-react'
import { Outlet, Route, Routes, useLocation, useNavigate } from 'react-router'

import { recordPath, recentTargets } from './recent/recent'

import { getPreferences, getStatus, type Preferences } from './api/client'
import { mutatePreferences } from './api/preferences'
import { Badge } from './components/ui/Badge'
import { CommandCenter, type CommandRoute } from './components/CommandCenter'
import { contextColor } from './components/ContextColorPicker'
import type { CSSProperties } from 'react'
import { ContextSelector } from './components/ContextSelector'
import { DefaultScopeGate } from './components/DefaultScopeGate'
import { GlobalNamespaceSelect } from './components/GlobalNamespaceSelect'
import { Sidebar } from './components/Sidebar'
import { MobileNavigation } from './components/MobileNavigation'
import { LoadingState } from './components/ui/LoadingState'
import { useMediaQuery } from './hooks/useMediaQuery'
import { ResourceSplitView } from './components/workspace/ResourceSplitView'
import { StatePanel } from './components/StatePanel'
import { ResourceWorkspaceProvider, useResourceWorkspace } from './components/workspace/ResourceWorkspaceProvider'
import { GlobalNamespaceProvider, useGlobalNamespace } from './context/GlobalNamespace'
import { ToastProvider } from './components/ui/Toast'
import { useAppVersion } from './hooks/useAppVersion'
import { navGroups, settingsNavItem } from './navigation/tree'
import { resourceDetailPath } from './navigation/paths'
import { desktopPlatform } from './api/desktop'
import { recordShellReady } from './observability/uxMetrics'

// Load the shell first. Sections render inside its independent fallback.
const DashboardPage = lazy(() => import('./components/Dashboard').then((module) => ({ default: module.DashboardPage })))
const NamespaceScopeEditor = lazy(() => import('./components/NamespaceScopeEditor').then((module) => ({ default: module.NamespaceScopeEditor })))
const PermissionsMatrixPage = lazy(() => import('./components/PermissionsMatrix').then((module) => ({ default: module.PermissionsMatrixPage })))
const LogsPage = lazy(() => import('./components/LogsPage').then((module) => ({ default: module.LogsPage })))
const ConfigPage = lazy(() => import('./components/ResourcePages').then((module) => ({ default: module.ConfigPage })))
const EventsPage = lazy(() => import('./components/ResourcePages').then((module) => ({ default: module.EventsPage })))
const NetworkPage = lazy(() => import('./components/ResourcePages').then((module) => ({ default: module.NetworkPage })))
const NodesPage = lazy(() => import('./components/ResourcePages').then((module) => ({ default: module.NodesPage })))
const PodsPage = lazy(() => import('./components/ResourcePages').then((module) => ({ default: module.PodsPage })))
const WorkloadsPage = lazy(() => import('./components/ResourcePages').then((module) => ({ default: module.WorkloadsPage })))
const LeasesPage = lazy(() => import('./components/FamilyPages').then((module) => ({ default: module.LeasesPage })))
const NamespaceObjectPage = lazy(() => import('./components/FamilyPages').then((module) => ({ default: module.NamespaceObjectPage })))
const StoragePage = lazy(() => import('./components/FamilyPages').then((module) => ({ default: module.StoragePage })))
const ConfigurationPage = lazy(() => import('./components/ConfigurationPages').then((module) => ({ default: module.ConfigurationPage })))
const GatewayPage = lazy(() => import('./components/GatewayPage').then((module) => ({ default: module.GatewayPage })))
const DynamicResourcePage = lazy(() => import('./components/DynamicResourcePage').then((module) => ({ default: module.DynamicResourcePage })))
const HelmPage = lazy(() => import('./components/HelmPage').then((module) => ({ default: module.HelmPage })))
const ServiceAccountsPage = lazy(() => import('./components/ConfigurationPages').then((module) => ({ default: module.ServiceAccountsPage })))
const AccessControlPage = lazy(() => import('./components/AccessPages').then((module) => ({ default: module.AccessControlPage })))
const AdministrationPage = lazy(() => import('./components/AccessPages').then((module) => ({ default: module.AdministrationPage })))
const SettingsPage = lazy(() => import('./components/SettingsPage').then((module) => ({ default: module.SettingsPage })))
const ResourceWorkspacePanel = lazy(() => import('./components/workspace/ResourceWorkspace').then((module) => ({ default: module.ResourceWorkspacePanel })))

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
  return resourceDetailPath({ collection, kind: item.kind ?? null, namespace: item.namespace ?? null, name: item.name })
}

function resourceEntryKeywords(collection: string, item: { kind?: string; namespace?: string }): string[] {
  return [item.kind ?? '', item.namespace ?? '', collection]
}

function favoriteEntryPath(kind: string, namespace: string | undefined, name: string): string | null {
  // Cluster-scoped favorites (V6-03) resolve by name only.
  if (kind === 'deployment' || kind === 'statefulset' || kind === 'daemonset' || kind === 'job' || kind === 'cronjob') {
    const titleKind = { deployment: 'Deployment', statefulset: 'StatefulSet', daemonset: 'DaemonSet', job: 'Job', cronjob: 'CronJob' }[kind]
    return resourceDetailPath({ collection: 'workloads', kind: titleKind, namespace: namespace ?? null, name })
  }
  const favoriteCollections: Record<string, string> = {
    pod: 'pods',
    service: 'services',
    ingress: 'ingresses',
    endpointslice: 'endpoint-slices',
    configmap: 'configmaps',
    secret: 'secrets',
    node: 'nodes',
    persistentvolume: 'persistent-volumes',
    storageclass: 'storage-classes',
    ingressclass: 'ingress-classes',
    priorityclass: 'priority-classes',
    runtimeclass: 'runtime-classes',
    customresourcedefinition: 'customresourcedefinitions',
  }
  const collection = favoriteCollections[kind]
  if (!collection) return null
  return resourceDetailPath({ collection, namespace: namespace ?? null, name })
}

function favoriteEntries(preferences: Preferences | undefined) {
  const entries: CommandRoute[] = []
  for (const item of preferences?.favorites?.items ?? []) {
    const path = favoriteEntryPath(item.kind, item.namespace, item.name)
    if (!path) continue
    entries.push({
      path,
      label: item.name,
      description: `${item.kind} · ${item.namespace ?? 'cluster'}`,
      keywords: [item.kind, item.namespace ?? '', 'favorite'],
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
    const infiniteCollection = key[5] === generation
    if (key[0] !== 'resources' || (key[2] !== generation && !infiniteCollection) || query.state.error) continue
    const collection = typeof key[1] === 'string' ? key[1] : ''
    type CachedPage = { items?: Array<{ name?: string; namespace?: string; kind?: string }>; snapshotRenewed?: boolean }
    const data = query.state.data as (CachedPage & { pages?: CachedPage[] }) | undefined
    const pages = infiniteCollection && Array.isArray(data?.pages) ? data.pages : data ? [data] : []
    let firstCurrentPage = 0
    for (let index = 0; index < pages.length; index += 1) {
      if (pages[index].snapshotRenewed) firstCurrentPage = index
    }
    for (const page of pages.slice(firstCurrentPage)) {
      if (!Array.isArray(page.items)) continue
      for (const item of page.items) {
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
  'workspace-detail',
  'workspace-events',
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
    // Desktop bootstrap may still be resolving kubeconfig after the shell
    // appears. Poll only that short unknown state, never an idle no-selection.
    refetchInterval: (query) => {
      const data = query.state.data
      return data && !data.selection && data.components.kubeconfig.status === 'unknown' ? 500 : false
    },
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

// persistShellPrefs delegates every shell/recent update to the shared
// preferences coordinator. The mutator receives a fresh backend document, so
// concurrent filters, favorites, columns and future sections are preserved.
function useShellPreferencePersistence(preferencesAvailable: boolean, onSaveError: () => void) {
  const queryClient = useQueryClient()
  return useCallback(async (change: (current: Preferences) => Preferences) => {
    if (!preferencesAvailable) return
    try {
      const saved = await mutatePreferences(change)
      queryClient.setQueryData(['preferences'], saved)
    } catch {
      onSaveError()
    }
  }, [onSaveError, preferencesAvailable, queryClient])
}

function Shell() {
  const mobile = useMediaQuery('(max-width: 760px), (max-width: 1024px) and (max-height: 500px)')
  const queryClient = useQueryClient()
  useEffect(() => {
    recordShellReady()
    void import('./components/ResourcePages')
  }, [])
  const navigate = useNavigate()
  const version = useAppVersion()
  const [compact, setCompact] = useState<boolean>(false)
  const [collapsedGroups, setCollapsedGroups] = useState<string[]>(() => navGroups.map((group) => group.id))
  const location = useLocation()
  const status = useQuery({
    queryKey: ['local-status'],
    queryFn: ({ signal }) => getStatus(signal),
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })
  const selection = status.data?.selection ?? null
  const selectionPendingForRoute = status.isPending
    && location.pathname !== '/'
    && location.pathname !== '/namespaces'
    && commandRoutes.some((route) => route.path === location.pathname)
  const globalNamespace = useGlobalNamespace()
  const prefetchedPodsFor = useRef('')
  useEffect(() => {
    if (!selection || selection.namespaceCount < 1 || selection.namespaceCount > 50 || globalNamespace.loading || globalNamespace.degraded || globalNamespace.options.length === 0 || location.pathname !== '/') return
    const previewNamespace = globalNamespace.value || globalNamespace.options[0]
    const key = `${selection.generation}:\0${globalNamespace.value}:\0${previewNamespace}`
    if (prefetchedPodsFor.current === key) return
    prefetchedPodsFor.current = key
    void import('./components/resource/podPreview').then(({ prefetchDefaultPodPreview }) => {
      if (prefetchedPodsFor.current === key) return prefetchDefaultPodPreview(queryClient, selection, globalNamespace.value, previewNamespace)
    })
  }, [queryClient, selection, globalNamespace.value, globalNamespace.options, globalNamespace.loading, globalNamespace.degraded, location.pathname])
  const previousGeneration = useRef<string | null>(null)
  const refreshActiveReads = useCallback(() => queryClient.refetchQueries({ type: 'active', predicate: isSafeGlobalRefreshQuery }), [queryClient])
  const preferences = useQuery({
    queryKey: ['preferences'],
    queryFn: ({ signal }) => getPreferences(signal),
    staleTime: 60_000,
  })
  const preferencesData = preferences.data
  const [, setHydrationError] = useState(false)
  const workspace = useResourceWorkspace()
  const inspectedPath = workspace.open && workspace.active ? resourceDetailPath(workspace.active) : null

  // Hydration (V6-05): initial state comes from the backend document; local
  // state only diverges after an explicit user action and is persisted by
  // merging into the current document. Sidebar groups default to CLOSED — the
  // active group auto-expands so the current section is always visible.
  const hydratedRef = useRef(false)
  useEffect(() => {
    if (!preferencesData || hydratedRef.current) return
    hydratedRef.current = true
    setCompact(preferencesData.shell?.sidebarCompact ?? false)
    // An empty stored list is the pre-F6 default, not a real choice: groups
    // start closed unless the user actually expanded them.
    const stored = preferencesData.shell?.collapsedGroups ?? []
    setCollapsedGroups(stored.length > 0 ? stored : navGroups.map((group) => group.id))
  }, [preferencesData])

  // The desktop app always opens on Overview; browser deep links keep their
  // standard URL semantics.
  const landingRef = useRef(false)
  useEffect(() => {
    if (landingRef.current) return
    landingRef.current = true
    void desktopPlatform().then((info) => {
      if (info && location.pathname !== '/') navigate('/', { replace: true })
    })
  }, [location.pathname, navigate])

  const persistShellPrefs = useShellPreferencePersistence(Boolean(preferencesData), () => setHydrationError(true))

  const persistRecent = useCallback(() => {
    void persistShellPrefs((currentPrefs) => {
      currentPrefs.recent = {
        version: 1,
        items: recentTargets().map((entry) => ({
          kind: entry.kind,
          namespace: entry.namespace ?? undefined,
          name: entry.name,
          recordedAt: new Date(entry.recordedAt).toISOString(),
        })),
      }
      return currentPrefs
    })
  }, [persistShellPrefs])
  const toggleCompact = useCallback(() => {
    setCompact((current) => {
      const next = !current
      void persistShellPrefs((currentPrefs) => {
        currentPrefs.shell = { ...(currentPrefs.shell ?? { sidebarCompact: false, collapsedGroups: [] }), sidebarCompact: next }
        return currentPrefs
      })
      return next
    })
  }, [persistShellPrefs])

  useEffect(() => {
    // V5-12/V6-04: completed detail navigations become in-memory recents.
    // Secrets and list pages never enter the history; a change is persisted
    // by merging into the current preferences document.
    if (recordPath(inspectedPath ?? location.pathname)) {
      persistRecent()
    }
  }, [inspectedPath, location.pathname, persistRecent])

  const toggleGroup = useCallback((id: string) => {
    setCollapsedGroups((current) => {
      const next = current.includes(id) ? current.filter((value) => value !== id) : [...current, id]
      void persistShellPrefs((currentPrefs) => {
        currentPrefs.shell = { ...(currentPrefs.shell ?? { sidebarCompact: false, collapsedGroups: [] }), collapsedGroups: next }
        return currentPrefs
      })
      return next
    })
  }, [persistShellPrefs])

  useEffect(() => {
    const current = selection?.generation ?? null
    const previous = previousGeneration.current
    if (previous && previous !== current) {
      const belongsToPreviousGeneration = (query: { queryKey: readonly unknown[] }) => query.queryKey.includes(previous)
      void queryClient.cancelQueries({ predicate: belongsToPreviousGeneration })
      queryClient.removeQueries({ predicate: belongsToPreviousGeneration })
      // The session nonce rotates with the generation: drop the cached token
      // so every consumer refetches a fresh one instead of CSRF_REJECTED.
      queryClient.removeQueries({ queryKey: ['session'] })
      // Workspace history points at resources of the previous selection.
      workspace.reset()
    }
    previousGeneration.current = current
  }, [queryClient, selection?.generation, workspace])

  return (
    <div className={`app-shell ${compact ? 'app-shell--compact' : ''}`} data-context-color={contextColor(preferencesData, selection)} style={{ '--context-color': contextColor(preferencesData, selection) } as CSSProperties}>
      <a className="skip-link" href="#main-content">Skip to main content</a>
      {!mobile ? <Sidebar version={version} compact={compact} onToggleCompact={toggleCompact} collapsedGroups={collapsedGroups} onToggleGroup={toggleGroup} /> : null}
      <div className="workspace">
        <header className="topbar">
          {mobile ? <div className="mobile-navigation-trigger"><MobileNavigation version={version} compact={false} onToggleCompact={toggleCompact} collapsedGroups={collapsedGroups} onToggleGroup={toggleGroup} /></div> : null}
          <div className="topbar-controls topbar-selection">
            <ContextSelector selection={selection} />
            <GlobalNamespaceSelect />
            <button
              type="button"
              onClick={() => navigate('/namespaces')}
              title={selection ? `${selection.cluster} · ${selection.namespaceCount} namespace${selection.namespaceCount === 1 ? '' : 's'} in scope` : 'Select a namespace scope to browse resources'}
              className="control flex min-w-0 max-w-[15rem] items-center gap-1.5 border border-kp-overlay-0 bg-kp-surface-0 px-2 text-menu text-kp-subtext hover:border-kp-accent-border hover:text-kp-text"
            >
              <Waypoints size={14} strokeWidth={1.8} className="shrink-0 text-kp-mauve" aria-hidden="true" />
              <span className="truncate">{scopeLabel(selection)}</span>
            </button>
          </div>
            <div className="topbar-controls topbar-actions">
              <AutoRefreshToggle />
            <StatusBadge />
            <CommandCenter routes={commandRoutes} getFavorites={() => favoriteEntries(preferences.data)} getResources={() => commandResourceEntries(queryClient, selection?.generation)} onRefresh={refreshActiveReads} />
          </div>
        </header>
        <main id="main-content">
          <ResourceFamilyNav />
          <ResourceSplitView detail={workspace.open ? <Suspense fallback={<LoadingState label="Opening resource…" />}><ResourceWorkspacePanel /></Suspense> : null}>
            <DefaultScopeGate selection={selection} selectionPending={selectionPendingForRoute}>
              {selection?.scopeMode && !globalNamespace.ready && location.pathname !== '/namespaces' && location.pathname !== '/settings'
                ? <StatePanel kind={globalNamespace.loading ? 'loading' : 'error'} title={globalNamespace.loading ? 'Loading the default namespace' : 'Default namespace unavailable'}>Open Namespace Scopes to review the namespaces available for this context.</StatePanel>
                : <Suspense fallback={<LoadingState label="Opening section…" layout="table" />}><ResourceListFocus.Provider value={new URLSearchParams(location.search).get('focus') ? { name: new URLSearchParams(location.search).get('focus')!, clear: () => {
                  if (workspace.open) workspace.clearListFocus()
                  else { const params = new URLSearchParams(location.search); params.delete('focus'); navigate({ pathname: location.pathname, search: params.toString() }, { replace: true }) }
                } } : null}><Outlet key={`${location.pathname}|${new URLSearchParams(location.search).get('focus') ?? ''}`} /></ResourceListFocus.Provider></Suspense>}
            </DefaultScopeGate>
          </ResourceSplitView>
        </main>
      </div>
    </div>
  )
}

function ShellProviders() {
  const location = useLocation()
  const status = useQuery({
    queryKey: ['local-status'],
    queryFn: ({ signal }) => getStatus(signal),
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })
  const selection = status.data?.selection ?? null
  return (
    <GlobalNamespaceProvider generation={selection?.generation} scopeId={selection?.scopeId ?? null} scopeMode={selection?.scopeMode ?? null} defaultNamespace={selection?.defaultNamespace} requestedNamespace={new URLSearchParams(location.search).get('namespace')}>
      <Shell />
    </GlobalNamespaceProvider>
  )
}

function WorkspaceRoutes() {
  const location = useLocation()
  const workspace = useResourceWorkspace()
  return (
      <Routes location={workspace.backgroundLocation ?? location}>
        <Route element={<ShellProviders />}>
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
          <Route path="storage/:tab/:name" element={<StoragePage />} />
          <Route path="storage/:tab/:namespace/:name" element={<StoragePage />} />
          <Route path="configuration" element={<ConfigurationPage />} />
          <Route path="network/gateway-api" element={<GatewayPage />} />
          <Route path="helm/releases" element={<HelmPage />} />
          <Route path="helm/releases/:driver" element={<HelmPage />} />
          <Route path="helm/releases/:driver/:namespace/:name" element={<HelmPage />} />
          <Route path="network/gateway-api/:tab" element={<GatewayPage />} />
          <Route path="network/gateway-api/:tab/:name" element={<GatewayPage />} />
          <Route path="network/gateway-api/:tab/:namespace/:name" element={<GatewayPage />} />
          <Route path="configuration/:tab" element={<ConfigurationPage />} />
          <Route path="configuration/:tab/:namespace/:name" element={<ConfigurationPage />} />
          <Route path="service-accounts" element={<ServiceAccountsPage />} />
          <Route path="service-accounts/:namespace/:name" element={<ServiceAccountsPage />} />
          <Route path="access/:tab" element={<AccessControlPage />} />
          <Route path="access/:tab/:name" element={<AccessControlPage />} />
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
          <Route path="workloads/custom" element={<DynamicResourcePage />} />
          <Route path="workloads/custom/:group/:version/:resource/:scope" element={<DynamicResourcePage />} />
          <Route path="workloads/custom/:group/:version/:resource/:scope/:namespace/:name" element={<DynamicResourcePage />} />
          <Route path="workloads/:kind/:namespace/:name" element={<WorkloadsPage />} />
          <Route path="network" element={<NetworkPage />} />
          <Route path="network/:tab" element={<NetworkPage />} />
          <Route path="network/:tab/:name" element={<NetworkPage />} />
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

export function App() {
  return <ToastProvider><AutoRefreshProvider><WarmInventoryCache><ResourceWorkspaceProvider><WorkspaceRoutes /></ResourceWorkspaceProvider></WarmInventoryCache></AutoRefreshProvider></ToastProvider>
}
