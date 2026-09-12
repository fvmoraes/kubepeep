import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SettingsPage } from './SettingsPage'

function json(data: unknown): Response {
  return new Response(JSON.stringify({ data }), { headers: { 'Content-Type': 'application/json' } })
}

function preferences() {
  return {
    version: 1,
    ui: { language: 'en' },
    logs: { wrap: false, timestamps: true, tailLines: 200 },
    dashboard: { logScanWindow: '15m', sectionOrder: ['summary', 'problems'], hiddenSections: [] },
    filters: {
      workloads: { version: 1, items: [{ id: 'filter_1', name: 'Degraded only', query: { status: ['Degraded'] } }] },
      pods: { version: 1, items: [] }, events: { version: 1, items: [] }, logs: { version: 1, items: [] },
    },
    shell: { sidebarCompact: true, collapsedGroups: ['workloads'] },
    favorites: { version: 1, items: [{ id: 'fav_1', kind: 'pod', namespace: 'payments', name: 'api-0' }] },
    columns: { hidden: { pods: ['node'] } },
    recent: { version: 1, items: [{ kind: 'pod', namespace: 'payments', name: 'api-0', recordedAt: '2026-09-12T00:00:00Z' }] },
    futureAllowlistedSection: { enabled: true },
  }
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

function queryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
}

describe('allowlisted settings', () => {
  it('updates only owned sections with CSRF and preserves concurrent/future preference fields', async () => {
    let savedBody: unknown
    let savedInit: RequestInit | undefined
    let preferenceReads = 0
    const concurrentPodFilter = { id: 'filter_concurrent', name: 'Running Pods', query: { status: ['Running'] } }
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/preferences' && init?.method !== 'PUT') {
        preferenceReads += 1
        const current = preferences()
        if (preferenceReads > 1) (current.filters.pods.items as Array<typeof concurrentPodFilter>).push(concurrentPodFilter)
        return Promise.resolve(json(current))
      }
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-settings', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/preferences' && init?.method === 'PUT') {
        savedInit = init
        savedBody = JSON.parse(String(init.body))
        return Promise.resolve(json(savedBody))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    render(<QueryClientProvider client={queryClient()}><SettingsPage /></QueryClientProvider>)

    expect(await screen.findByText('Degraded only')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Default tail lines' }), { target: { value: '350' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    expect(await screen.findByRole('status')).toHaveTextContent('Preferences saved transactionally')
    expect(savedBody).toEqual(expect.objectContaining({
      version: 1,
      logs: expect.objectContaining({ tailLines: 350 }),
      filters: expect.objectContaining({
        workloads: { version: 1, items: [] },
        pods: { version: 1, items: [concurrentPodFilter] },
      }),
      shell: preferences().shell,
      favorites: preferences().favorites,
      columns: preferences().columns,
      recent: preferences().recent,
      futureAllowlistedSection: { enabled: true },
    }))
    expect(savedInit).toEqual(expect.objectContaining({
      method: 'PUT',
      headers: expect.objectContaining({ 'Content-Type': 'application/json', 'X-KubePeep-CSRF': 'csrf-settings' }),
    }))
    await waitFor(() => expect(screen.queryByText('Degraded only')).not.toBeInTheDocument())
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })

  it('disables no-op reset/save controls with reasons and restores a changed draft', async () => {
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/preferences') return Promise.resolve(json(preferences()))
      throw new Error(`Unexpected request: ${path}`)
    }))
    render(<QueryClientProvider client={queryClient()}><SettingsPage /></QueryClientProvider>)

    const reset = await screen.findByRole('button', { name: 'Reset unsaved changes' })
    expect(reset).toBeDisabled()
    expect(reset).toHaveAttribute('title', 'There are no unsaved settings to reset.')
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled()

    fireEvent.change(screen.getByRole('spinbutton', { name: 'Default tail lines' }), { target: { value: '350' } })
    expect(reset).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeEnabled()
    fireEvent.click(reset)
    expect(screen.getByRole('spinbutton', { name: 'Default tail lines' })).toHaveValue(200)
    expect(reset).toBeDisabled()
  })

  it('keeps Reset and all editors disabled until a pending PUT settles', async () => {
    let resolvePut!: (response: Response) => void
    let savedBody: unknown
    const pendingPut = new Promise<Response>((resolve) => { resolvePut = resolve })
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/preferences' && init?.method !== 'PUT') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-settings', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/preferences' && init?.method === 'PUT') {
        savedBody = JSON.parse(String(init.body))
        return pendingPut
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    render(<QueryClientProvider client={queryClient()}><SettingsPage /></QueryClientProvider>)

    const tail = await screen.findByRole('spinbutton', { name: 'Default tail lines' })
    fireEvent.change(tail, { target: { value: '350' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    const reset = screen.getByRole('button', { name: 'Reset unsaved changes' })
    await waitFor(() => expect(reset).toBeDisabled())
    expect(reset).toHaveAttribute('title', 'Settings are currently being saved.')
    expect(tail).toBeDisabled()
    fireEvent.click(reset)
    expect(tail).toHaveValue(350)

    resolvePut(json(savedBody))
    expect(await screen.findByRole('status')).toHaveTextContent('Preferences saved transactionally')
    expect(tail).toHaveValue(350)
    expect(reset).toBeDisabled()
  })
})
