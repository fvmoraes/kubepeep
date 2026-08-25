import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { PodDetail, SelectionSummary, WorkloadDetail } from '../api/types'
import { PodActions, WorkloadActions } from './ResourceActions'

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(status < 400 ? { data } : data), { status, headers: { 'Content-Type': 'application/json' } })
}

const selection: SelectionSummary = { clusterProfileId: 1, context: 'development', cluster: 'dev-cluster', scopeId: 7, scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 1, generation: 'gen_42' }

const workload: WorkloadDetail = {
  metadata: { namespace: 'payments', name: 'api', uid: 'uid-api', resourceVersion: '17', creationTimestamp: '2026-08-17T10:00:00Z', labels: {} },
  kind: 'Deployment', ready: 2, desired: 3, available: 2, updated: 3, status: 'Degraded', selector: { app: 'api' }, restartAt: null, conditions: [], containers: [{ name: 'api', image: 'example/api:1', ports: [] }], related: [],
}

const pod: PodDetail = {
  metadata: { namespace: 'payments', name: 'api-abc', uid: 'uid-pod', resourceVersion: '19', creationTimestamp: '2026-08-17T10:00:00Z', labels: {} },
  summary: { namespace: 'payments', name: 'api-abc', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, node: 'worker-1', ip: '10.0.0.2', owner: { kind: 'ReplicaSet', name: 'api-xyz' }, ageSeconds: 60, problematic: false },
  conditions: [], containers: [{ spec: { name: 'api', image: 'example/api:1', ports: [] }, type: 'regular', ready: true, restartCount: 0, state: 'running', reason: null }], initContainers: [], ephemeralContainers: [], relatedEvents: [],
}

