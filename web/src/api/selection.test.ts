import { QueryClient, QueryObserver } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { publishSelection } from './selection'
import type { SelectionData, StatusData } from './types'

const selected: SelectionData = {
  clusterProfileId: 1, context: 'dev', cluster: 'cluster', scopeId: null,
  scopeName: null, scopeMode: null, scopeSource: 'none', defaultNamespace: null,
  namespaceCount: 0, generation: 'g2',
}

it('refreshes a pending initial status after a successful context selection', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let initialAborted = false
  const queryFn = vi.fn(({ signal }: { signal: AbortSignal }) => {
    if (queryFn.mock.calls.length === 1) return new Promise<StatusData>((_resolve, reject) => {
      signal.addEventListener('abort', () => { initialAborted = true; reject(new DOMException('aborted', 'AbortError')) })
    })
    return Promise.resolve({ selection: selected, components: {} } as StatusData)
  })
  const observer = new QueryObserver(client, { queryKey: ['local-status'], queryFn })
  const stop = observer.subscribe(() => {})
  try {
    expect(await publishSelection(client, selected, undefined)).toBe(true)
    await vi.waitFor(() => expect(client.getQueryData(['local-status'])).toMatchObject({ selection: selected }))
    expect(initialAborted).toBe(true)
    expect(queryFn).toHaveBeenCalledTimes(2)
  } finally { stop(); client.clear() }
})

it('preserves a newer generation published during status cancellation', async () => {
  const client = new QueryClient()
  client.setQueryData(['local-status'], { selection: { ...selected, generation: 'g1' }, components: {} })
  const newer = { ...selected, context: 'prod', generation: 'g3' }
  const publication = publishSelection(client, selected, 'g1')
  client.setQueryData(['local-status'], { selection: newer, components: {} })
  expect(await publication).toBe(false)
  expect(client.getQueryData(['local-status'])).toMatchObject({ selection: newer })
  client.clear()
})
