import { afterEach, expect, it, vi } from 'vitest'
import { consumeAggregateStream } from './podLogStream'

vi.mock('../../api/desktop', () => ({ streamURL: async (path: string) => path }))
afterEach(() => vi.unstubAllGlobals())
const target = { namespace: 'payments', pod: 'api', container: 'main' }
const options = { timestamps: true, tailLines: 200, since: '' }
const event = (type: string, payload: unknown) => `event: ${type}\ndata: ${JSON.stringify(payload)}\n\n`
const meta = event('meta', { generation: 'gen' })
function response(body: string) { return new Response(body, { headers: { 'Content-Type': 'text/event-stream' } }) }

it('parses large transport chunks without mistaking many bounded events for an oversized event', async () => {
  const body = meta + Array.from({ length: 3000 }, (_, index) => event('line', { text: `line ${index} ${'x'.repeat(60)}` })).join('') + event('end', { reason: 'completed', generation: 'gen', truncated: false })
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(body)))
  const lines = vi.fn()
  const terminal = await consumeAggregateStream(target, { generation: 'gen' }, { csrfToken: 'csrf' }, options, new AbortController().signal, lines)
  expect(lines).toHaveBeenCalledTimes(3000)
  expect(terminal).toEqual({ reason: 'completed', truncated: false })
})

it('exports the full previous log through the protected streaming route without a tail filter', async () => {
  const fetch = vi.fn().mockResolvedValue(response(meta + event('end', { generation: 'gen', reason: 'completed' })))
  vi.stubGlobal('fetch', fetch)
  await consumeAggregateStream(target, { generation: 'gen' }, { csrfToken: 'csrf' }, { ...options, download: true, previous: true }, new AbortController().signal, vi.fn())
  expect(fetch.mock.calls[0][0]).toBe('/api/v1/pods/payments/api/logs/download/stream?container=main&timestamps=true&previous=true')
  expect(fetch.mock.calls[0][1].headers['X-KubePeep-CSRF']).toBe('csrf')
})

it.each([
  ['missing metadata', event('line', { text: 'private' })],
  ['wrong generation', event('meta', { generation: 'old' }) + event('line', { text: 'private' })],
  ['oversized event', meta + event('line', { text: 'x'.repeat(150 * 1024) })],
])('rejects %s before publishing data', async (_name, body) => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(body)))
  const lines = vi.fn()
  await expect(consumeAggregateStream(target, { generation: 'gen' }, { csrfToken: 'csrf' }, options, new AbortController().signal, lines)).rejects.toThrow()
  expect(lines).not.toHaveBeenCalled()
})

it('does not emit buffered lines after cancellation', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(meta + event('line', { text: 'one' }) + event('line', { text: 'two' }))))
  const controller = new AbortController()
  const lines = vi.fn(() => controller.abort())
  await expect(consumeAggregateStream(target, { generation: 'gen' }, { csrfToken: 'csrf' }, options, controller.signal, lines)).rejects.toThrow()
  expect(lines).toHaveBeenCalledTimes(1)
})
