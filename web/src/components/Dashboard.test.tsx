import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DashboardPage } from './Dashboard'

function json(data: unknown, meta = { generation: 'gen_42', collectedAt: '2026-08-10T12:00:00Z' }): Response {
  return new Response(JSON.stringify({ data, meta }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function block<T>(value: T, overrides: Record<string, unknown> = {}) {
  return {
    value,
    complete: true,
    truncated: false,
    coverage: { requestedNamespaces: 2, completedNamespaces: 2, deniedNamespaces: [], failed: [] },
    errors: [],
    ...overrides,
  }
}

function counter(value: number) {
  return { state: 'available', value }
}

function summary() {
  return {
    namespaces: counter(2),
    podsTotal: counter(8),
    podsHealthy: counter(7),
    podsProblematic: counter(1),
    workloadsDegraded: counter(1),
    restarts: counter(12),
    warningEvents: { state: 'denied', value: null },
    possibleLogMatches: { state: 'notCollected', value: null },
  }
}

function selectedStatus() {
  return {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, {
      status: name === 'cluster' || name === 'application' ? 'healthy' : 'unknown',
      code: 'TEST', message: name === 'cluster' ? 'Cluster connection is healthy.' : 'test', checkedAt: null,
    }])),
    selection: {
      clusterProfileId: 1, context: 'development', cluster: 'dev-cluster', scopeId: 7,
      scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments',
      namespaceCount: 2, generation: 'gen_42',
    },
  }
}

function renderDashboard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter><DashboardPage /></MemoryRouter>
    </QueryClientProvider>,
  )
}

