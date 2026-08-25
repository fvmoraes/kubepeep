import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ResourceLiveUpdates } from './ResourceLiveUpdates'

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(status < 400 ? { data } : data), { status, headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('optional resource SSE', () => {
  it('uses fetch with CSRF, invalidates HTTP snapshots, and aborts on unmount', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    let responseController: ReadableStreamDefaultController<Uint8Array> | undefined
    let streamSignal: AbortSignal | undefined
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-live', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/stream?topic=pods') {
        streamSignal = init?.signal as AbortSignal
        const body = new ReadableStream<Uint8Array>({ start(controller) {
          responseController = controller
          controller.enqueue(new TextEncoder().encode('event: snapshot\ndata: {"generation":"gen_42","final":true}\n\nevent: modified\ndata: {"generation":"gen_42"}\n\n'))
        } })
        return Promise.resolve(new Response(body, { headers: { 'Content-Type': 'text/event-stream; charset=utf-8' } }))
      }
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetch)

    const view = render(<QueryClientProvider client={client}><ResourceLiveUpdates generation="gen_42" topics={['pods']} queryKeys={[["resources", "pods"]]} /></QueryClientProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Start live updates' }))

    expect(await screen.findByText(/Live updates active for pods/)).toBeInTheDocument()
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ['resources', 'pods'] })))
    const streamCall = fetch.mock.calls.find(([input]) => String(input).startsWith('/api/v1/stream'))
    expect(streamCall?.[1]).toEqual(expect.objectContaining({
      cache: 'no-store', credentials: 'same-origin',
      headers: expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf-live', Accept: 'text/event-stream' }),
    }))
    expect(String(streamCall?.[0])).not.toContain('csrf-live')

    view.unmount()
    expect(streamSignal?.aborted).toBe(true)
    responseController?.close()
  })

  it('falls back to explicit refresh without starting implicit polling when watch authorization is unavailable', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const setInterval = vi.spyOn(window, 'setInterval')
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-live', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/stream?topic=events') return Promise.resolve(json({ code: 'AUTHORIZATION_UNAVAILABLE', message: 'Authorization could not be confirmed.' }, 503))
      throw new Error(`Unexpected request: ${path}`)
    }))

    render(<QueryClientProvider client={client}><ResourceLiveUpdates generation="gen_42" topics={['events']} queryKeys={[["resources", "events"]]} /></QueryClientProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Start live updates' }))

    expect(await screen.findByText(/Automatic polling is disabled/)).toBeInTheDocument()
    expect(invalidate).not.toHaveBeenCalled()
    expect(setInterval).not.toHaveBeenCalledWith(expect.any(Function), 15_000)
    expect(screen.getByRole('button', { name: 'Retry live updates' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh now' }))
    await waitFor(() => expect(invalidate).toHaveBeenCalledTimes(1))
  })
})
