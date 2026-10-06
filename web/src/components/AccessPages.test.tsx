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
  ])('automatically applies and clears string search for $label', async ({ path, route, endpoint, element }) => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const requested = String(input)
      paths.push(requested)
      if (requested === '/api/v1/preferences') return Promise.resolve(json({ columns: { hidden: {} } }))
      if (requested === '/api/v1/status') return Promise.resolve(json(selectedStatus()))
      if (requested === '/api/v1/namespace-scopes/7') return Promise.resolve(json({ namespaces: ['payments'] }))
      if (requested.startsWith(endpoint)) {
        const filtered = new URL(requested, 'http://127.0.0.1').searchParams.has('search')
        return Promise.resolve(json(filtered ? [] : [{ name: 'original-resource', namespace: 'payments', ruleCount: 0, ageSeconds: 60, group: 'example.test', kind: 'Example', scope: 'Namespaced', versions: [] }], page()))
      }
      throw new Error(`Unexpected request: ${requested}`)
    }))

    renderPage(element, path, route)
    await waitFor(() => expect(paths.some((requested) => requested.startsWith(endpoint))).toBe(true))

    fireEvent.change(screen.getByLabelText('Search resources'), { target: { value: 'backend' } })

    await waitFor(() => expect(paths.some((requested) => {
      if (!requested.startsWith(endpoint)) return false
      const query = new URL(requested, 'http://127.0.0.1').searchParams
      return query.get('search') === 'backend' && !query.has('sort') && !query.has('order')
    })).toBe(true))
    expect(screen.queryByText('Filter changes pending; apply filters to update the bounded result.')).not.toBeInTheDocument()

    expect(await screen.findByText('No matching resources')).toBeInTheDocument()
    const beforeClear = paths.length
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: '' } })
    expect(await screen.findByText('original-resource')).toBeInTheDocument()
    // Clearing search reuses the still-fresh first page, just like Pods.
    expect(paths).toHaveLength(beforeClear)
    expect(screen.getByLabelText('Search resources')).toHaveValue('')
  })
})
