import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef } from 'react'
import {
  Activity,
  Boxes,
  Braces,
  CircleGauge,
  FileText,
  KeyRound,
  Network,
  ScrollText,
  Settings,
  TerminalSquare,
} from 'lucide-react'
import { NavLink, Outlet, Route, Routes } from 'react-router'

import { getStatus } from './api/client'
import { BrandLogo } from './components/BrandLogo'
import { BrandWordmark } from './components/BrandWordmark'
import { Badge } from './components/ui/Badge'
import { CommandCenter } from './components/CommandCenter'
import { ContextSelector } from './components/ContextSelector'
import { DashboardPage } from './components/Dashboard'
import { NamespaceScopeEditor } from './components/NamespaceScopeEditor'
import { PermissionsMatrixPage } from './components/PermissionsMatrix'
import { LogsPage } from './components/LogsPage'
import { ConfigPage, EventsPage, NetworkPage, PodsPage, WorkloadsPage } from './components/ResourcePages'
import { SettingsPage } from './components/SettingsPage'
import { StatePanel } from './components/StatePanel'

const applicationDestinations = [
  { path: '/', label: 'Overview', description: 'Cluster health and operational summary', keywords: ['dashboard', 'health'], icon: CircleGauge, page: DashboardPage },
  { path: '/workloads', label: 'Workloads', description: 'Deployments, StatefulSets and DaemonSets', keywords: ['deployment', 'statefulset', 'daemonset'], icon: Boxes, page: WorkloadsPage },
  { path: '/pods', label: 'Pods', description: 'Pod inventory, status and containers', keywords: ['container', 'restart'], icon: Activity, page: PodsPage },
  { path: '/logs', label: 'Logs', description: 'Bounded container log viewer', keywords: ['tail', 'stream'], icon: ScrollText, page: LogsPage },
  { path: '/events', label: 'Events', description: 'Kubernetes event timeline', keywords: ['warning', 'reason'], icon: FileText, page: EventsPage },
  { path: '/network', label: 'Network', description: 'Services, ingresses and endpoints', keywords: ['service', 'ingress', 'endpointslice'], icon: Network, page: NetworkPage },
  { path: '/config', label: 'Config', description: 'Safe configuration resource views', keywords: ['configmap', 'yaml'], icon: Braces, page: ConfigPage },
  { path: '/namespaces', label: 'Namespaces', description: 'Namespace scopes and selection', keywords: ['scope'], icon: TerminalSquare, page: NamespaceScopeEditor },
  { path: '/permissions', label: 'Permissions', description: 'Effective Kubernetes capabilities', keywords: ['rbac', 'authorization'], icon: KeyRound, page: PermissionsMatrixPage },
  { path: '/settings', label: 'Settings', description: 'Allowlisted local preferences', keywords: ['preferences'], icon: Settings, page: SettingsPage },
] as const

const commandRoutes = applicationDestinations.map(({ path, label, description, keywords }) => ({ path, label, description, keywords }))
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
  const status = useQuery({
    queryKey: ['local-status'],
    queryFn: ({ signal }) => getStatus(signal),
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })
  const selection = status.data?.selection ?? null
  const previousGeneration = useRef<string | null>(null)
  const refreshActiveReads = useCallback(() => queryClient.refetchQueries({ type: 'active', predicate: isSafeGlobalRefreshQuery }), [queryClient])

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
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to main content</a>
      <aside className="sidebar">
        <div className="brand" aria-label="kubePeep home">
          <BrandLogo size={34} />
          <div>
            <BrandWordmark height={18} />
            <small>local cluster view</small>
          </div>
        </div>
        <nav aria-label="Primary navigation">
          {applicationDestinations.map(({ path, label, icon: Icon }) => (
            <NavLink key={path} to={path} end={path === '/'}>
              <Icon size={17} strokeWidth={1.8} aria-hidden="true" />
              <span>{label}</span>
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <div className="active-context">
            <span className="eyebrow">context</span>
            <strong>{selection ? `${selection.context} · ${selection.scopeName ?? 'no namespace scope'}` : 'No Kubernetes context selected'}</strong>
            {selection ? <small>{selection.cluster} · {selection.namespaceCount} namespace{selection.namespaceCount === 1 ? '' : 's'}</small> : null}
          </div>
          <div className="topbar-controls">
            <CommandCenter routes={commandRoutes} onRefresh={refreshActiveReads} />
            <ContextSelector selection={selection} />
            <StatusBadge />
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
        {applicationDestinations.map(({ path, page: Page }) => path === '/'
          ? <Route key={path} index element={<Page />} />
          : <Route key={path} path={path.slice(1)} element={<Page />} />)}
        <Route path="workloads/:kind/:namespace/:name" element={<WorkloadsPage />} />
        <Route path="pods/:namespace/:name" element={<PodsPage />} />
        <Route path="network/:tab/:namespace/:name" element={<NetworkPage />} />
        <Route path="config/:tab/:namespace/:name" element={<ConfigPage />} />
        <Route path="*" element={<StatePanel kind="error" title="Page not found">Return to Overview using the navigation.</StatePanel>} />
      </Route>
    </Routes>
  )
}
