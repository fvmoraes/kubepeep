import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { StoragePage } from './FamilyPages'
import { ConfigPage, EventsPage, NetworkPage, PodsPage, WorkloadsPage } from './ResourcePages'
import { prefetchDefaultPodPreview } from './resource/podPreview'
import type { SelectionSummary } from '../api/types'
import { ToastProvider } from './ui/Toast'
import { ResourceWorkspaceProvider } from './workspace/ResourceWorkspaceProvider'
import { ResourceWorkspaceOverlay, tabsFor } from './workspace/ResourceWorkspace'
import { GlobalNamespaceProvider, useGlobalNamespace } from '../context/GlobalNamespace'

const generation = 'gen_42'

function json(data: unknown, meta: Record<string, unknown> = {}, responseGeneration = generation): Response {
  return new Response(JSON.stringify({ data, meta: { generation: responseGeneration, ...meta } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function page(next = '') {
  return { page: { limit: 100, next, complete: next === '', truncated: false, filterScope: 'page' }, coverage: null }
}

function selectedStatus(selectedGeneration = generation) {
  return {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: 'healthy', code: 'TEST', message: 'test', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'dev-cluster', scopeId: 7, scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 1, generation: selectedGeneration },
  }
}

function preferences() {
  const empty = { version: 1, items: [] }
  return { version: 1, ui: { language: 'en' }, logs: { wrap: false, timestamps: true, tailLines: 200 }, dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] }, filters: { workloads: empty, pods: empty, events: empty, logs: empty } }
}

function NamespaceTestControl() {
  const namespace = useGlobalNamespace()
  return <button onClick={() => namespace.setValue(namespace.value ? '' : 'payments')}>Change global namespace</button>
}

function LocationProbe() {
  return <output aria-label="Current route">{useLocation().pathname}</output>
}

function renderPage(component: React.ReactNode, initialEntries: string[] = ['/'], client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })) {
  const selection = selectedStatus()
  return { client, ...render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={initialEntries}>
        <ToastProvider>
          <ResourceWorkspaceProvider>
            <GlobalNamespaceProvider generation={selection.selection.generation} scopeId={selection.selection.scopeId} scopeMode={selection.selection.scopeMode}>
              <NamespaceTestControl />
              {component}
              <ResourceWorkspaceOverlay />
            </GlobalNamespaceProvider>
          </ResourceWorkspaceProvider>
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  ) }
}

