import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { App } from './App'

const placeholderRoutes = [
  ['/workloads', 'Workloads'],
  ['/pods', 'Pods'],
  ['/logs', 'Logs'],
  ['/events', 'Events'],
  ['/network', 'Network'],
  ['/config', 'Config'],
  ['/settings', 'Settings'],
] as const

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(status < 400 ? { data } : data), { status, headers: { 'Content-Type': 'application/json' } })
}

function localStatus() {
  return {
    version: 'test', commit: 'unknown', buildDate: 'unknown', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((key) => [key, { status: key === 'application' ? 'healthy' : 'unknown', code: 'TEST', message: 'test', checkedAt: null }])),
    selection: null,
  }
}

function renderApp(path = '/') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('application shell', () => {
  it('renders the accessible loading state while bootstrap is pending', () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => undefined)))
    renderApp()

    const heading = screen.getByRole('heading', { name: 'Preparing the local workspace' })
    const panel = heading.closest('section')
    expect(panel).toHaveAttribute('aria-live', 'polite')
    expect(panel).toHaveAttribute('aria-busy', 'true')
  })

  it('shows the empty context state without persisting remote data', async () => {
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      if (String(input) === '/api/v1/cluster/profiles') {
        return Promise.resolve(json([]))
      }
      return Promise.resolve(json(localStatus()))
    }))

    renderApp()

    expect(await screen.findByRole('heading', { name: 'Choose a Kubernetes context' })).toBeInTheDocument()
    expect(screen.getByRole('navigation', { name: 'Primary navigation' })).toBeInTheDocument()
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })

  it('renders an offline state when the local API cannot be reached', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('offline')))
    renderApp()
    expect(await screen.findByRole('heading', { name: 'The local API is unavailable' })).toBeInTheDocument()
  })

  it('updates the header and overview from the active in-memory selection', async () => {
    const selected = {
      ...localStatus(),
      selection: {
        clusterProfileId: 1, context: 'development', cluster: 'dev-cluster', scopeId: 7,
        scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments',
        namespaceCount: 3, generation: 'gen_42',
      },
    }
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => Promise.resolve(
      String(input) === '/api/v1/cluster/profiles' ? json([]) : json(selected),
    )))
    renderApp()

    expect(await screen.findByRole('heading', { name: 'Cluster overview' })).toBeInTheDocument()
    expect(screen.getByText('development · Finance')).toBeInTheDocument()
    expect(screen.getByText('dev-cluster · 3 namespaces')).toBeInTheDocument()
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })

  it('renders a distinct error state when the local API rejects the request', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(json({
      code: 'INTERNAL',
      message: 'Internal server error.',
      requestId: 'req_test',
    }, 500))))
    renderApp()

    expect(await screen.findByRole('heading', { name: 'The local API returned an error' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'The local API is unavailable' })).not.toBeInTheDocument()
  })

  it('renders every placeholder route on a direct deep link', () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('offline')))
    for (const [path, title] of placeholderRoutes) {
      const view = renderApp(path)
      expect(screen.getByRole('heading', { name: title })).toBeInTheDocument()
      view.unmount()
    }
  })

  it('renders the explicit not-found error route', () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('offline')))
    renderApp('/not-a-kubepeep-route')

    const heading = screen.getByRole('heading', { name: 'Page not found' })
    expect(heading.closest('section')).toHaveAttribute('aria-live', 'assertive')
    expect(screen.getByRole('link', { name: 'Overview' })).toBeInTheDocument()
  })
})
