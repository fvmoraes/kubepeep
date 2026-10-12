import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { SelectionSummary } from '../api/types'
import { NamespaceScopeSelector } from './NamespaceScopeSelector'

const selection: SelectionSummary = { clusterProfileId: 1, context: 'dev', cluster: 'dev', scopeId: 1, scopeName: 'Apps', scopeMode: 'single', scopeSource: 'saved', defaultNamespace: 'apps', namespaceCount: 1, generation: 'g1' }
const json = (data: unknown, next = '') => new Response(JSON.stringify({ data, meta: { page: { next } } }), { headers: { 'Content-Type': 'application/json' } })
afterEach(() => { cleanup(); vi.unstubAllGlobals() })

it('lists every page for only the active profile/context and publishes a switch before status refresh finishes', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['local-status'], { selection, components: {} })
  let body: unknown
  const fetch = vi.fn(async (input: unknown, init?: RequestInit) => {
    const path = String(input)
    if (path === '/api/v1/namespace-scopes?limit=100') return json([{ id: 1, name: 'Apps', clusterProfileId: 1, context: 'dev' }, { id: 9, name: 'Wrong context', clusterProfileId: 1, context: 'prod' }], 'next')
    if (path.includes('continue=next')) return json([{ id: 2, name: 'Infra', clusterProfileId: 1, context: 'dev' }, { id: 10, name: 'Wrong source', clusterProfileId: 2, context: 'dev' }])
    if (path === '/api/v1/session') return json({ csrfToken: 'csrf', generation: 'g1' })
    if (path === '/api/v1/namespace-scopes/2/select') { body = JSON.parse(String(init?.body)); return json({ ...selection, scopeId: 2, scopeName: 'Infra', defaultNamespace: 'infra', generation: 'g2' }) }
    throw new Error(path)
  })
  vi.stubGlobal('fetch', fetch)
  render(<QueryClientProvider client={client}><NamespaceScopeSelector selection={selection} /></QueryClientProvider>)
  await screen.findByRole('option', { name: 'dev / Infra' })
  expect(screen.getAllByRole('option')).toHaveLength(2)
  fireEvent.change(screen.getByRole('combobox', { name: 'Namespace scope' }), { target: { value: '2' } })
  await waitFor(() => expect(client.getQueryData(['local-status'])).toMatchObject({ selection: { scopeId: 2, defaultNamespace: 'infra', generation: 'g2' } }))
  expect(body).toEqual({ expectedGeneration: 'g1' })
  client.clear()
})

it('keeps the active scope after a failed switch and lets the user retry', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  client.setQueryData(['local-status'], { selection, components: {} })
  let attempts = 0
  vi.stubGlobal('fetch', vi.fn(async (input: unknown) => {
    const path = String(input)
    if (path === '/api/v1/namespace-scopes?limit=100') return json([{ id: 1, name: 'Apps', clusterProfileId: 1, context: 'dev' }, { id: 2, name: 'Infra', clusterProfileId: 1, context: 'dev' }])
    if (path === '/api/v1/session') return json({ csrfToken: 'csrf', generation: 'g1' })
    if (path === '/api/v1/namespace-scopes/2/select') {
      if (++attempts === 1) return new Response(JSON.stringify({ code: 'UNAVAILABLE', message: 'Scope selection is temporarily unavailable.' }), { status: 503, headers: { 'Content-Type': 'application/json' } })
      return json({ ...selection, scopeId: 2, scopeName: 'Infra', generation: 'g2' })
    }
    throw new Error(path)
  }))
  render(<QueryClientProvider client={client}><NamespaceScopeSelector selection={selection} /></QueryClientProvider>)
  await screen.findByRole('option', { name: 'dev / Infra' })
  const select = screen.getByRole('combobox', { name: 'Namespace scope' })
  fireEvent.change(select, { target: { value: '2' } })
  expect(await screen.findByRole('alert')).toHaveTextContent('Scope selection is temporarily unavailable.')
  expect(select).toHaveValue('1')
  expect(select).toBeEnabled()
  expect(client.getQueryData(['local-status'])).toMatchObject({ selection })
  fireEvent.change(select, { target: { value: '2' } })
  await waitFor(() => expect(client.getQueryData(['local-status'])).toMatchObject({ selection: { scopeId: 2, generation: 'g2' } }))
  expect(attempts).toBe(2)
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  client.clear()
})

