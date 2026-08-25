import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
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
import { ContextSelector } from './components/ContextSelector'
import { DashboardPage } from './components/Dashboard'
import { NamespaceScopeEditor } from './components/NamespaceScopeEditor'
import { PermissionsMatrixPage } from './components/PermissionsMatrix'
import { LogsPage } from './components/LogsPage'
import { ConfigPage, EventsPage, NetworkPage, PodsPage, WorkloadsPage } from './components/ResourcePages'
import { SettingsPage } from './components/SettingsPage'
import { StatePanel } from './components/StatePanel'

const navigation = [
  { path: '/', label: 'Overview', icon: CircleGauge },
  { path: '/workloads', label: 'Workloads', icon: Boxes },
  { path: '/pods', label: 'Pods', icon: Activity },
  { path: '/logs', label: 'Logs', icon: ScrollText },
  { path: '/events', label: 'Events', icon: FileText },
  { path: '/network', label: 'Network', icon: Network },
  { path: '/config', label: 'Config', icon: Braces },
  { path: '/namespaces', label: 'Namespaces', icon: TerminalSquare },
  { path: '/permissions', label: 'Permissions', icon: KeyRound },
  { path: '/settings', label: 'Settings', icon: Settings },
] as const

function StatusBadge() {
  const status = useQuery({
    queryKey: ['local-status'],
    queryFn: ({ signal }) => getStatus(signal),
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })

  if (status.isPending) {
    return <span className="status-badge status-badge--unknown">checking local service</span>
  }
  if (status.isError) {
    return <span className="status-badge status-badge--unhealthy">local API unavailable</span>
  }
  const local = status.data.components.application.status
  return <span className={`status-badge status-badge--${local}`}>{local}</span>
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
          <span className="brand-mark" aria-hidden="true">kp</span>
          <div><strong>kubePeep</strong><small>local cluster view</small></div>
        </div>
        <nav aria-label="Primary navigation">
          {navigation.map(({ path, label, icon: Icon }) => (
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
        <Route index element={<DashboardPage />} />
        <Route path="workloads" element={<WorkloadsPage />} />
        <Route path="pods" element={<PodsPage />} />
        <Route path="logs" element={<LogsPage />} />
        <Route path="events" element={<EventsPage />} />
        <Route path="network" element={<NetworkPage />} />
        <Route path="config" element={<ConfigPage />} />
        <Route path="namespaces" element={<NamespaceScopeEditor />} />
        <Route path="permissions" element={<PermissionsMatrixPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<StatePanel kind="error" title="Page not found">Return to Overview using the navigation.</StatePanel>} />
      </Route>
    </Routes>
  )
}
