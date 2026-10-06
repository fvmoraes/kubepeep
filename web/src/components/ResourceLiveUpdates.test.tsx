import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Profiler } from 'react'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ResourceLiveUpdates } from './ResourceLiveUpdates'

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(status < 400 ? { data } : data), { status, headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('automatic resource SSE', () => {
  it('uses fetch with CSRF, invalidates HTTP snapshots, and aborts on unmount', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    let streamSignal: AbortSignal | undefined
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-live', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/stream?topic=pods') {
        streamSignal = init?.signal as AbortSignal
        const body = new ReadableStream<Uint8Array>({ start(controller) {
          controller.enqueue(new TextEncoder().encode('event: snapshot\ndata: {"generation":"gen_42","final":true}\n\nevent: modified\ndata: {"generation":"gen_42"}\n\n'))
        } })
        return Promise.resolve(new Response(body, { headers: { 'Content-Type': 'text/event-stream; charset=utf-8' } }))
      }
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetch)

    const view = render(<QueryClientProvider client={client}><ResourceLiveUpdates generation="gen_42" topics={['pods']} queryKeys={[["resources", "pods"]]} /></QueryClientProvider>)

    expect(await screen.findByText('Live')).toBeInTheDocument()
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ['resources', 'pods'] })))
    const streamCall = fetch.mock.calls.find(([input]) => String(input).startsWith('/api/v1/stream'))
    expect(streamCall?.[1]).toEqual(expect.objectContaining({
      cache: 'no-store', credentials: 'same-origin',
      headers: expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf-live', Accept: 'text/event-stream' }),
    }))
    expect(String(streamCall?.[0])).not.toContain('csrf-live')

    view.unmount()
    expect(streamSignal?.aborted).toBe(true)
  })

  it('shows automatic HTTP fallback when watch authorization is unavailable', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-live', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/stream?topic=events') return Promise.resolve(json({ code: 'AUTHORIZATION_UNAVAILABLE', message: 'Authorization could not be confirmed.' }, 503))
      throw new Error(`Unexpected request: ${path}`)
    }))

    render(<QueryClientProvider client={client}><ResourceLiveUpdates generation="gen_42" topics={['events']} queryKeys={[["resources", "events"]]} /></QueryClientProvider>)

    expect(await screen.findByText('Auto · 15s')).toBeInTheDocument()
    expect(invalidate).not.toHaveBeenCalled()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('coalesces 10k watch deltas into one bounded HTTP refresh', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    let commits = 0
    let renderCPU = 0
    let responseController: ReadableStreamDefaultController<Uint8Array> | undefined
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/session') return Promise.resolve(json({ csrfToken: 'csrf-live', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-17T18:00:00Z' }))
      if (path === '/api/v1/stream?topic=pods') {
        const body = new ReadableStream<Uint8Array>({ start(controller) { responseController = controller } })
        return Promise.resolve(new Response(body, { headers: { 'Content-Type': 'text/event-stream' } }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))

    const view = render(<QueryClientProvider client={client}><Profiler id="live-updates" onRender={(_id, _phase, actualDuration) => { commits += 1; renderCPU += actualDuration }}><ResourceLiveUpdates generation="gen_42" topics={['pods']} queryKeys={[["resources", "pods"]]} /></Profiler></QueryClientProvider>)
    expect(await screen.findByText('Live')).toBeInTheDocument()
    const commitsBeforeBurst = commits
    const renderCPUBeforeBurst = renderCPU
    vi.useFakeTimers()

    const event = 'event: modified\ndata: {"generation":"gen_42"}\n\n'
    for (let batch = 0; batch < 10; batch += 1) {
      responseController?.enqueue(new TextEncoder().encode(event.repeat(1_000)))
    }
    await vi.advanceTimersByTimeAsync(0)
    expect(invalidate).not.toHaveBeenCalled()
    await act(async () => { await vi.advanceTimersByTimeAsync(150) })
    expect(screen.getByLabelText('Resource live updates')).toBeInTheDocument()
    expect(commits - commitsBeforeBurst).toBeLessThan(6)
    expect(renderCPU - renderCPUBeforeBurst).toBeLessThan(500)
    await vi.advanceTimersByTimeAsync(1_849)
    expect(invalidate).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(invalidate).toHaveBeenCalledTimes(1)

    view.unmount()
  })
})
