import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SavedFilterControls } from './SavedFilterControls'

const generation = 'gen_42'

function json(data: unknown): Response {
  return new Response(JSON.stringify({ data }), { headers: { 'Content-Type': 'application/json' } })
}

function preferences() {
  const empty = { version: 1 as const, items: [] }
  return {
    version: 1 as const,
    ui: { language: 'en' as const },
    logs: { wrap: false, timestamps: true, tailLines: 200 },
    dashboard: { logScanWindow: '15m' as const, sectionOrder: ['summary'], hiddenSections: [] },
    filters: {
      workloads: empty,
      pods: { version: 1 as const, items: [{ id: 'existing', name: 'Worker failures', query: { namespace: ['payments'], status: ['Failed'], node: 'worker-1' } }] },
      events: empty,
      logs: empty,
    },
  }
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('saved filter controls', () => {
  it('applies an existing allowlisted query and saves the current bounded query transactionally', async () => {
    const applied = vi.fn()
    let putBody: ReturnType<typeof preferences> | undefined
    let putInit: RequestInit | undefined
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/preferences' && (init?.method ?? 'GET') === 'GET') return json(preferences())
      if (path === '/api/v1/session') return json({ csrfToken: 'csrf-filter', origin: 'http://127.0.0.1:2748', generation, expiresAt: '2026-08-17T18:00:00Z' })
      if (path === '/api/v1/preferences' && init?.method === 'PUT') {
        putInit = init
        putBody = JSON.parse(String(init.body)) as ReturnType<typeof preferences>
        return json(putBody)
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(<QueryClientProvider client={client}><SavedFilterControls collection="pods" generation={generation} currentQuery={{ namespace: ['payments'], search: 'backend', workload: 'api', node: 'worker-2', problematic: true }} onApply={applied} /></QueryClientProvider>)

    await screen.findByRole('option', { name: 'Worker failures' })
    fireEvent.change(screen.getByLabelText('Saved filter'), { target: { value: 'existing' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply saved filter' }))
    expect(applied).toHaveBeenCalledWith({ namespace: ['payments'], status: ['Failed'], node: 'worker-1' })

    fireEvent.change(screen.getByLabelText('Save current filter as'), { target: { value: 'Backend Pods' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save current filter' }))
    expect(await screen.findByText('Current bounded filter saved.')).toBeInTheDocument()

    expect(putInit?.headers).toEqual(expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf-filter', 'Content-Type': 'application/json' }))
    const saved = putBody?.filters.pods.items.at(-1)
    expect(saved?.name).toBe('Backend Pods')
    expect(saved?.query).toEqual({ namespace: ['payments'], search: 'backend', workload: 'api', node: 'worker-2', problematic: true })
    expect(saved?.query).not.toHaveProperty('continue')
    expect(saved?.query).not.toHaveProperty('limit')
    await waitFor(() => expect(screen.getByRole('option', { name: 'Backend Pods' })).toBeInTheDocument())
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })
})
