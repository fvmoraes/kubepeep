import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ConfigPage, NetworkPage, PodsPage, WorkloadsPage } from './ResourcePages'

const generation = 'gen_42'

function json(data: unknown, meta: Record<string, unknown> = {}): Response {
  return new Response(JSON.stringify({ data, meta: { generation, ...meta } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function page(next = '') {
  return { page: { limit: 100, next, complete: next === '', truncated: false, filterScope: 'page' }, coverage: null }
}

function selectedStatus() {
  return {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: 'healthy', code: 'TEST', message: 'test', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'dev-cluster', scopeId: 7, scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 1, generation },
  }
}

function preferences() {
  const empty = { version: 1, items: [] }
  return { version: 1, ui: { language: 'en' }, logs: { wrap: false, timestamps: true, tailLines: 200 }, dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] }, filters: { workloads: empty, pods: empty, events: empty, logs: empty } }
}

function renderPage(component: React.ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter>{component}</MemoryRouter></QueryClientProvider>)
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('read-only resource pages', () => {
  it('navigates a bounded workload list to generation-fenced detail and explicit YAML', async () => {
    const calls: Array<{ path: string; init?: RequestInit }> = []
    const workload = { namespace: 'payments', kind: 'Deployment', name: 'api', ready: 2, desired: 3, available: 2, updated: 3, status: 'Degraded', ageSeconds: 120 }
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      calls.push({ path, init })
      if (path === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/workloads?limit=100') return Promise.resolve(json([workload], page('next-token')))
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

    renderPage(<WorkloadsPage />)

    fireEvent.click(await screen.findByRole('button', { name: 'Open Deployment api in payments' }))
    expect(await screen.findByText('17')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Load authorized YAML' }))
    expect(await screen.findByLabelText('YAML document')).toHaveTextContent('kind: Deployment')

    fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    expect(await screen.findByRole('button', { name: 'Open Deployment worker in payments' })).toBeInTheDocument()
    expect(calls.some((call) => call.path.endsWith('continue=next-token'))).toBe(true)
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
      if (path === '/api/v1/preferences') return Promise.resolve(json({ ...preferences(), filters: { ...preferences().filters, pods: { version: 1, items: [{ id: 'saved-pods', name: 'Saved problem Pods', query: { namespace: ['ops', 'payments'], search: 'failed', status: ['Failed'], workload: 'api', node: 'worker-9', restarts: 'gte3', problematic: true } }] } } }))
      if (path.startsWith('/api/v1/pods?')) return Promise.resolve(json([], page()))
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderPage(<PodsPage />)
    fireEvent.change(await screen.findByLabelText('Namespace'), { target: { value: 'payments' } })
    fireEvent.change(screen.getByLabelText('Workload owner'), { target: { value: 'api' } })
    fireEvent.change(screen.getByLabelText('Node'), { target: { value: 'worker-1' } })
    fireEvent.change(screen.getByLabelText('Search this bounded page'), { target: { value: 'backend' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))

    await waitFor(() => expect(paths.some((path) => {
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return query.get('namespace') === 'payments' && query.get('workload') === 'api' && query.get('node') === 'worker-1' && query.get('search') === 'backend'
    })).toBe(true))

    fireEvent.change(await screen.findByRole('combobox', { name: 'Saved filter' }), { target: { value: 'saved-pods' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply saved filter' }))
    expect(screen.getByLabelText('Namespace')).toHaveValue('ops, payments')
    expect(screen.getByLabelText('Workload owner')).toHaveValue('api')
    expect(screen.getByLabelText('Node')).toHaveValue('worker-9')
    expect(screen.getByLabelText('Status')).toHaveValue('Failed')
    await waitFor(() => expect(paths.some((path) => {
      const query = new URL(path, 'http://127.0.0.1').searchParams
      return query.getAll('namespace').join(',') === 'ops,payments' && query.get('restarts') === 'gte3' && query.get('problematic') === 'true'
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
    fireEvent.click(screen.getByRole('tab', { name: 'secrets' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Open Secret registry in payments' }))

    expect(await screen.findByText(/Secret values, annotations, managed fields and YAML are intentionally unavailable/)).toBeInTheDocument()
    expect(screen.getAllByText('uid-secret').length).toBeGreaterThanOrEqual(1)
    expect(screen.queryByText(/super-secret|annotation-secret|raw-token/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Load authorized YAML' })).not.toBeInTheDocument()
    expect(paths.some((path) => path.includes('/secrets/') && path.endsWith('/yaml'))).toBe(false)
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
