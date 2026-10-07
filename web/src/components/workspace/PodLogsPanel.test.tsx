import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { StrictMode } from 'react'
import type { SelectionSummary } from '../../api/types'
import { PodLogsPanel, type PodLogTarget } from './PodLogsPanel'

const selection = { generation: 'gen_logs' } as SelectionSummary
const json = (data: unknown) => new Response(JSON.stringify({ data, meta: { generation: selection.generation } }), { headers: { 'Content-Type': 'application/json' } })
function mockLogs(mode: 'stream' | 'fallback' = 'stream') {
  const signals: AbortSignal[] = [], canceled: string[] = [], paths: string[] = []
  const controllers: ReadableStreamDefaultController<Uint8Array>[] = []
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input); paths.push(path)
    if (path === '/api/v1/session') return json({ csrfToken: 'csrf-logs', generation: selection.generation })
    const match = /^\/api\/v1\/pods\/([^/]+)\/([^/?]+)$/.exec(path)
    if (match) return json({ metadata: { namespace: match[1], name: match[2] }, containers: [{ spec: { name: 'app' } }, { spec: { name: 'sidecar' } }], initContainers: [], ephemeralContainers: [] })
    if (path.includes('/logs/stream')) {
      signals.push(init!.signal as AbortSignal)
      expect(init!.headers).toMatchObject({ 'X-KubePeep-CSRF': 'csrf-logs' })
      if (mode === 'fallback' && signals.length === 1) return new Response('', { status: 503 })
      return new Response(new ReadableStream<Uint8Array>({ start(controller) {
        controllers.push(controller)
        controller.enqueue(new TextEncoder().encode(`event: meta\ndata: {"generation":"gen_logs"}\n\nevent: line\ndata: {"text":"hello from selected Pod"}\n\n`))
      }, cancel() { canceled.push(path) } }), { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (path.includes('/logs?')) return json({ lines: [{ text: 'snapshot', timestamp: null, truncated: false }], truncated: false })
    throw new Error(`Unexpected request: ${path}`)
  }))
  return { paths, signals, canceled, controllers }
}
function open(pods: PodLogTarget[]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<StrictMode><QueryClientProvider client={client}><PodLogsPanel pods={pods} selection={selection} /></QueryClientProvider></StrictMode>)
}
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers() })

it('automatically follows only the exact Pod and cancels old containers and unmounted streams', async () => {
  const mock = mockLogs()
  const view = open([{ namespace: 'payments', name: 'api' }])
  await waitFor(() => expect(screen.getByLabelText('Log output')).toHaveTextContent('hello from selected Pod'))
  expect(mock.paths.every((path) => path === '/api/v1/session' || path.startsWith('/api/v1/pods/payments/api'))).toBe(true)
  expect(mock.paths.some((path) => path.includes('/pods?') || path.includes('/workloads'))).toBe(false)
  fireEvent.change(screen.getByLabelText('Log container'), { target: { value: 'sidecar' } })
  await waitFor(() => expect(mock.paths.some((path) => path.includes('/logs/stream') && path.includes('container=sidecar'))).toBe(true))
  expect(mock.signals[0].aborted).toBe(true)
  view.unmount()
  expect(mock.signals.every((signal) => signal.aborted)).toBe(true)
  await waitFor(() => expect(mock.canceled).toHaveLength(mock.signals.length))
})

it('bounds aggregation to five exact targets and stops sibling streams on a generation mismatch', async () => {
  const mock = mockLogs()
  open(Array.from({ length: 6 }, (_, index) => ({ namespace: `ns-${index}`, name: 'api' })))
  await waitFor(() => expect(mock.controllers).toHaveLength(5))
  expect(screen.getByRole('note')).toHaveTextContent('first 5')
  expect(mock.paths.some((path) => path.includes('/ns-5/'))).toBe(false)
  await act(async () => mock.controllers[0].enqueue(new TextEncoder().encode('event: heartbeat\ndata: {"generation":"other"}\n\n')))
  expect(await screen.findByRole('alert')).toHaveTextContent('generation changed')
  expect(mock.signals.every((signal) => signal.aborted)).toBe(true)
  await waitFor(() => expect(mock.canceled).toHaveLength(5))
})

it('reconnects live streams and keeps receiving while the display is paused', async () => {
  const mock = mockLogs('fallback')
  open([{ namespace: 'payments', name: 'api' }])
  await waitFor(() => expect(screen.getByLabelText('Log output')).toHaveTextContent('hello from selected Pod'))
  expect(mock.signals).toHaveLength(2)
  expect(mock.paths.some((path) => path.includes('/logs?'))).toBe(false)
  expect(screen.getByRole('status')).toHaveTextContent('Live')
  fireEvent.click(screen.getByRole('button', { name: 'Pause' }))
  vi.useFakeTimers()
  await act(async () => {
    mock.controllers[0].enqueue(new TextEncoder().encode('event: line\ndata: {"text":"received while paused"}\n\n'))
    await vi.advanceTimersByTimeAsync(100)
  })
  expect(mock.signals.at(-1)?.aborted).toBe(false)
  expect(screen.getByLabelText('Log output')).not.toHaveTextContent('received while paused')
  fireEvent.click(screen.getByRole('button', { name: 'Resume' }))
  expect(screen.getByLabelText('Log output')).toHaveTextContent('received while paused')
})