it('retries an unavailable scope list without changing the active selection', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['local-status'], { selection, components: {} })
  let attempts = 0
  vi.stubGlobal('fetch', vi.fn(async () => {
    if (++attempts === 1) return new Response(JSON.stringify({ code: 'UNAVAILABLE', message: 'Offline' }), { status: 503, headers: { 'Content-Type': 'application/json' } })
    return json([{ id: 2, name: 'Infra', clusterProfileId: 1, context: 'dev' }])
  }))
  render(<QueryClientProvider client={client}><NamespaceScopeSelector selection={selection} /></QueryClientProvider>)
  const retry = await screen.findByRole('button', { name: 'Retry scopes' })
  expect(screen.getByRole('combobox', { name: 'Namespace scope' })).toHaveValue('1')
  fireEvent.click(retry)
  await screen.findByRole('option', { name: 'dev / Infra' })
  expect(screen.queryByRole('button', { name: 'Retry scopes' })).not.toBeInTheDocument()
  expect(client.getQueryData(['local-status'])).toMatchObject({ selection })
  expect(attempts).toBe(2)
  client.clear()
})

it('does not send a scope mutation when the session belongs to another context generation', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['local-status'], { selection, components: {} })
  const fetch = vi.fn(async (input: unknown) => {
    if (String(input) === '/api/v1/namespace-scopes?limit=100') return json([{ id: 2, name: 'Infra', clusterProfileId: 1, context: 'dev' }])
    if (String(input) === '/api/v1/session') return json({ csrfToken: 'rotated', generation: 'other-context' })
    throw new Error(`Unexpected mutation: ${input}`)
  })
  vi.stubGlobal('fetch', fetch)
  render(<QueryClientProvider client={client}><NamespaceScopeSelector selection={selection} /></QueryClientProvider>)
  await screen.findByRole('option', { name: 'dev / Infra' })
  fireEvent.change(screen.getByRole('combobox', { name: 'Namespace scope' }), { target: { value: '2' } })
  expect(await screen.findByRole('alert')).toHaveTextContent('The active selection changed before the action.')
  expect(fetch.mock.calls.map(([path]) => String(path))).toEqual(['/api/v1/namespace-scopes?limit=100', '/api/v1/session'])
  expect(client.getQueryData(['local-status'])).toMatchObject({ selection })
  client.clear()
})

it.each([true, false])('preserves a newer context when a scope response arrives late (unmounted: %s)', async shouldUnmount => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['local-status'], { selection, components: {} })
  let requestSignal: AbortSignal | null | undefined
  let respond: (response: Response) => void = () => { throw new Error('Scope request did not start') }
  const pending = new Promise<Response>(resolve => { respond = resolve })
  vi.stubGlobal('fetch', vi.fn(async (input: unknown, init?: RequestInit) => {
    const path = String(input)
    if (path === '/api/v1/namespace-scopes?limit=100') return json([{ id: 2, name: 'Infra', clusterProfileId: 1, context: 'dev' }])
    if (path === '/api/v1/session') return json({ csrfToken: 'csrf', generation: 'g1' })
    if (path === '/api/v1/namespace-scopes/2/select') { requestSignal = init?.signal; return pending }
    throw new Error(path)
  }))
  const { unmount } = render(<QueryClientProvider client={client}><NamespaceScopeSelector selection={selection} /></QueryClientProvider>)
  await screen.findByRole('option', { name: 'dev / Infra' })
  fireEvent.change(screen.getByRole('combobox', { name: 'Namespace scope' }), { target: { value: '2' } })
  await waitFor(() => expect(requestSignal).toBeDefined())
  if (shouldUnmount) {
    unmount()
    expect(requestSignal?.aborted).toBe(true)
  }
  const newer = { ...selection, context: 'prod', generation: 'g3' }
  client.setQueryData(['local-status'], { selection: newer, components: {} })
  await act(async () => { respond(json({ ...selection, scopeId: 2, generation: 'g2' })); await pending })
  expect(client.getQueryData(['local-status'])).toMatchObject({ selection: newer })
  client.clear()
})