function defaultResponse(path: string): Response {
  if (path === '/api/v1/status') return json(selectedStatus())
  if (path === '/api/v1/session') return json({ csrfToken: 'csrf_test', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-10T13:00:00Z' })
  if (path === '/api/v1/dashboard/summary') return json(block(summary(), { coverage: null }))
  if (path === '/api/v1/dashboard/problems') return json(block([]))
  if (path === '/api/v1/dashboard/restarts?limit=10') return json(block([]))
  if (path === '/api/v1/dashboard/events') return json(block([]))
  if (path === '/api/v1/metrics') return json(block({ collectedAt: '2026-08-10T12:00:00Z', windowSeconds: 60, pods: [], topCPU: [], topMemory: [] }))
  throw new Error(`Unexpected request: ${path}`)
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('progressive dashboard', () => {
  it('keeps partial, empty, and optional blocks distinct while loading all queries independently', async () => {
    const fetch = vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/dashboard/problems') {
        return Promise.resolve(json(block([{
          namespace: 'payments', pod: 'api-abc', owner: { kind: 'Deployment', name: 'api' },
          container: 'api', containerType: 'regular', status: 'Running', reason: 'CrashLoopBackOff',
          message: 'back-off restarting failed container', source: 'containerWaiting', severity: 'critical', ageSeconds: 180,
        }], {
          complete: false,
          coverage: { requestedNamespaces: 2, completedNamespaces: 1, deniedNamespaces: ['restricted'], failed: [] },
          errors: [{ namespace: 'restricted', code: 'FORBIDDEN', message: 'Access was denied.' }],
        })))
      }
      if (path === '/api/v1/dashboard/restarts?limit=10') {
        return Promise.resolve(json(block([{
          namespace: 'payments', pod: 'api-abc', owner: { kind: 'Deployment', name: 'api' }, container: 'api',
          containerType: 'regular', restarts: 12, severity: 'critical', status: 'CrashLoopBackOff', lastReason: 'Error', ageSeconds: 840,
        }])))
      }
      if (path === '/api/v1/dashboard/events') {
        return Promise.resolve(json(block([{
          timestamp: '2026-08-10T11:59:00Z', namespace: 'payments', objectKind: 'Pod', objectName: 'api-abc',
          reason: 'BackOff', message: 'Back-off restarting failed container', count: 4, source: 'kubelet', type: 'Warning',
        }])))
      }
      if (path === '/api/v1/metrics') {
        return Promise.resolve(json(block({ collectedAt: '', windowSeconds: 0, pods: [], topCPU: [], topMemory: [] }, {
          complete: false,
          errors: [{ code: 'FEATURE_UNAVAILABLE', message: 'The optional feature is unavailable.' }],
        })))
      }
      return Promise.resolve(defaultResponse(path))
    })
    vi.stubGlobal('fetch', fetch)

    renderDashboard()

    expect(await screen.findByRole('heading', { name: 'Cluster overview' })).toBeInTheDocument()
    expect((await screen.findAllByText('api-abc', { selector: 'strong' })).length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('Coverage: 1 of 2 namespaces. 1 denied.')).toBeInTheDocument()
    expect(screen.getByText('BackOff', { selector: 'strong' })).toBeInTheDocument()
    expect(screen.getByText('Metrics API is not available. The rest of the dashboard is unaffected.')).toBeInTheDocument()
    expect(screen.getByText('Scan has not been run')).toBeInTheDocument()
    expect(screen.getByLabelText('Warning events: access denied')).toBeInTheDocument()

    const paths = fetch.mock.calls.map(([input]) => String(input))
    expect(paths).toEqual(expect.arrayContaining([
      '/api/v1/dashboard/summary', '/api/v1/dashboard/problems', '/api/v1/dashboard/restarts?limit=10',
      '/api/v1/dashboard/events', '/api/v1/metrics',
    ]))
  })

  it('runs the bounded log scan explicitly with CSRF and keeps only sanitized in-memory results', async () => {
    let scanInit: RequestInit | undefined
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/dashboard/log-scan') {
        scanInit = init
        return Promise.resolve(json(block([{
          namespace: 'payments', pod: 'api-abc', container: 'api', workload: { kind: 'Deployment', name: 'api' },
          timestamp: '2026-08-10T12:00:00Z', excerpt: 'authorization=[REDACTED]', reasonCode: 'ERROR_KEYWORD', redacted: true, truncated: false,
        }])))
      }
      return Promise.resolve(defaultResponse(path))
    })
    vi.stubGlobal('fetch', fetch)
    renderDashboard()

    const scanButton = await screen.findByRole('button', { name: 'Scan logs now' })
    await waitFor(() => expect(scanButton).toBeEnabled())
    fireEvent.click(scanButton)

    expect(await screen.findByText('authorization=[REDACTED]')).toBeInTheDocument()
    expect(screen.getByText('sensitive value redacted')).toBeInTheDocument()
    expect(scanInit).toEqual(expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf_test' }),
    }))
    expect(JSON.parse(String(scanInit?.body))).toEqual({ window: '15m', tailLines: 200, maxPods: 20, maxConcurrentContainers: 4 })
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })

  it('cancels the previous log scan when a new explicit scan starts', async () => {
    const scanSignals: AbortSignal[] = []
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/dashboard/log-scan') {
        const signal = init?.signal as AbortSignal
        scanSignals.push(signal)
        return new Promise<Response>((_resolve, reject) => {
          signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')), { once: true })
        })
      }
      return Promise.resolve(defaultResponse(path))
    })
    vi.stubGlobal('fetch', fetch)
    renderDashboard()

    const scanButton = await screen.findByRole('button', { name: 'Scan logs now' })
    await waitFor(() => expect(scanButton).toBeEnabled())
    fireEvent.click(scanButton)
    fireEvent.click(await screen.findByRole('button', { name: 'Restart scan' }))

    await waitFor(() => expect(scanSignals).toHaveLength(2))
    expect(scanSignals[0].aborted).toBe(true)
    expect(scanSignals[1].aborted).toBe(false)
  })

  it('renders denial and connectivity failure at the affected blocks only', async () => {
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/dashboard/problems') {
        return Promise.resolve(json(block([], { complete: false, errors: [{ code: 'FORBIDDEN', message: 'Denied.' }] })))
      }
      if (path === '/api/v1/dashboard/restarts?limit=10') {
        return Promise.reject(new TypeError('offline'))
      }
      return Promise.resolve(defaultResponse(path))
    }))
    renderDashboard()

    expect(await screen.findByRole('heading', { name: 'Cluster overview' })).toBeInTheDocument()
    expect(await screen.findByText('Access denied', { selector: 'strong' })).toBeInTheDocument()
    expect(await screen.findByText('Cluster data is offline', { selector: 'strong' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Warning events' })).toBeInTheDocument()
  })
})
