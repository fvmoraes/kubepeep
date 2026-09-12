import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { StoragePage } from './FamilyPages'
import { ConfigPage, EventsPage, NetworkPage, PodsPage, WorkloadsPage } from './ResourcePages'
import { ToastProvider } from './ui/Toast'
import { ResourceWorkspaceProvider } from './workspace/ResourceWorkspaceProvider'
import { ResourceWorkspaceOverlay } from './workspace/ResourceWorkspace'
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

function renderPage(component: React.ReactNode, initialEntries: string[] = ['/']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
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

  it('reloads Pods and drops the old cursor when the global namespace changes', async () => {
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
    fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    await screen.findByRole('button', { name: 'Open Pod page-two in payments' })
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

    fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    expect(await screen.findByRole('button', { name: 'Open Deployment worker in payments' })).toBeInTheDocument()
    expect(calls.some((call) => call.path.endsWith('continue=next-token'))).toBe(true)
    const resourceRequestsBeforeGenerationChange = calls.filter((call) => call.path.startsWith('/api/v1/workloads?')).length
    activeGeneration = 'gen_43'
    await act(async () => client.setQueryData(['local-status'], selectedStatus(activeGeneration)))
    expect(await screen.findByRole('button', { name: 'Open Deployment fresh in payments' })).toBeInTheDocument()
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
    await waitFor(() => expect(paths.slice(requestsBeforeClear).some((path) => {
      if (!path.startsWith('/api/v1/pods?')) return false
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return !query.has('namespace') && !query.has('search') && !query.has('sort') && !query.has('order') && !query.has('continue')
    })).toBe(true))
    expect(screen.getByText('None')).toBeInTheDocument()

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
