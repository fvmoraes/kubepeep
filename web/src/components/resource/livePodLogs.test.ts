import { afterEach, expect, it, vi } from 'vitest'
import { APIError, getSession } from '../../api/client'
import { followPodLogs } from './livePodLogs'
import { consumeAggregateStream } from './podLogStream'

vi.mock('../../api/client', async (original) => ({ ...await original<typeof import('../../api/client')>(), getSession: vi.fn() }))
vi.mock('./podLogStream', () => ({ consumeAggregateStream: vi.fn() }))
afterEach(() => { vi.useRealTimers(); vi.resetAllMocks() })
const target = { namespace: 'payments', pod: 'api', container: 'main' }
const session = { generation: 'gen', csrfToken: 'csrf', origin: 'http://localhost', expiresAt: '2026-10-07T00:00:00Z' }

it('reconnects with overlap, preserves same-timestamp repeated lines and stops when the container ends', async () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-06T12:00:05Z'))
  vi.mocked(getSession).mockResolvedValue(session)
  const line = { ...target, timestamp: '2026-10-06T12:00:01.123456789Z', text: 'repeated', truncated: false }
  vi.mocked(consumeAggregateStream).mockImplementationOnce(async (_target, _selection, _session, options, _signal, emit) => {
    options.onOpen?.(); emit(line); emit(line)
    return { reason: 'upstream_eof', truncated: false }
  }).mockImplementationOnce(async (_target, _selection, _session, options, _signal, emit) => {
    expect(options.since).toBe('6s')
    options.onOpen?.(); emit(line); emit(line)
    emit({ ...line, timestamp: '2026-10-06T12:00:06Z', text: 'new' })
    return { reason: 'container_terminated', truncated: false }
  })
  const lines = vi.fn(); const status = vi.fn()
  const running = followPodLogs(target, 'gen', new AbortController().signal, lines, status)
  await vi.advanceTimersByTimeAsync(1000)
  await running
  expect(lines.mock.calls.map(([line]) => line.text)).toEqual(['repeated', 'repeated', 'new'])
  expect(status).toHaveBeenCalledWith('Live')
  expect(status).toHaveBeenLastCalledWith('Container ended')
})

it('retries transient failures but cancels pending reconnects when the panel closes', async () => {
  vi.useFakeTimers()
  vi.mocked(getSession).mockResolvedValue(session)
  vi.mocked(consumeAggregateStream).mockRejectedValue(new Error('network disconnected'))
  const controller = new AbortController()
  const running = followPodLogs(target, 'gen', controller.signal, vi.fn(), vi.fn()).catch(() => {})
  await vi.advanceTimersByTimeAsync(1000)
  expect(consumeAggregateStream).toHaveBeenCalledTimes(2)
  controller.abort()
  await running
  await vi.advanceTimersByTimeAsync(60_000)
  expect(consumeAggregateStream).toHaveBeenCalledTimes(2)
})

it('does not open a log stream after the generation changes', async () => {
  vi.mocked(getSession).mockResolvedValue({ ...session, generation: 'new' })
  await expect(followPodLogs(target, 'gen', new AbortController().signal, vi.fn(), vi.fn())).rejects.toMatchObject({ code: 'GENERATION_CHANGED' })
  expect(consumeAggregateStream).not.toHaveBeenCalled()
})

it('clears unavailable authorization before retrying and never retries a definite denial', async () => {
  vi.useFakeTimers()
  vi.mocked(getSession).mockResolvedValue(session)
  vi.mocked(consumeAggregateStream)
    .mockRejectedValueOnce(new APIError(503, { code: 'AUTHORIZATION_UNAVAILABLE', message: 'Temporary authorization failure' }))
    .mockRejectedValueOnce(new APIError(403, { code: 'FORBIDDEN', message: 'Denied' }))
  const clear = vi.fn()
  const running = followPodLogs(target, 'gen', new AbortController().signal, vi.fn(), vi.fn(), clear)
  const failure = expect(running).rejects.toMatchObject({ code: 'FORBIDDEN' })
  await vi.advanceTimersByTimeAsync(1000)
  await failure
  expect(clear).toHaveBeenCalledOnce()
  await vi.advanceTimersByTimeAsync(60000)
  expect(consumeAggregateStream).toHaveBeenCalledTimes(2)
})
