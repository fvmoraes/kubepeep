import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { applyColumnVisibility, ColumnVisibilityControl, usePreferenceColumnVisibility } from './columns'

function json(data: unknown): Response {
  return new Response(JSON.stringify({ data }), { headers: { 'Content-Type': 'application/json' } })
}

function preferences() {
  const empty = { version: 1, items: [] }
  return {
    version: 1,
    ui: { language: 'en' },
    logs: { wrap: false, timestamps: true, tailLines: 200 },
    dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] },
    filters: { workloads: empty, pods: empty, events: empty, logs: empty },
    columns: { hidden: {} as Record<string, string[]> },
  }
}

function Harness({ collectionId = 'pods' }: { collectionId?: string }) {
  const state = usePreferenceColumnVisibility(collectionId)
  const columns = [{ key: 'name' }, { key: 'status' }, { key: 'age' }]
  const visible = applyColumnVisibility(columns, state)
  return <section data-testid={`columns-${collectionId}`}><ColumnVisibilityControl state={state} columns={columns} /><output aria-label={`Visible columns ${collectionId}`}>{visible.map((column) => column.key).join(',')}</output></section>
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function queryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
}

describe('column visibility', () => {
  it('keeps hidden columns in the chooser and reports persistence failures without reverting the local click', async () => {
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/preferences' && init?.method !== 'PUT') return Promise.resolve(json(preferences()))
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf', generation: 'gen', origin: 'http://127.0.0.1:2748', expiresAt: '2026-09-12T12:00:00Z' }))
      if (path === '/api/v1/preferences' && init?.method === 'PUT') return Promise.reject(new Error('disk unavailable'))
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    render(<QueryClientProvider client={queryClient()}><Harness /></QueryClientProvider>)

    fireEvent.click(screen.getByRole('button', { name: 'Choose visible columns' }))
    const status = await screen.findByRole('checkbox', { name: 'status' })
    fireEvent.click(status)
    expect(screen.getByLabelText('Visible columns pods')).toHaveTextContent('name,age')
    expect(screen.getByRole('checkbox', { name: 'status' })).not.toBeChecked()
    expect(await screen.findByRole('alert')).toHaveTextContent('changed locally')

    fireEvent.click(screen.getByRole('checkbox', { name: 'status' }))
    expect(screen.getByLabelText('Visible columns pods')).toHaveTextContent('name,status,age')
    await waitFor(() => expect(fetchMock.mock.calls.filter(([, init]) => init?.method === 'PUT')).toHaveLength(2))
  })

  it('serializes concurrent collection writers and changes only each hidden collection key', async () => {
    let stored = preferences()
    const putBodies: ReturnType<typeof preferences>[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/preferences' && init?.method !== 'PUT') return Promise.resolve(json(structuredClone(stored)))
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf', generation: 'gen', origin: 'http://127.0.0.1:2748', expiresAt: '2026-09-12T12:00:00Z' }))
      if (path === '/api/v1/preferences' && init?.method === 'PUT') {
        stored = JSON.parse(String(init.body)) as ReturnType<typeof preferences>
        putBodies.push(structuredClone(stored))
        return Promise.resolve(json(stored))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    render(<QueryClientProvider client={queryClient()}><Harness collectionId="pods" /><Harness collectionId="services" /></QueryClientProvider>)

    const pods = within(screen.getByTestId('columns-pods'))
    const services = within(screen.getByTestId('columns-services'))
    fireEvent.click(pods.getByRole('button', { name: 'Choose visible columns' }))
    fireEvent.click(await pods.findByRole('checkbox', { name: 'status' }))
    fireEvent.click(services.getByRole('button', { name: 'Choose visible columns' }))
    fireEvent.click(await services.findByRole('checkbox', { name: 'age' }))

    await waitFor(() => expect(putBodies).toHaveLength(2))
    expect(putBodies[0].columns.hidden).toEqual({ pods: ['status'] })
    expect(putBodies[1].columns.hidden).toEqual({ pods: ['status'], services: ['age'] })
    expect(stored.columns.hidden).toEqual({ pods: ['status'], services: ['age'] })
  })
})