function wrapper(client: QueryClient, child: React.ReactNode) {
  return <QueryClientProvider client={client}>{child}</QueryClientProvider>
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('generation-bound authorized actions', () => {
  it('keeps unknown or denied workload capabilities disabled even after confirmation', async () => {
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path.startsWith('/api/v1/permissions?')) return Promise.resolve(json({ generation: 'gen_42', complete: true, truncated: false, errors: [], decisions: [
        { capabilityId: 'deployments.restart', decision: 'denied' },
        { capabilityId: 'deployments.scale', decision: 'unknown' },
      ] }))
      throw new Error(`Unexpected request: ${path}`)
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(wrapper(client, <WorkloadActions detail={workload} selection={selection} />))

    expect(await screen.findByText(/denied by Kubernetes/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox'))
    expect(screen.getByRole('button', { name: 'Restart Deployment' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Scale' })).toBeDisabled()
  })

  it('sends confirmed restart with CSRF and a fresh idempotency key', async () => {
    let restartInit: RequestInit | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path.startsWith('/api/v1/permissions?')) return Promise.resolve(json({ generation: 'gen_42', complete: true, truncated: false, errors: [], decisions: [
        { capabilityId: 'deployments.restart', decision: 'allowed' }, { capabilityId: 'deployments.scale', decision: 'allowed' },
      ] }))
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-action', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/workloads/deployments/payments/api/restart') {
        restartInit = init
        return Promise.resolve(json({ accepted: true, action: 'restart', target: {}, generation: 'gen_42', resourceVersion: '18' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const invalidateQueries = vi.spyOn(client, 'invalidateQueries')
    render(wrapper(client, <WorkloadActions detail={workload} selection={selection} />))

    await waitFor(() => expect(screen.getByRole('button', { name: 'Restart Deployment' })).toBeDisabled())
    fireEvent.click(screen.getByRole('checkbox'))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Restart Deployment' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'Restart Deployment' }))

    expect(await screen.findByRole('status')).toHaveTextContent('Restart accepted')
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['action-permissions', 'gen_42'] })
    expect(restartInit?.headers).toEqual(expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf-action', 'Idempotency-Key': expect.stringMatching(/^kp-[0-9a-f]{32}$/) }))
    expect(JSON.parse(String(restartInit?.body))).toEqual(expect.objectContaining({ confirmed: true, expectedGeneration: 'gen_42', expectedResourceVersion: '17', consequenceCode: 'RECREATE_WORKLOAD_PODS' }))
  })

  it('refreshes capability hints immediately when execution is denied with 403', async () => {
    let permissionCalls = 0
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path.startsWith('/api/v1/permissions?')) {
        permissionCalls += 1
        const permission = permissionCalls === 1 ? 'allowed' : 'denied'
        return Promise.resolve(json({ generation: 'gen_42', complete: true, truncated: false, errors: [], decisions: [
          { capabilityId: 'deployments.restart', decision: permission }, { capabilityId: 'deployments.scale', decision: permission },
        ] }))
      }
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-action', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/workloads/deployments/payments/api/restart') {
        return Promise.resolve(json({ code: 'ACTION_FORBIDDEN', message: 'Kubernetes denied the action.' }, 403))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const invalidateQueries = vi.spyOn(client, 'invalidateQueries')
    render(wrapper(client, <WorkloadActions detail={workload} selection={selection} />))

    fireEvent.click(screen.getByRole('checkbox'))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Restart Deployment' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'Restart Deployment' }))

    expect(await screen.findByText(/ACTION_FORBIDDEN/)).toBeInTheDocument()
    await waitFor(() => expect(permissionCalls).toBeGreaterThan(1))
    expect(screen.getByRole('button', { name: 'Restart Deployment' })).toBeDisabled()
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['action-permissions', 'gen_42'] })
  })

  it('opens exec only from an ephemeral ticket, echoes heartbeat, and closes on generation change', async () => {
    class MockWebSocket {
      static OPEN = 1
      static instances: MockWebSocket[] = []
      readonly url: string
      readonly protocols: string[]
      readyState = MockWebSocket.OPEN
      binaryType = ''
      onmessage: ((event: MessageEvent) => void) | null = null
      onerror: (() => void) | null = null
      onclose: (() => void) | null = null
      send = vi.fn()
      close = vi.fn()
      constructor(url: string, protocols: string[]) { this.url = url; this.protocols = protocols; MockWebSocket.instances.push(this) }
    }
    vi.stubGlobal('WebSocket', MockWebSocket)
    let currentGeneration = 'gen_42'
    let portForwardInit: RequestInit | undefined
    let execInit: RequestInit | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path.startsWith('/api/v1/permissions?')) return Promise.resolve(json({ generation: currentGeneration, complete: true, truncated: false, errors: [], decisions: [
        { capabilityId: 'pods.delete', decision: 'allowed' }, { capabilityId: 'pods.exec.create', decision: 'allowed' }, { capabilityId: 'pods.portforward.create', decision: 'allowed' },
      ] }))
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-pod', origin: 'http://127.0.0.1:2748', generation: currentGeneration, expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/pods/payments/api-abc/port-forward') {
        portForwardInit = init
        return Promise.resolve(json({ id: 'pf_1', clusterProfileId: 1, context: 'development', generation: currentGeneration, namespace: 'payments', pod: 'api-abc', remotePort: 8080, localAddress: '127.0.0.1', localPort: 49152, status: 'active', createdAt: '2026-08-17T10:00:00Z', expiresAt: '2026-08-17T18:00:00Z', endedAt: null, endReason: null }))
      }
      if (path === '/api/v1/pods/payments/api-abc/exec') {
        execInit = init
        return Promise.resolve(json({ sessionId: 'exec_1', websocketUrl: '/api/v1/exec/exec_1/stream', protocols: ['kubepeep.exec.v1', 'kp-ticket.ephemeral'], expiresAt: '2026-08-17T10:00:10Z' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const view = render(wrapper(client, <PodActions detail={pod} selection={selection} />))

    fireEvent.click(screen.getByRole('checkbox'))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Start port-forward' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'Start port-forward' }))
    expect(await screen.findByText(/Loopback listener:/)).toHaveTextContent('127.0.0.1:49152')
    expect(portForwardInit?.headers).toEqual(expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf-pod', 'Idempotency-Key': expect.stringMatching(/^kp-/) }))

    fireEvent.click(screen.getByRole('button', { name: 'Open exec session' }))
    await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1))
    const socket = MockWebSocket.instances[0]
    expect(socket.url).toBe('/api/v1/exec/exec_1/stream')
    expect(socket.protocols).toEqual(['kubepeep.exec.v1', 'kp-ticket.ephemeral'])
    socket.onmessage?.({ data: JSON.stringify({ type: 'ready', sessionId: 'exec_1', generation: 'gen_42', tty: true, stdin: true }) } as MessageEvent)
    expect(await screen.findByText('Exec session ready.')).toBeInTheDocument()
    socket.onmessage?.({ data: JSON.stringify({ type: 'heartbeat', nonce: 'hb_test' }) } as MessageEvent)
    expect(socket.send).toHaveBeenCalledWith(JSON.stringify({ type: 'heartbeat', nonce: 'hb_test' }))
    expect(JSON.parse(String(execInit?.body))).toEqual(expect.objectContaining({ command: ['/bin/sh'], expectedGeneration: 'gen_42' }))
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)

    currentGeneration = 'gen_43'
    view.rerender(wrapper(client, <PodActions detail={pod} selection={{ ...selection, generation: 'gen_43' }} />))
    expect(socket.close).toHaveBeenCalledWith(1000, 'page_closed')
  })
})
