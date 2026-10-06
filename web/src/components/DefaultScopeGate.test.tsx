import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router'

import type { NamespaceScope, SelectionSummary } from '../api/types'
import { DefaultScopeGate } from './DefaultScopeGate'

const selection: SelectionSummary = {
  clusterProfileId: 7,
  context: 'development',
  cluster: 'dev-cluster',
  scopeId: null,
  scopeName: null,
  scopeMode: null,
  scopeSource: 'none',
  defaultNamespace: null,
  namespaceCount: 0,
  generation: 'gen_42',
}

const defaultScope: NamespaceScope = {
  id: 9,
  clusterProfileId: 7,
  context: 'development',
  name: 'Finance',
  mode: 'list',
  namespaces: ['payments', 'billing'],
  defaultNamespace: 'payments',
  isDefault: true,
  version: 1,
  createdAt: '2026-09-29T12:00:00Z',
  updatedAt: '2026-09-29T12:00:00Z',
}

function response(data: unknown): Response {
  return new Response(JSON.stringify({ data }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function renderGate(children = <div>Protected resources</div>) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter><DefaultScopeGate selection={selection}>{children}</DefaultScopeGate></MemoryRouter></QueryClientProvider>)
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('default scope gate', () => {
  it('does not mount resource children before the active selection is known', () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/pods']}><DefaultScopeGate selection={null} selectionPending><div>Protected resources</div></DefaultScopeGate></MemoryRouter></QueryClientProvider>)

    expect(screen.getByRole('status', { name: 'Loading the active context' })).toBeInTheDocument()
    expect(screen.queryByText('Protected resources')).not.toBeInTheDocument()
  })

  it('activates the persisted default before exposing resources', async () => {
    let selectBody: unknown
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/namespace-scopes?limit=100') return Promise.resolve(response([defaultScope]))
      if (path === '/api/v1/session') return Promise.resolve(response({ csrfToken: 'csrf-next', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-09-29T13:00:00Z' }))
      if (path === '/api/v1/namespace-scopes/9/select') {
        selectBody = JSON.parse(String(init?.body))
        return Promise.resolve(response({ ...selection, scopeId: 9, scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved', namespaceCount: 2, generation: 'gen_43' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetch)
    renderGate()

    expect(screen.queryByText('Protected resources')).not.toBeInTheDocument()
    await waitFor(() => expect(selectBody).toEqual({ expectedGeneration: 'gen_42' }))
  })

  it('keeps resource children unmounted until local status confirms the active scope', async () => {
    const protectedResources = vi.fn(() => <div>Protected resources</div>)
    const ProtectedResources = protectedResources
    const selectScope = vi.fn()
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/namespace-scopes?limit=100') return Promise.resolve(response([defaultScope]))
      if (path === '/api/v1/session') return Promise.resolve(response({ csrfToken: 'csrf-next', generation: selection.generation }))
      if (path === '/api/v1/namespace-scopes/9/select') {
        selectScope()
        return Promise.resolve(response({ ...selection, scopeId: 9, generation: 'gen_43' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderGate(<ProtectedResources />)

    await waitFor(() => expect(selectScope).toHaveBeenCalledOnce())
    await waitFor(() => expect(screen.getByRole('status', { name: 'Activating the default scope' })).toBeInTheDocument())
    expect(protectedResources).not.toHaveBeenCalled()
  })

  it.each(['session', 'activation'])('retries a failed %s with a fresh session', async (failure) => {
    let sessionRequests = 0
    const activated = vi.fn()
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/namespace-scopes?limit=100') return Promise.resolve(response([defaultScope]))
      if (path === '/api/v1/session') {
        sessionRequests += 1
        if (failure === 'session' && sessionRequests === 1) return Promise.reject(new Error('Session temporarily unavailable'))
        return Promise.resolve(response({ csrfToken: `csrf-${sessionRequests}`, generation: selection.generation }))
      }
      if (path === '/api/v1/namespace-scopes/9/select') {
        if (sessionRequests < 2) return Promise.reject(new Error('Session expired'))
        activated()
        return Promise.resolve(response({ ...selection, scopeId: 9, generation: 'gen_43' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderGate()

    fireEvent.click(await screen.findByRole('button', { name: 'Retry activation' }))
    await waitFor(() => expect(activated).toHaveBeenCalledOnce())
    expect(screen.queryByText('Protected resources')).not.toBeInTheDocument()
  })

  it('requires an explicit default instead of assuming all namespaces', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response([{ ...defaultScope, isDefault: false }])))
    renderGate()

    expect(await screen.findByRole('heading', { name: 'Choose a default namespace scope' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Choose Default Scope' })).toBeInTheDocument()
    expect(screen.queryByText('Protected resources')).not.toBeInTheDocument()
  })

  it('finds the default beyond the first page of saved scopes', async () => {
    const selected = vi.fn()
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/namespace-scopes?limit=100') {
        return Promise.resolve(new Response(JSON.stringify({
          data: [{ ...defaultScope, id: 1, isDefault: false }],
          meta: { page: { next: 'next-page' } },
        }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      }
      if (path === '/api/v1/namespace-scopes?limit=100&continue=next-page') return Promise.resolve(response([defaultScope]))
      if (path === '/api/v1/session') return Promise.resolve(response({ csrfToken: 'csrf-next', generation: selection.generation }))
      if (path === '/api/v1/namespace-scopes/9/select') {
        selected()
        return Promise.resolve(response({ ...selection, scopeId: 9, generation: 'gen_43' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    renderGate()

    await waitFor(() => expect(selected).toHaveBeenCalledOnce())
    expect(screen.queryByText('Protected resources')).not.toBeInTheDocument()
  })

  it('loads each context generation with its own persisted default', async () => {
    const selectedIDs: number[] = []
    let scopeRequest = 0
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/namespace-scopes?limit=100') {
        scopeRequest += 1
        const scope = scopeRequest === 1 ? defaultScope : { ...defaultScope, id: 10, context: 'production', name: 'Production', namespaces: ['prod'], defaultNamespace: 'prod' }
        return Promise.resolve(response([scope]))
      }
      if (path === '/api/v1/session') return Promise.resolve(response({ csrfToken: 'csrf-next', origin: 'http://127.0.0.1:2748', generation: scopeRequest === 1 ? 'gen_42' : 'gen_44', expiresAt: '2026-09-29T13:00:00Z' }))
      if (/\/api\/v1\/namespace-scopes\/\d+\/select/.test(path)) {
        selectedIDs.push(Number(path.split('/')[4]))
        return Promise.resolve(response({ ...selection, generation: scopeRequest === 1 ? 'gen_43' : 'gen_45' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const view = render(<QueryClientProvider client={client}><MemoryRouter><DefaultScopeGate selection={selection}><div>Protected resources</div></DefaultScopeGate></MemoryRouter></QueryClientProvider>)
    await waitFor(() => expect(selectedIDs).toEqual([9]))

    const production = { ...selection, context: 'production', cluster: 'prod-cluster', generation: 'gen_44' }
    view.rerender(<QueryClientProvider client={client}><MemoryRouter><DefaultScopeGate selection={production}><div>Protected resources</div></DefaultScopeGate></MemoryRouter></QueryClientProvider>)
    await waitFor(() => expect(selectedIDs).toEqual([9, 10]))
  })
})
