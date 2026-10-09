import { afterEach, expect, it, vi } from 'vitest'
import { getBulkPermissions } from './bulkPermissions'

afterEach(() => vi.unstubAllGlobals())
it('checks 100 selected objects in bounded batches with exact namespace/name pairs', async () => {
  const calls: URLSearchParams[] = []
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const params = new URL(input, 'http://local').searchParams
    calls.push(params)
    const names = params.getAll('resourceName')
    expect(names.length).toBeLessThanOrEqual(20)
    expect(new Set(names).size).toBe(names.length)
    return new Response(JSON.stringify({ data: { generation: 'gen', complete: true, truncated: false, errors: [], decisions: names.map((resourceName) => ({ capabilityId: params.get('capability'), namespace: params.get('namespace'), resourceName, decision: resourceName === 'pod-3' ? 'denied' : 'allowed' })) } }), { headers: { 'Content-Type': 'application/json' } })
  }))
  const targets = Array.from({ length: 100 }, (_, index) => ({ capabilityId: 'pods.delete', namespace: index < 50 ? 'one' : 'two', name: `pod-${index % 50}` }))
  const result = await getBulkPermissions(targets, 'gen')
  expect(result.decisions).toHaveLength(100)
  expect(calls).toHaveLength(6)
  expect(result.decisions.filter((item) => item.decision === 'denied')).toHaveLength(2)
  expect(result.decisions.filter((item) => item.resourceName === 'pod-3').map((item) => item.namespace)).toEqual(['one', 'two'])
})

it('does not issue permission requests after cancellation', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  const abort = new AbortController(); abort.abort()
  await expect(getBulkPermissions([{ capabilityId: 'pods.delete', namespace: 'one', name: 'pod' }], 'gen', abort.signal)).rejects.toThrow()
  expect(fetch).not.toHaveBeenCalled()
})