function sortOptionValues(): string[] {
  return within(screen.getByLabelText('Sort this bounded page')).getAllByRole('option').map((option) => (option as HTMLOptionElement).value)
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('read-only resource pages', () => {

  it('publishes the complete workspace tab catalog for each actionable kind', () => {
    const labels = (collection: string, kind: string | null = null) => tabsFor({ collection, kind, namespace: 'payments', name: 'api', tab: 'overview' }).map((tab) => tab.label)
    expect(labels('pods', 'Pod')).toEqual(['Overview', 'Investigation', 'Logs', 'YAML', 'Events', 'Metrics', 'Containers', 'Actions'])
    expect(labels('workloads', 'Deployment')).toEqual(['Overview', 'Investigation', 'Pods', 'ReplicaSets', 'YAML', 'Events', 'Rollout', 'Actions'])
    expect(labels('workloads', 'StatefulSet')).toEqual(['Overview', 'Investigation', 'Pods', 'PVCs', 'YAML', 'Events', 'Actions'])
    expect(labels('workloads', 'CronJob')).toEqual(['Overview', 'Investigation', 'Jobs', 'YAML', 'Events', 'Actions'])
    expect(labels('services', 'Service')).toEqual(['Overview', 'Endpoints', 'YAML', 'Events', 'Actions'])
    expect(labels('ingresses', 'Ingress')).toEqual(['Overview', 'Rules', 'Backends', 'YAML', 'Events', 'Actions'])
  })

  it('starts a Service port-forward only after all prerequisite permissions are allowed', async () => {
    let portForwardInit: RequestInit | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path.startsWith('/api/v1/services?')) return Promise.resolve(json([{ namespace: 'payments', name: 'api', type: 'ClusterIP', clusterIPs: ['10.96.0.10'], ports: [{ name: 'http', protocol: 'TCP', port: 80, targetPort: { type: 'name', value: 'web' }, nodePort: null, appProtocol: null }], selector: { app: 'api' }, externalEndpoints: [] }], page()))
      if (path.startsWith('/api/v1/stream?')) return Promise.resolve(new Response('', { status: 503, headers: { 'Content-Type': 'application/json' } }))
      if (path === '/api/v1/services/payments/api') return Promise.resolve(json({
        metadata: { namespace: 'payments', name: 'api', uid: 'uid-service', resourceVersion: '7', creationTimestamp: '2026-08-17T10:00:00Z', labels: {} },
        summary: { namespace: 'payments', name: 'api', type: 'ClusterIP', clusterIPs: ['10.96.0.10'], ports: [{ name: 'http', protocol: 'TCP', port: 80, targetPort: { type: 'name', value: 'web' }, nodePort: null, appProtocol: null }], selector: { app: 'api' }, externalEndpoints: [] },
        sessionAffinity: 'None', externalTrafficPolicy: null, ipFamilies: ['IPv4'], healthCheckNodePort: null,
      }))
      if (path.startsWith('/api/v1/permissions?')) {
        const ids = new URL(path, 'http://127.0.0.1').searchParams.getAll('capability')
        return Promise.resolve(json({ generation, complete: true, truncated: false, errors: [], decisions: ids.map((capabilityId) => ({ capabilityId, namespace: 'payments', resourceName: capabilityId === 'services.get' ? 'api' : '', decision: 'allowed' })) }))
      }
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-service', generation, origin: 'http://127.0.0.1:2748', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/services/payments/api/port-forward') {
        portForwardInit = init
        return Promise.resolve(json({ id: 'pf_service', clusterProfileId: 1, context: 'development', generation, namespace: 'payments', pod: 'api-a', remotePort: 8080, localAddress: '127.0.0.1', localPort: 49152, status: 'active', createdAt: '2026-08-17T10:00:00Z', expiresAt: '2026-08-17T18:00:00Z', endedAt: null, endReason: null }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<Routes><Route path="/network/:tab" element={<NetworkPage />} /></Routes>, ['/network/services'])
    fireEvent.click(await screen.findByRole('button', { name: 'Open services api in payments' }))
    fireEvent.click(await screen.findByRole('tab', { name: 'Actions' }))
    const start = await screen.findByRole('button', { name: 'Start port-forward' })
    await waitFor(() => expect(start).toBeEnabled())
    fireEvent.click(start)

    expect(await screen.findByRole('status')).toHaveTextContent(/Service port-forward active/i)
    expect(portForwardInit?.headers).toEqual(expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf-service', 'Idempotency-Key': expect.stringMatching(/^kp-/) }))
    expect(JSON.parse(String(portForwardInit?.body))).toEqual(expect.objectContaining({ remotePort: 80, consequenceCode: 'EXPOSE_SERVICE_PORT_LOCALLY', target: expect.objectContaining({ kind: 'Service', name: 'api' }) }))
  })

  it('restarts a compatible authorized workload selection with one contextual confirmation', async () => {
    const restarted: string[] = []
    const workloads = [
      { namespace: 'payments', kind: 'Deployment', name: 'api', ready: 2, desired: 2, available: 2, updated: 2, status: 'Healthy', ageSeconds: 120 },
      { namespace: 'payments', kind: 'StatefulSet', name: 'db', ready: 1, desired: 1, available: 1, updated: 1, status: 'Healthy', ageSeconds: 240 },
    ]
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path.startsWith('/api/v1/workloads?')) return Promise.resolve(json(workloads, page()))
      if (path.startsWith('/api/v1/stream?')) return Promise.resolve(new Response('', { status: 503, headers: { 'Content-Type': 'application/json' } }))
      if (path.startsWith('/api/v1/permissions?')) {
        const query = new URL(path, 'http://127.0.0.1').searchParams
        const capabilities = query.getAll('capability')
        const names = query.getAll('resourceName')
        return Promise.resolve(json({ generation, complete: true, truncated: false, errors: [], decisions: capabilities.flatMap((capabilityId) => names.map((resourceName) => ({ capabilityId, namespace: 'payments', resourceName, decision: 'allowed' }))) }))
      }
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-bulk', generation, origin: 'http://127.0.0.1:2748', expiresAt: '2026-08-17T18:00:00Z' }))
      const detail = path.match(/^\/api\/v1\/workloads\/(deployments|statefulsets)\/payments\/(api|db)$/)
      if (detail) {
        const kind = detail[1] === 'deployments' ? 'Deployment' : 'StatefulSet'
        return Promise.resolve(json({ metadata: { namespace: 'payments', name: detail[2], uid: `uid-${detail[2]}`, resourceVersion: '17', creationTimestamp: '2026-08-17T10:00:00Z', labels: {} }, kind, ready: 1, desired: 1, available: 1, updated: 1, status: 'Healthy', selector: {}, restartAt: null, conditions: [], containers: [], related: [] }))
      }
      const restart = path.match(/^\/api\/v1\/workloads\/(deployments|statefulsets)\/payments\/(api|db)\/restart$/)
      if (restart) {
        restarted.push(restart[2])
        return Promise.resolve(json({ accepted: true, action: 'restart', target: {}, generation, resourceVersion: '18' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<WorkloadsPage />)
    fireEvent.click(await screen.findByRole('checkbox', { name: 'Select row Deployment/payments/api' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select row StatefulSet/payments/db' }))
    expect(screen.getByRole('toolbar', { name: 'Bulk actions' })).toHaveTextContent('2 selected')
    const restart = screen.getByRole('button', { name: 'Restart selected' })
    await waitFor(() => expect(restart).toBeEnabled())
    fireEvent.click(restart)
    const dialog = screen.getByRole('alertdialog', { name: 'Restart 2 workloads' })
    expect(dialog).toHaveTextContent('Deployment api · ns payments')
    expect(dialog).toHaveTextContent('StatefulSet db · ns payments')
    fireEvent.click(within(dialog).getByRole('checkbox'))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart selected' }))

    await waitFor(() => expect(restarted).toEqual(['api', 'db']))
    expect(await screen.findByRole('status')).toHaveTextContent('Restarted 2 workloads')
  })

  it('shows only the authorized namespace seed as a non-selectable preview and clears it on revocation', async () => {
    const pod = { namespace: 'payments', name: 'seed-pod', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, node: null, ip: null, owner: null, ageSeconds: 60, problematic: false }
    let rejectFullPage: ((response: Response) => void) | undefined
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path === '/api/v1/pods?limit=20&namespace=payments') return Promise.resolve(json([pod], page()))
      if (path.startsWith('/api/v1/pods?')) return new Promise<Response>((resolve) => { rejectFullPage = resolve })
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf', generation, origin: 'http://127.0.0.1:2748', expiresAt: '2026-09-12T12:00:00Z' }))
      if (path === '/api/v1/stream?topic=pods') return Promise.resolve(new Response(JSON.stringify({ code: 'FORBIDDEN', message: 'watch revoked' }), { status: 403, headers: { 'Content-Type': 'application/json' } }))
      throw new Error(`Unexpected request: ${path}`)
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    await prefetchDefaultPodPreview(client, selectedStatus().selection as SelectionSummary, '', 'payments')
    renderPage(<PodsPage />, ['/'], client)
    expect(await screen.findByRole('button', { name: 'Open Pod seed-pod in payments' })).toBeInTheDocument()
    expect(screen.getByText(/✓ 1\/1 namespaces · partial preview/)).toBeInTheDocument()
    expect(screen.queryByRole('checkbox', { name: 'Select row payments/seed-pod' })).not.toBeInTheDocument()
    expect(paths.filter((path) => path === '/api/v1/pods?limit=20&namespace=payments')).toHaveLength(1)
    await waitFor(() => expect(rejectFullPage).toBeDefined())
    await act(async () => rejectFullPage?.(new Response(JSON.stringify({ code: 'FORBIDDEN', message: 'list revoked' }), { status: 403, headers: { 'Content-Type': 'application/json' } })))
    expect(await screen.findByText('Resource request failed')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Open Pod seed-pod in payments' })).not.toBeInTheDocument()
  })

  it('shows a bounded Pod stream preview while HTTP is pending, then replaces it with the authorized page', async () => {
    const streamed = { namespace: 'payments', name: 'streamed', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, node: null, ip: null, owner: null, ageSeconds: 60, problematic: false }
    let resolvePods: ((response: Response) => void) | undefined
    let streamController: ReadableStreamDefaultController<Uint8Array> | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path.startsWith('/api/v1/pods?')) return new Promise<Response>((resolve) => { resolvePods = resolve })
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf', generation, origin: 'http://127.0.0.1:2748', expiresAt: '2026-09-12T12:00:00Z' }))
      if (path === '/api/v1/stream?topic=pods') return Promise.resolve(new Response(new ReadableStream<Uint8Array>({ start(controller) {
        streamController = controller
        controller.enqueue(new TextEncoder().encode(`event: progress\ndata: ${JSON.stringify({ generation, topic: 'pods', snapshotId: 'snap_1', items: [streamed], completedNamespaces: 1, requestedNamespaces: 2 })}\n\n`))
      } }), { headers: { 'Content-Type': 'text/event-stream' } }))
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderPage(<PodsPage />)
    expect(await screen.findByRole('button', { name: 'Open Pod streamed in payments' })).toBeInTheDocument()
    expect(screen.getByText(/✓ 1\/2 namespaces · partial preview/)).toBeInTheDocument()
    expect(screen.queryByRole('checkbox', { name: 'Select row payments/streamed' })).not.toBeInTheDocument()
    await act(async () => { resolvePods?.(json([{ ...streamed, name: 'from-http' }], page())) })
    expect(await screen.findByRole('button', { name: 'Open Pod from-http in payments' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Open Pod streamed in payments' })).not.toBeInTheDocument()
    streamController?.close()
  })

  it.each([
    { name: 'Workloads', path: '/api/v1/workloads?', topic: 'workloads', route: '/', component: <WorkloadsPage />, item: { kind: 'Deployment', namespace: 'payments', name: 'streamed', status: 'Healthy', ready: 1, desired: 1, ageSeconds: 60 } },
    { name: 'Events', path: '/api/v1/events?', topic: 'events', route: '/', component: <EventsPage />, item: { namespace: 'payments', objectKind: 'Pod', objectName: 'streamed', timestamp: '2026-09-21T12:00:00Z', type: 'Normal', reason: 'Started', count: 1, message: 'streamed event' } },
    { name: 'Services', path: '/api/v1/services?', topic: 'services', route: '/network/services', component: <Routes><Route path="/network/:tab" element={<NetworkPage />} /></Routes>, item: { namespace: 'payments', name: 'streamed', type: 'ClusterIP', clusterIPs: ['10.0.0.1'] } },
    { name: 'ConfigMaps', path: '/api/v1/configmaps?', topic: 'configmaps', route: '/config/configmaps', component: <Routes><Route path="/config/:tab" element={<ConfigPage />} /></Routes>, item: { namespace: 'payments', name: 'streamed', uid: 'uid-streamed', creationTimestamp: '2026-09-21T12:00:00Z' } },
  ])('shows $name stream rows and coverage before the HTTP page completes', async ({ path, topic, route, component, item }) => {
    let resolvePage: ((response: Response) => void) | undefined
    let streamController: ReadableStreamDefaultController<Uint8Array> | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const requestPath = String(input)
      if (requestPath === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (requestPath === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (requestPath === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (requestPath.startsWith(path)) return new Promise<Response>((resolve) => { resolvePage = resolve })
      if (requestPath === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf', generation, origin: 'http://127.0.0.1:2748', expiresAt: '2026-09-12T12:00:00Z' }))
      if (requestPath === `/api/v1/stream?topic=${topic}`) return Promise.resolve(new Response(new ReadableStream<Uint8Array>({ start(controller) {
        streamController = controller
        controller.enqueue(new TextEncoder().encode(`event: progress\ndata: ${JSON.stringify({ generation, topic, snapshotId: 'snap_1', items: [item], completedNamespaces: 1, requestedNamespaces: 2 })}\n\n`))
      } }), { headers: { 'Content-Type': 'text/event-stream' } }))
      throw new Error(`Unexpected request: ${requestPath}`)
    }))
    renderPage(component, [route])
    await waitFor(() => expect(screen.getByRole('table')).toHaveTextContent('streamed'))
    expect(screen.getByText(/✓ 1\/2 namespaces · partial preview/)).toBeInTheDocument()
    await act(async () => resolvePage?.(json([{ ...item, name: 'from-http', objectName: 'from-http', message: 'from-http event', uid: 'uid-from-http' }], page())))
    await waitFor(() => expect(screen.getByRole('table')).toHaveTextContent('from-http'))
    expect(screen.getByRole('table')).not.toHaveTextContent('streamed')
    streamController?.close()
  })

  it('keeps hidden Storage columns in the chooser so they can be restored', async () => {
    const storedPreferences = {
      ...preferences(),
      columns: { hidden: { 'storage/persistent-volumes': ['status'] } },
    }
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/preferences' && init?.method !== 'PUT') return Promise.resolve(json(storedPreferences))
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf', generation, origin: 'http://127.0.0.1:2748', expiresAt: '2026-09-12T12:00:00Z' }))
      if (path === '/api/v1/preferences' && init?.method === 'PUT') return Promise.resolve(json({ ...storedPreferences, columns: { hidden: { 'storage/persistent-volumes': [] } } }))
      if (path === '/api/v1/persistent-volumes?limit=100') return Promise.resolve(json([{
        name: 'pv-data', status: 'Bound', capacity: '10Gi', storageClass: 'fast', claim: null, ageSeconds: 60,
      }], page()))
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderPage(
      <Routes><Route path="/storage/:tab" element={<StoragePage />} /></Routes>,
      ['/storage/persistent-volumes'],
    )

    await screen.findByRole('button', { name: 'Open pv-data' })
    expect(screen.queryByRole('columnheader', { name: 'Phase' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Choose visible columns' }))
    const statusColumn = screen.getByRole('checkbox', { name: 'status' })
    expect(statusColumn).not.toBeChecked()
    fireEvent.click(statusColumn)
    expect(await screen.findByRole('columnheader', { name: 'Phase' })).toBeInTheDocument()
  })

  it('accumulates Pod pages and drops old cursors when the global namespace changes', async () => {
    const paths: string[] = []
    const pod = { namespace: 'payments', name: 'all-pods', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false }
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path.startsWith('/api/v1/pods?')) {
        const params = new URL(path, 'http://127.0.0.1').searchParams
        return Promise.resolve(json([{ ...pod, name: params.has('namespace') ? 'filtered-pod' : params.has('continue') ? 'page-two' : 'all-pods' }], page(params.has('continue') ? '' : 'old-cursor')))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderPage(<PodsPage />)
    await screen.findByRole('button', { name: 'Open Pod all-pods in payments' })
    expect(screen.getByRole('button', { name: 'First page' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'First page' })).toHaveAttribute('title', 'Already on the first page.')
    fireEvent.click(screen.getByRole('button', { name: 'Load next page' }))
    await screen.findByRole('button', { name: 'Open Pod page-two in payments' })
    expect(screen.getByRole('button', { name: 'Open Pod all-pods in payments' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'First page' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'First page' }))
    await screen.findByRole('button', { name: 'Open Pod all-pods in payments' })
    const before = paths.length
    fireEvent.click(screen.getByRole('button', { name: 'Change global namespace' }))
    await screen.findByRole('button', { name: 'Open Pod filtered-pod in payments' })
    expect(paths.slice(before).filter((path) => path.startsWith('/api/v1/pods?')).every((path) => !path.includes('continue='))).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Change global namespace' }))
    await screen.findByRole('button', { name: 'Open Pod all-pods in payments' })
  })

  it('retains no more than five Pod pages while loading forward', async () => {
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path.startsWith('/api/v1/pods?')) {
        const cursor = new URL(path, 'http://127.0.0.1').searchParams.get('continue')
        const index = cursor === null ? 0 : Number(cursor)
        return Promise.resolve(json([{ namespace: 'payments', name: `page-${index}`, status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false }], page(index < 6 ? String(index + 1) : '')))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    const { client } = renderPage(<PodsPage />)
    await screen.findByRole('button', { name: 'Open Pod page-0 in payments' })
    for (let index = 1; index <= 5; index += 1) {
      fireEvent.click(screen.getByRole('button', { name: 'Load next page' }))
      await screen.findByRole('button', { name: `Open Pod page-${index} in payments` })
    }
    expect(screen.queryByRole('button', { name: 'Open Pod page-0 in payments' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open Pod page-1 in payments' })).toBeInTheDocument()
    const data = client.getQueriesData({ queryKey: ['resources', 'pods'] }).map(([, value]) => value).find((value): value is { pages: unknown[]; pageParams: unknown[] } => Boolean(value && typeof value === 'object' && 'pages' in value))
    expect(data).toBeDefined()
    expect(data?.pages).toHaveLength(5)
    expect(data?.pageParams).toHaveLength(5)
  })

  it('replaces accumulated Pods when a continuation expires', async () => {
    let firstPageRequests = 0
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path.startsWith('/api/v1/pods?')) {
        const cursor = new URL(path, 'http://127.0.0.1').searchParams.get('continue')
        if (cursor === 'expired-token') return Promise.resolve(new Response(JSON.stringify({ code: 'CURSOR_EXPIRED', message: 'expired' }), { status: 410, headers: { 'Content-Type': 'application/json' } }))
        firstPageRequests += 1
        const name = firstPageRequests === 1 ? 'old-pod' : 'fresh-pod'
        return Promise.resolve(json([{ namespace: 'payments', name, status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false }], page(firstPageRequests === 1 ? 'expired-token' : '')))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderPage(<PodsPage />)
    await screen.findByRole('button', { name: 'Open Pod old-pod in payments' })
    fireEvent.click(screen.getByRole('button', { name: 'Load next page' }))
    await screen.findByRole('button', { name: 'Open Pod fresh-pod in payments' })
    expect(screen.queryByRole('button', { name: 'Open Pod old-pod in payments' })).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('snapshot expired')
    expect(firstPageRequests).toBe(2)
  })

  it.each([
    { code: 'FORBIDDEN', status: 403 },
    { code: 'AUTHORIZATION_UNAVAILABLE', status: 503 },
  ])('hides loaded Pods when a later page returns $code', async ({ code, status }) => {
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path.startsWith('/api/v1/pods?')) {
        const cursor = new URL(path, 'http://127.0.0.1').searchParams.get('continue')
        if (cursor) return Promise.resolve(new Response(JSON.stringify({ code, message: 'access cannot be confirmed' }), { status, headers: { 'Content-Type': 'application/json' } }))
        return Promise.resolve(json([{ namespace: 'payments', name: 'private-pod', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false }], page('next-token')))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderPage(<PodsPage />)
    await screen.findByRole('button', { name: 'Open Pod private-pod in payments' })
    fireEvent.click(screen.getByRole('button', { name: 'Load next page' }))
    await screen.findByText('Resource request failed')
    expect(screen.queryByRole('button', { name: 'Open Pod private-pod in payments' })).not.toBeInTheDocument()
  })

  it('keeps Pods visible while same-selection filters refresh', async () => {
    let release: ((response: Response) => void) | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (path.startsWith('/api/v1/pods?')) {
        if (new URL(path, 'http://127.0.0.1').searchParams.has('search')) return new Promise<Response>((resolve) => { release = resolve })
        return Promise.resolve(json([{ namespace: 'payments', name: 'old-pod', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false }], page()))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderPage(<PodsPage />)
    await screen.findByRole('button', { name: 'Open Pod old-pod in payments' })
    fireEvent.change(screen.getByLabelText('Search this bounded page'), { target: { value: 'new' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))
    await screen.findByText('Refreshing Pods…')
    expect(screen.getByRole('button', { name: 'Open Pod old-pod in payments' })).toBeInTheDocument()
    await act(async () => release?.(json([{ namespace: 'payments', name: 'new-pod', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false }], page())))
    await screen.findByRole('button', { name: 'Open Pod new-pod in payments' })
    expect(screen.queryByRole('button', { name: 'Open Pod old-pod in payments' })).not.toBeInTheDocument()
  })

  it.each([
    { name: 'Workloads', path: '/api/v1/workloads?', component: <WorkloadsPage />, item: { kind: 'Deployment', namespace: 'payments', name: 'old-marker', status: 'Healthy', ready: 1, desired: 1, ageSeconds: 60 } },
    { name: 'Pods', path: '/api/v1/pods?', component: <PodsPage />, item: { namespace: 'payments', name: 'old-marker', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false } },
    { name: 'Events', path: '/api/v1/events?', component: <EventsPage />, item: { namespace: 'payments', objectKind: 'Pod', objectName: 'old-marker', timestamp: '2026-09-21T12:00:00Z', type: 'Normal', reason: 'Started', count: 1, message: 'old-marker' } },
  ])('drops $name rows as soon as the effective namespace filter changes', async ({ path, component, item }) => {
    let release: ((response: Response) => void) | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const requestPath = String(input)
      if (requestPath === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (requestPath === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (requestPath === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (requestPath.startsWith(path)) {
        const namespace = new URL(requestPath, 'http://127.0.0.1').searchParams.get('namespace')
        if (namespace === 'other') return new Promise<Response>((resolve) => { release = resolve })
        return Promise.resolve(json([item], page()))
      }
      throw new Error(`Unexpected request: ${requestPath}`)
    }))
    renderPage(component)
    await screen.findAllByText('old-marker')
    fireEvent.change(screen.getByLabelText('Namespace'), { target: { value: 'other' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))
    await waitFor(() => expect(release).toBeDefined())
    expect(screen.queryAllByText('old-marker')).toHaveLength(0)
    await act(async () => release?.(json([], page())))
  })


  it.each([null, []])('opens a Pod whose relatedEvents is %j without blanking the page', async (relatedEvents) => {
    const pod = { namespace: 'payments', name: 'api-empty-events', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, node: 'worker-1', ip: null, owner: null, ageSeconds: 60, problematic: false }
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json({ ...selectedStatus(), components: { ...selectedStatus().components, metrics: { status: 'unknown' } } }))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path.startsWith('/api/v1/pods?')) return Promise.resolve(json([pod], page()))
      if (path === '/api/v1/pods/payments/api-empty-events') return Promise.resolve(json({
        metadata: { namespace: pod.namespace, name: pod.name, uid: 'uid-empty-events', resourceVersion: '1', labels: {} },
        summary: pod, conditions: [], containers: [], initContainers: [], ephemeralContainers: [], relatedEvents,
      }))
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderPage(<PodsPage />)
    fireEvent.click(await screen.findByRole('button', { name: 'Open Pod api-empty-events in payments' }))
    expect(await screen.findByText('uid-empty-events')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Pods' })).toBeInTheDocument()
    expect(screen.getByRole('dialog')).toHaveTextContent('This Pod has no controller owner.')
    expect(screen.getByRole('button', { name: 'Go to previous resource' })).toHaveAttribute('title', 'There is no previous resource in this workspace history.')
    expect(screen.getByRole('button', { name: 'Go to next resource' })).toHaveAttribute('title', 'There is no next resource in this workspace history.')
  })

  it('navigates a bounded workload list to generation-fenced detail and explicit YAML', async () => {
    const calls: Array<{ path: string; init?: RequestInit }> = []
    let activeGeneration = generation
    const workload = { namespace: 'payments', kind: 'Deployment', name: 'api', ready: 2, desired: 3, available: 2, updated: 3, status: 'Degraded', ageSeconds: 120 }
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      calls.push({ path, init })
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus(activeGeneration), {}, activeGeneration))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/workloads?limit=100') return Promise.resolve(json(activeGeneration === generation ? [workload] : [{ ...workload, name: 'fresh' }], page(activeGeneration === generation ? 'next-token' : ''), activeGeneration))
      if (path === '/api/v1/workloads?limit=100&continue=next-token') return Promise.resolve(json([{ ...workload, name: 'worker' }], page()))
      if (path === '/api/v1/workloads/deployments/payments/api') return Promise.resolve(json({
        metadata: { namespace: 'payments', name: 'api', uid: 'uid-api', resourceVersion: '17', creationTimestamp: '2026-08-17T10:00:00Z', labels: { app: 'api' } },
        kind: 'Deployment', ready: 2, desired: 3, available: 2, updated: 3, status: 'Degraded', selector: { app: 'api' }, restartAt: null, conditions: [], containers: [{ name: 'api', image: 'example/api:1', ports: [] }], related: [],
      }))
      if (path.startsWith('/api/v1/permissions?')) return Promise.resolve(json({ generation, complete: true, truncated: false, errors: [], decisions: [
        { capabilityId: 'deployments.restart', namespace: 'payments', decision: 'allowed' },
        { capabilityId: 'deployments.scale', namespace: 'payments', decision: 'allowed' },
      ] }))
      if (path === '/api/v1/workloads/deployments/payments/api/yaml') return Promise.resolve(new Response('apiVersion: apps/v1\nkind: Deployment\n', { headers: { 'Content-Type': 'application/yaml' } }))
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetch)

    const { client } = renderPage(<WorkloadsPage />)

    expect(sortOptionValues()).toEqual(['identity', 'name', 'age', 'status'])

    fireEvent.click(await screen.findByRole('button', { name: 'Open Deployment api in payments' }))
    expect(await screen.findByText('17')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: 'YAML' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Load authorized YAML' }))
    expect(await screen.findByLabelText('YAML document')).toHaveTextContent('kind: Deployment')

    fireEvent.click(screen.getByRole('button', { name: 'Load next page' }))
    expect(await screen.findByRole('button', { name: 'Open Deployment worker in payments' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open Deployment api in payments' })).toBeInTheDocument()
    expect(calls.some((call) => call.path.endsWith('continue=next-token'))).toBe(true)
    const resourceRequestsBeforeGenerationChange = calls.filter((call) => call.path.startsWith('/api/v1/workloads?')).length
    activeGeneration = 'gen_43'
    await act(async () => client.setQueryData(['local-status'], selectedStatus(activeGeneration)))
    expect(await screen.findByRole('button', { name: 'Open Deployment fresh in payments' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Open Deployment worker in payments' })).not.toBeInTheDocument()
    const requestsAfterGenerationChange = calls.filter((call) => call.path.startsWith('/api/v1/workloads?')).slice(resourceRequestsBeforeGenerationChange)
    expect(requestsAfterGenerationChange.some((call) => call.path === '/api/v1/workloads?limit=100')).toBe(true)
    expect(requestsAfterGenerationChange.some((call) => call.path.includes('continue='))).toBe(false)
    const yamlCall = calls.find((call) => call.path.endsWith('/yaml'))
    expect(yamlCall?.init).toEqual(expect.objectContaining({ cache: 'no-store', credentials: 'same-origin' }))
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })

  it('applies visual namespace, workload and node filters to the bounded Pod query', async () => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/preferences') return Promise.resolve(json({ ...preferences(), filters: { ...preferences().filters, pods: { version: 1, items: [
        { id: 'saved-pods', name: 'Saved problem Pods', query: { namespace: ['ops', 'payments'], search: 'failed', status: ['Failed'], workload: 'api', node: 'worker-9', restarts: 'gte3', problematic: true, sort: 'age', order: 'desc' } },
        { id: 'saved-safe', name: 'Saved non-problem Pods', query: { problematic: false, sort: 'secretData', order: 'sideways' } },
      ] } } }))
      if (path.startsWith('/api/v1/pods?')) return Promise.resolve(json([], page()))
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<PodsPage />)
    await screen.findByRole('combobox', { name: 'Saved filter' })
    expect(sortOptionValues()).toEqual(['identity', 'name', 'age', 'restarts', 'status'])
    const initialPodRequests = paths.filter((path) => path.startsWith('/api/v1/pods?')).length
    fireEvent.change(await screen.findByLabelText('Namespace'), { target: { value: 'payments' } })
    fireEvent.change(screen.getByLabelText('Workload owner'), { target: { value: 'api' } })
    fireEvent.change(screen.getByLabelText('Node'), { target: { value: 'worker-1' } })
    fireEvent.change(screen.getByLabelText('Search this bounded page'), { target: { value: 'backend' } })
    fireEvent.change(screen.getByLabelText('Sort this bounded page'), { target: { value: 'restarts' } })
    fireEvent.change(screen.getByLabelText('Order'), { target: { value: 'desc' } })
    expect(screen.getByRole('status')).toHaveTextContent('Filter changes pending')
    expect(paths.filter((path) => path.startsWith('/api/v1/pods?'))).toHaveLength(initialPodRequests)
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))

    await waitFor(() => expect(paths.some((path) => {
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return query.get('namespace') === 'payments' && query.get('workload') === 'api' && query.get('node') === 'worker-1' && query.get('search') === 'backend' && query.get('sort') === 'restarts' && query.get('order') === 'desc'
    })).toBe(true))
    expect(screen.getByText('Restarts · descending')).toBeInTheDocument()

    const requestsBeforeClear = paths.length
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    await waitFor(() => expect(screen.getByText('None')).toBeInTheDocument())
    expect(paths.slice(requestsBeforeClear).filter((path) => path.startsWith('/api/v1/pods?')).every((path) => {
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return !query.has('namespace') && !query.has('search') && !query.has('sort') && !query.has('order') && !query.has('continue')
    })).toBe(true)

    fireEvent.change(await screen.findByRole('combobox', { name: 'Saved filter' }), { target: { value: 'saved-pods' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply saved filter' }))
    expect(screen.getByLabelText('Namespace')).toHaveValue('ops, payments')
    expect(screen.getByLabelText('Workload owner')).toHaveValue('api')
    expect(screen.getByLabelText('Node')).toHaveValue('worker-9')
    expect(screen.getByLabelText('Status')).toHaveValue('Failed')
    expect(screen.getByLabelText('Sort this bounded page')).toHaveValue('age')
    expect(screen.getByLabelText('Order')).toHaveValue('desc')
    await waitFor(() => expect(paths.some((path) => {
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return query.getAll('namespace').join(',') === 'ops,payments' && query.get('restarts') === 'gte3' && query.get('problematic') === 'true' && query.get('sort') === 'age' && query.get('order') === 'desc'
    })).toBe(true))

    fireEvent.change(screen.getByRole('combobox', { name: 'Saved filter' }), { target: { value: 'saved-safe' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply saved filter' }))
    expect(screen.getByLabelText('Problem evidence')).toHaveValue('false')
    expect(screen.getByLabelText('Sort this bounded page')).toHaveValue('identity')
    expect(screen.getByLabelText('Order')).toHaveValue('asc')
    await waitFor(() => expect(paths.some((path) => {
      if (!path.startsWith('/api/v1/pods?')) return false
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return query.get('problematic') === 'false' && !query.has('sort') && !query.has('order')
    })).toBe(true))
  })

  it('renders Secret detail through an explicit metadata allowlist and never offers YAML', async () => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/configmaps?limit=100') return Promise.resolve(json([], page()))
      if (path === '/api/v1/secrets?limit=100') return Promise.resolve(json([{ apiVersion: 'v1', kind: 'Secret', metadata: { namespace: 'payments', name: 'registry', uid: 'uid-secret', creationTimestamp: '2026-08-17T10:00:00Z' } }], page()))
      if (path === '/api/v1/secrets/payments/registry') return Promise.resolve(json({
        apiVersion: 'v1', kind: 'Secret', metadata: { namespace: 'payments', name: 'registry', uid: 'uid-secret', creationTimestamp: '2026-08-17T10:00:00Z', annotations: { token: 'annotation-secret' } }, data: { password: 'super-secret' }, stringData: { token: 'raw-token' },
      }))
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<ConfigPage />)
    expect(sortOptionValues()).toEqual(['identity', 'name', 'createdAt'])
    fireEvent.click(screen.getByRole('tab', { name: 'secrets' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Open Secret registry in payments' }))

    expect(await screen.findByText(/Secret values, annotations, managed fields and YAML are intentionally unavailable/)).toBeInTheDocument()
    expect(screen.getAllByText('uid-secret').length).toBeGreaterThanOrEqual(1)
    expect(screen.queryByText(/super-secret|annotation-secret|raw-token/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Load authorized YAML' })).not.toBeInTheDocument()
    expect(paths.some((path) => path.includes('/secrets/') && path.endsWith('/yaml'))).toBe(false)
  })

  it('navigates Network tabs from the canonical sidebar route and changes the active query', async () => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path.startsWith('/api/v1/services?') || path.startsWith('/api/v1/ingresses?')) return Promise.resolve(json([], page()))
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<><Routes><Route path="/network/:tab" element={<NetworkPage />} /></Routes><LocationProbe /></>, ['/network/services'])
    await waitFor(() => expect(paths.some((path) => path.startsWith('/api/v1/services?'))).toBe(true))
    fireEvent.click(screen.getByRole('tab', { name: 'ingresses' }))
    expect(await screen.findByLabelText('Current route')).toHaveTextContent('/network/ingresses')
    await waitFor(() => expect(paths.some((path) => path.startsWith('/api/v1/ingresses?'))).toBe(true))
  })

  it('navigates Config tabs from the canonical sidebar route and changes the active query', async () => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path.startsWith('/api/v1/configmaps?') || path.startsWith('/api/v1/secrets?')) return Promise.resolve(json([], page()))
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<><Routes><Route path="/config/:tab" element={<ConfigPage />} /></Routes><LocationProbe /></>, ['/config/configmaps'])
    await waitFor(() => expect(paths.some((path) => path.startsWith('/api/v1/configmaps?'))).toBe(true))
    fireEvent.click(screen.getByRole('tab', { name: 'secrets' }))
    expect(await screen.findByLabelText('Current route')).toHaveTextContent('/config/secrets')
    await waitFor(() => expect(paths.some((path) => path.startsWith('/api/v1/secrets?'))).toBe(true))
  })

  it('keeps draft search and allowlisted ordering independent across Network tabs', async () => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path.startsWith('/api/v1/services?') || path.startsWith('/api/v1/ingresses?') || path.startsWith('/api/v1/endpoint-slices?')) return Promise.resolve(json([], page()))
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<NetworkPage />)
    await screen.findByLabelText('Search this bounded page')
    expect(sortOptionValues()).toEqual(['identity', 'name', 'type'])
    await waitFor(() => expect(paths.filter((path) => path.startsWith('/api/v1/services?'))).toHaveLength(1))
    const initialServiceRequests = paths.filter((path) => path.startsWith('/api/v1/services?')).length
    fireEvent.change(screen.getByLabelText('Search this bounded page'), { target: { value: 'edge' } })
    fireEvent.change(screen.getByLabelText('Sort this bounded page'), { target: { value: 'type' } })
    expect(paths.filter((path) => path.startsWith('/api/v1/services?'))).toHaveLength(initialServiceRequests)

    fireEvent.click(screen.getByRole('tab', { name: 'ingresses' }))
    expect(sortOptionValues()).toEqual(['identity', 'name'])
    expect(screen.getByLabelText('Search this bounded page')).toHaveValue('')
    expect(screen.getByLabelText('Sort this bounded page')).toHaveValue('identity')
    fireEvent.change(screen.getByLabelText('Search this bounded page'), { target: { value: 'public' } })
    fireEvent.change(screen.getByLabelText('Sort this bounded page'), { target: { value: 'name' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))
    await waitFor(() => expect(paths.some((path) => {
      if (!path.startsWith('/api/v1/ingresses?')) return false
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return query.get('search') === 'public' && query.get('sort') === 'name' && query.get('order') === 'asc'
    })).toBe(true))

    fireEvent.click(screen.getByRole('tab', { name: 'services' }))
    expect(screen.getByLabelText('Search this bounded page')).toHaveValue('edge')
    expect(screen.getByLabelText('Sort this bounded page')).toHaveValue('type')
    expect(screen.getByRole('status')).toHaveTextContent('Filter changes pending')
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))
    await waitFor(() => expect(paths.some((path) => {
      if (!path.startsWith('/api/v1/services?')) return false
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return query.get('search') === 'edge' && query.get('sort') === 'type' && query.get('order') === 'asc'
    })).toBe(true))

    fireEvent.click(screen.getByRole('tab', { name: 'endpoint-slices' }))
    expect(sortOptionValues()).toEqual(['identity', 'name', 'addressType'])
    await waitFor(() => expect(paths.some((path) => path === '/api/v1/endpoint-slices?limit=100')).toBe(true))
  })

  it('uses the exact Event ordering catalog and applies non-default server ordering', async () => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path.startsWith('/api/v1/events?')) return Promise.resolve(json([], page()))
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<EventsPage />)
    await screen.findByLabelText('Sort this bounded page')
    expect(sortOptionValues()).toEqual(['timestamp', 'count', 'identity'])
    fireEvent.change(screen.getByLabelText('Sort this bounded page'), { target: { value: 'count' } })
    fireEvent.change(screen.getByLabelText('Order'), { target: { value: 'asc' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))

    await waitFor(() => expect(paths.some((path) => {
      if (!path.startsWith('/api/v1/events?')) return false
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return query.get('sort') === 'count' && query.get('order') === 'asc'
    })).toBe(true))
  })

  it('shows port-forward sessions only as generation-matched loopback listeners', async () => {
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/services?limit=100') return Promise.resolve(json([], page()))
      if (path === '/api/v1/port-forwards') return Promise.resolve(json([{ id: 'pf_1', clusterProfileId: 1, context: 'development', generation, namespace: 'payments', pod: 'api-abc', remotePort: 8080, localAddress: '127.0.0.1', localPort: 49152, status: 'active', createdAt: '2026-08-17T10:00:00Z', expiresAt: '2026-08-17T18:00:00Z', endedAt: null, endReason: null }]))
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<NetworkPage />)
    fireEvent.click(screen.getByRole('tab', { name: 'port-forwards' }))

    expect(await screen.findByText('127.0.0.1:49152')).toBeInTheDocument()
    expect(screen.getByText('development · payments/api-abc → 8080')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: 'Close loopback session' })).toBeEnabled())
  })
})
