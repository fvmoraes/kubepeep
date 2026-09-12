import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AccessControlPage, AdministrationPage } from './AccessPages'
import { GlobalNamespaceProvider } from '../context/GlobalNamespace'
import { ResourceWorkspaceProvider } from './workspace/ResourceWorkspaceProvider'

const generation = 'gen_access'

function json(data: unknown, meta: Record<string, unknown> = {}): Response {
  return new Response(JSON.stringify({ data, meta: { generation, ...meta } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function selectedStatus() {
  return {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: 'healthy', code: 'TEST', message: 'test', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'dev-cluster', scopeId: 7, scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 1, generation },
  }
}

function page() {
  return { page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'page' }, coverage: null }
}

function renderPage(element: React.ReactNode, path: string, route: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const selection = selectedStatus().selection
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <ResourceWorkspaceProvider>
          <GlobalNamespaceProvider generation={selection.generation} scopeId={selection.scopeId} scopeMode={selection.scopeMode}>
            <Routes><Route path={route} element={element} /></Routes>
          </GlobalNamespaceProvider>
        </ResourceWorkspaceProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('Access and administration list controls', () => {
  it.each([
    { label: 'Access Control', path: '/access/roles', route: '/access/:tab', endpoint: '/api/v1/roles?', element: <AccessControlPage /> },
    { label: 'Administration', path: '/administration/customresourcedefinitions', route: '/administration/:tab', endpoint: '/api/v1/customresourcedefinitions?', element: <AdministrationPage /> },
  ])('applies and clears search and ordering for $label', async ({ path, route, endpoint, element }) => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const requested = String(input)
      paths.push(requested)
      if (requested === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (requested === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (requested.startsWith(endpoint)) return Promise.resolve(json([], page()))
      throw new Error(`Unexpected request: ${requested}`)
    }))

    renderPage(element, path, route)
    await waitFor(() => expect(paths.some((requested) => requested.startsWith(endpoint))).toBe(true))
    expect(screen.getByRole('button', { name: 'Apply filters' })).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Search this bounded page'), { target: { value: 'backend' } })
    fireEvent.change(screen.getByLabelText('Sort this bounded page'), { target: { value: 'name' } })
    fireEvent.change(screen.getByLabelText('Order'), { target: { value: 'desc' } })
    expect(screen.getByRole('button', { name: 'Apply filters' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))

    await waitFor(() => expect(paths.some((requested) => {
      if (!requested.startsWith(endpoint)) return false
      const query = new URL(requested, 'http://127.0.0.1').searchParams
      return query.get('search') === 'backend' && query.get('sort') === 'name' && query.get('order') === 'desc'
    })).toBe(true))
    expect(screen.queryByText('Filter changes pending; apply filters to update the bounded result.')).not.toBeInTheDocument()

    const beforeClear = paths.length
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    await waitFor(() => expect(paths.slice(beforeClear).some((requested) => {
      if (!requested.startsWith(endpoint)) return false
      const query = new URL(requested, 'http://127.0.0.1').searchParams
      return !query.has('search') && !query.has('sort') && !query.has('order')
    })).toBe(true))
    expect(screen.getByLabelText('Search this bounded page')).toHaveValue('')
  })
})
