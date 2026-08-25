import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { LogsPage } from './LogsPage'

const generation = 'gen_42'

function json(data: unknown, meta: Record<string, unknown> = {}): Response {
  return new Response(JSON.stringify({ data, meta: { generation, ...meta } }), { headers: { 'Content-Type': 'application/json' } })
}

function status() {
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

function catalogResponse(path: string): Response | undefined {
  if (path === '/api/v1/pods?limit=100') return json([{
    namespace: 'payments', name: 'api-abc', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 2, node: 'worker-1', ip: '10.0.0.8', owner: { kind: 'Deployment', name: 'api' }, ageSeconds: 60, problematic: false,
  }], { page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }, coverage: null })
  if (path === '/api/v1/permissions?namespace=payments&capability=pods.logs.get&resourceName=api-abc') return json({
    generation, complete: true, truncated: false, errors: [], decisions: [{ capabilityId: 'pods.logs.get', namespace: 'payments', resourceName: 'api-abc', decision: 'allowed', apiGroup: '', resource: 'pods', subresource: 'log', verb: 'get', reasonCode: 'SAR_ALLOWED', expiresAt: null }],
  })
  if (path === '/api/v1/pods/payments/api-abc') return json({
    metadata: { namespace: 'payments', name: 'api-abc', uid: 'uid-api', resourceVersion: '17', creationTimestamp: '2026-08-17T10:00:00Z', labels: {} },
    summary: { namespace: 'payments', name: 'api-abc', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 2, node: 'worker-1', ip: '10.0.0.8', owner: { kind: 'Deployment', name: 'api' }, ageSeconds: 60, problematic: false },
    conditions: [], containers: [{ spec: { name: 'api', image: 'example/api:1', ports: [] }, type: 'regular', ready: true, restartCount: 2, state: 'running', reason: null }], initContainers: [], ephemeralContainers: [], relatedEvents: [],
  })
  return undefined
}

function renderLogs() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/logs?namespace=payments&pod=api-abc&container=api']}><LogsPage /></MemoryRouter></QueryClientProvider>)
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('bounded log viewer', () => {
  it('reads current logs with generation fencing and keeps content only in memory', async () => {
    let logInit: RequestInit | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json(status()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      const catalog = catalogResponse(path)
      if (catalog) return Promise.resolve(catalog)
      if (path.startsWith('/api/v1/pods/payments/api-abc/logs?')) {
        logInit = init
        return Promise.resolve(json({ container: 'api', previous: false, lines: [{ timestamp: '2026-08-17T10:00:00Z', text: 'sanitized line', truncated: false }], truncated: false }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderLogs()
    const read = await screen.findByRole('button', { name: 'Read logs' })
    await waitFor(() => expect(read).toBeEnabled())
    fireEvent.click(read)

    expect(await screen.findByText('sanitized line')).toBeInTheDocument()
    expect(logInit).toEqual(expect.objectContaining({ cache: 'no-store', credentials: 'same-origin' }))
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })

  it('derives a selectable target catalog from authorized Pod data and exposes partial coverage', async () => {
    let permissionPath = ''
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json(status()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/pods?limit=100') return Promise.resolve(json([
        { namespace: 'payments', name: 'api-abc', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 2, node: 'worker-1', ip: null, owner: null, ageSeconds: 60, problematic: false },
        { namespace: 'payments', name: 'admin', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, node: 'worker-1', ip: null, owner: null, ageSeconds: 60, problematic: false },
      ], { page: { limit: 100, next: 'opaque-next', complete: false, truncated: false, filterScope: 'page' }, coverage: { requestedNamespaces: 2, completedNamespaces: 1, deniedNamespaces: ['restricted'], failed: [] } }))
      if (path.startsWith('/api/v1/permissions?')) {
        permissionPath = path
        return Promise.resolve(json({ generation, complete: true, truncated: false, errors: [], decisions: [
          { capabilityId: 'pods.logs.get', namespace: 'payments', resourceName: 'api-abc', decision: 'allowed' },
          { capabilityId: 'pods.logs.get', namespace: 'payments', resourceName: 'admin', decision: 'denied' },
        ] }))
      }
      const common = catalogResponse(path)
      if (common) return Promise.resolve(common)
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderLogs()

    expect(await screen.findByText(/Partial catalog: 1 log-authorized Pod.*1 denied and 0 unknown/)).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'api-abc' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'admin' })).not.toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Previous container' })).toBeEnabled())
    expect(permissionPath).toContain('capability=pods.logs.get')
    expect(permissionPath).toContain('resourceName=api-abc')
    expect(permissionPath).toContain('resourceName=admin')
  })

  it('follows through fetch plus CSRF and enforces the 1,000-line in-memory ring', async () => {
    let streamCall: [unknown, RequestInit | undefined] | undefined
    const events = [`event: meta\ndata: ${JSON.stringify({ generation, requestId: 'req_1', container: 'api' })}\n\n`]
    for (let index = 0; index < 1_005; index += 1) events.push(`event: line\ndata: ${JSON.stringify({ timestamp: null, text: `line-${index}`, truncated: false })}\n\n`)
    events.push(`event: end\ndata: ${JSON.stringify({ generation, reason: 'limit_reached', truncated: true })}\n\n`)
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json(status()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      const catalog = catalogResponse(path)
      if (catalog) return Promise.resolve(catalog)
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-follow', origin: 'http://127.0.0.1:2748', generation, expiresAt: '2026-08-17T18:00:00Z' }))
      if (path.includes('/logs/stream?')) {
        streamCall = [input, init]
        return Promise.resolve(new Response(new TextEncoder().encode(events.join('')), { headers: { 'Content-Type': 'text/event-stream' } }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))

    renderLogs()
    const follow = await screen.findByRole('button', { name: 'Follow' })
    await waitFor(() => expect(follow).toBeEnabled())
    fireEvent.click(follow)

    expect(await screen.findByText('line-1004')).toBeInTheDocument()
    expect(screen.queryByText('line-0')).not.toBeInTheDocument()
    expect(screen.getByText(/1000 visible of 1000 lines/)).toBeInTheDocument()
    expect(streamCall?.[1]).toEqual(expect.objectContaining({ headers: expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf-follow' }), cache: 'no-store', credentials: 'same-origin' }))
    expect(String(streamCall?.[0])).not.toContain('csrf-follow')
  })

  it('aborts an active follow request when the generation-bound page unmounts', async () => {
    let streamSignal: AbortSignal | undefined
    let responseController: ReadableStreamDefaultController<Uint8Array> | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(json(status()))
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      const catalog = catalogResponse(path)
      if (catalog) return Promise.resolve(catalog)
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-follow', origin: 'http://127.0.0.1:2748', generation, expiresAt: '2026-08-17T18:00:00Z' }))
      if (path.includes('/logs/stream?')) {
        streamSignal = init?.signal as AbortSignal
        const body = new ReadableStream<Uint8Array>({ start(controller) {
          responseController = controller
          controller.enqueue(new TextEncoder().encode(`event: meta\ndata: ${JSON.stringify({ generation, requestId: 'req_1', container: 'api' })}\n\n`))
        } })
        return Promise.resolve(new Response(body, { headers: { 'Content-Type': 'text/event-stream' } }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))

    const view = renderLogs()
    const follow = await screen.findByRole('button', { name: 'Follow' })
    await waitFor(() => expect(follow).toBeEnabled())
    fireEvent.click(follow)
    await waitFor(() => expect(streamSignal).toBeDefined())
    view.unmount()

    expect(streamSignal?.aborted).toBe(true)
    responseController?.close()
  })
})
