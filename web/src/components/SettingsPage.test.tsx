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
  }
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('allowlisted settings', () => {
  it('updates the complete schema with CSRF and removes saved filters without browser persistence', async () => {
    let savedBody: unknown
    let savedInit: RequestInit | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/preferences' && init?.method !== 'PUT') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-settings', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/preferences' && init?.method === 'PUT') {
        savedInit = init
        savedBody = JSON.parse(String(init.body))
        return Promise.resolve(json(savedBody))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(<QueryClientProvider client={client}><SettingsPage /></QueryClientProvider>)

    expect(await screen.findByText('Degraded only')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Default tail lines' }), { target: { value: '350' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    expect(await screen.findByRole('status')).toHaveTextContent('Preferences saved transactionally')
    expect(savedBody).toEqual(expect.objectContaining({
      version: 1,
      logs: expect.objectContaining({ tailLines: 350 }),
      filters: expect.objectContaining({ workloads: { version: 1, items: [] } }),
    }))
    expect(savedInit).toEqual(expect.objectContaining({
      method: 'PUT',
      headers: expect.objectContaining({ 'Content-Type': 'application/json', 'X-KubePeep-CSRF': 'csrf-settings' }),
    }))
    await waitFor(() => expect(screen.queryByText('Degraded only')).not.toBeInTheDocument())
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })
})
