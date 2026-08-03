import { useQuery } from '@tanstack/react-query'
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

import { APIError, getStatus } from './api/client'
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
  return (
    <div className="app-shell">
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
          <div>
            <span className="eyebrow">context</span>
            <strong>No Kubernetes context selected</strong>
          </div>
          <StatusBadge />
        </header>
        <main id="main-content"><Outlet /></main>
      </div>
    </div>
  )
}

function Overview() {
  const status = useQuery({
    queryKey: ['local-status'],
    queryFn: ({ signal }) => getStatus(signal),
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })

  if (status.isPending) {
    return <StatePanel kind="loading" title="Preparing the local workspace">Checking application and SQLite health.</StatePanel>
  }
  if (status.isError) {
	  if (status.error instanceof APIError) {
	    return <StatePanel kind="error" title="The local API returned an error">The response was rejected safely. Retry after checking the local status or running kubePeep doctor.</StatePanel>
	  }
    return <StatePanel kind="offline" title="The local API is unavailable">Reload after the kubePeep process is ready.</StatePanel>
  }
  if (!status.data.selection) {
    return <StatePanel kind="empty" title="Choose a Kubernetes context">The local application is ready. Context and namespace setup arrives in the next implementation phase.</StatePanel>
  }
  return <StatePanel kind="unavailable" title="Cluster data is not enabled yet">The foundation is healthy; Kubernetes queries are introduced in Phase 4.</StatePanel>
}

function Placeholder({ title }: { title: string }) {
  return <StatePanel kind="empty" title={title}>This area is intentionally empty until its backend capability is implemented.</StatePanel>
}

export function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<Overview />} />
        {navigation.slice(1).map(({ path, label }) => (
          <Route key={path} path={path.slice(1)} element={<Placeholder title={label} />} />
        ))}
        <Route path="*" element={<StatePanel kind="error" title="Page not found">Return to Overview using the navigation.</StatePanel>} />
      </Route>
    </Routes>
  )
}
