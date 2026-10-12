import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'
import type { PortForward, SavedPortForward, SelectionSummary } from '../api/types'
import { forwardRecipe, portForwardURL } from '../api/portForwards'
import { PortForwardList } from './PortForwardList'
import { ToastProvider } from './ui/Toast'

const selection: SelectionSummary = { clusterProfileId: 1, context: 'dev', cluster: 'dev', scopeId: 1, scopeName: 'Apps', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'apps', namespaceCount: 1, generation: 'gen' }
const recipe: SavedPortForward = { clusterProfileId: 1, context: 'dev', kind: 'Service', namespace: 'apps', name: 'api', remotePort: 80, localPort: 33155 }
const active: PortForward = { ...recipe, sourceKind: 'Service', sourceName: 'api', sourcePort: 80, generation: 'gen', id: 'pf_1', pod: 'api-pod', remotePort: 8080, localAddress: '127.0.0.1', status: 'active', createdAt: '', expiresAt: '', endedAt: null, endReason: null }
const response = (data: unknown) => new Response(JSON.stringify({ data }), { headers: { 'Content-Type': 'application/json' } })

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

function fixture(initial: PortForward[] = [active]) {
  let sessions = initial
  let preferences = { portForwards: [recipe, { ...recipe, context: 'other' }] }
  const requests: Array<{ path: string; init?: RequestInit }> = []
  const fetch = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input); requests.push({ path, init })
    if (path === '/api/v1/session') return response({ generation: 'gen', csrfToken: 'csrf' })
    if (path === '/api/v1/port-forwards') return response(sessions)
    if (path === '/api/v1/preferences') { if (init?.method === 'PUT') preferences = JSON.parse(String(init.body)); return response(preferences) }
    if (path === '/api/v1/port-forwards/pf_1' && init?.method === 'DELETE') { sessions = []; return new Response(null, { status: 204 }) }
    if (path === '/api/v1/services/apps/api/port-forward') { sessions = [active]; return response(active) }
    throw new Error(`Unexpected request ${path}`)
  })
  vi.stubGlobal('fetch', fetch)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const view = render(<StrictMode><QueryClientProvider client={client}><ToastProvider><PortForwardList selection={selection} target={recipe} /></ToastProvider></QueryClientProvider></StrictMode>)
  return { view, client, requests, saved: () => preferences.portForwards }
}

it('opens the exact active loopback port in the desktop browser and stops without losing its recipe', async () => {
  const state = fixture()
  const BrowserOpenURL = vi.fn(); vi.stubGlobal('runtime', { BrowserOpenURL })
  const link = await screen.findByRole('link', { name: 'Open http://127.0.0.1:33155 in browser' })
  expect(link).toHaveAttribute('href', 'http://127.0.0.1:33155')
  fireEvent.click(link)
  expect(BrowserOpenURL).toHaveBeenCalledWith('http://127.0.0.1:33155')
  fireEvent.click(screen.getByRole('button', { name: 'Stop' }))
  expect(await screen.findByText('Inactive', { exact: true })).toBeInTheDocument()
  expect(state.saved()).toHaveLength(2)
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
  expect(state.requests.find(item => item.init?.method === 'DELETE')?.init?.headers).toEqual(expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf' }))
})

it('restores an inactive Service on the saved local and service ports without auto-starting on mount', async () => {
  const state = fixture([])
  const enable = await screen.findByRole('button', { name: 'Re-enable' })
  expect(state.requests.some(item => item.init?.method === 'POST')).toBe(false)
  fireEvent.click(enable)
  expect(await screen.findByText('Active', { exact: true })).toBeInTheDocument()
  const request = state.requests.find(item => item.init?.method === 'POST')!
  expect(request.path).toBe('/api/v1/services/apps/api/port-forward')
  expect(JSON.parse(String(request.init?.body))).toMatchObject({ localPort: 33155, remotePort: 80, expectedGeneration: 'gen', target: { kind: 'Service', name: 'api' } })
  expect(screen.getAllByRole('button', { name: /^Delete port-forward/ })).toHaveLength(1)
})

it('deletes the live connection and its saved recipe while preserving another context', async () => {
  const state = fixture()
  fireEvent.click(await screen.findByRole('button', { name: 'Delete port-forward for Service api' }))
  await waitFor(() => expect(state.saved()).toEqual([{ ...recipe, context: 'other' }]))
  expect(await screen.findByText('No port forwards for this object.')).toBeInTheDocument()
  const changes = state.requests.filter(item => item.init?.method === 'PUT' || item.init?.method === 'DELETE')
  expect(changes.map(item => item.init?.method)).toEqual(['DELETE', 'PUT'])
})

it('does not offer a browser URL for inactive, non-loopback or invalid ports', () => {
  expect(forwardRecipe(active)).toEqual(recipe)
  expect(portForwardURL({ ...active, status: 'closed' })).toBeNull()
  expect(portForwardURL({ ...active, localPort: 70000 })).toBeNull()
  expect(portForwardURL({ ...active, localAddress: 'example.com' as '127.0.0.1' })).toBeNull()
})
