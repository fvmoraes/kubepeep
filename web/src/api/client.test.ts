import { afterEach, describe, expect, it, vi } from 'vitest'

import { getContexts, getDashboardRestarts, getDashboardSummary, getPermissions, scanDashboardLogs, selectContext } from './client'

function json(data: unknown): Response {
  return new Response(JSON.stringify({ data }), { status: 200, headers: { 'Content-Type': 'application/json; charset=utf-8' } })
}

afterEach(() => vi.unstubAllGlobals())

describe('local API client security boundary', () => {
  it('uses no-store, same-origin credentials, and forwards cancellation', async () => {
    const controller = new AbortController()
    const fetch = vi.fn().mockResolvedValue(json([]))
    vi.stubGlobal('fetch', fetch)

    await getContexts(42, controller.signal)

    expect(fetch).toHaveBeenCalledWith('/api/v1/contexts?clusterProfileId=42', expect.objectContaining({
      cache: 'no-store',
      credentials: 'same-origin',
      signal: controller.signal,
    }))
  })

  it('builds bounded repeated permission query parameters without arbitrary SAR fields', async () => {
    const fetch = vi.fn().mockResolvedValue(json({ generation: 'gen_1', decisions: [], complete: true, truncated: false, errors: [] }))
    vi.stubGlobal('fetch', fetch)

    await getPermissions({ namespaces: ['payments', 'billing'], capabilityIds: ['pods.list'], resourceNames: ['api'], refresh: true })

    expect(fetch.mock.calls[0][0]).toBe('/api/v1/permissions?namespace=payments&namespace=billing&capability=pods.list&resourceName=api&refresh=true')
  })

  it('sends the ephemeral CSRF token only as a mutation header', async () => {
    const fetch = vi.fn().mockResolvedValue(json({ generation: 'gen_2' }))
    vi.stubGlobal('fetch', fetch)

    await selectContext({ clusterProfileId: 1, context: 'development', setDefault: true, expectedGeneration: 'gen_1' }, 'ephemeral-token')

    expect(fetch).toHaveBeenCalledWith('/api/v1/contexts/select', expect.objectContaining({
      method: 'POST',
      cache: 'no-store',
      credentials: 'same-origin',
      headers: expect.objectContaining({ 'X-KubePeep-CSRF': 'ephemeral-token' }),
    }))
    expect(String(fetch.mock.calls[0][0])).not.toContain('ephemeral-token')
  })

	it('keeps dashboard metadata while normalizing a bounded block', async () => {
    const controller = new AbortController()
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      data: { value: { namespaces: { state: 'available', value: 0 } }, complete: true, truncated: false, coverage: null, errors: [] },
      meta: { generation: 'gen_42', collectedAt: '2026-08-10T12:00:00Z' },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetch)

    const response = await getDashboardSummary(controller.signal)

    expect(response.meta?.generation).toBe('gen_42')
    expect(response.block.complete).toBe(true)
    expect(fetch).toHaveBeenCalledWith('/api/v1/dashboard/summary', expect.objectContaining({ signal: controller.signal }))
	})

	it('rejects a dashboard response from another selection generation', async () => {
		const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({
			data: { value: [], complete: true, truncated: false, coverage: null, errors: [] },
			meta: { generation: 'gen_old', collectedAt: '2026-08-10T12:00:00Z' },
		}), { status: 200, headers: { 'Content-Type': 'application/json' } }))
		vi.stubGlobal('fetch', fetch)

		await expect(getDashboardSummary(undefined, 'gen_current')).rejects.toMatchObject({
			status: 409,
			code: 'GENERATION_CHANGED',
		})
	})

  it('uses the documented restart limit and sends log scan settings only in the CSRF-protected body', async () => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(json({ value: [], complete: true, truncated: false, coverage: null, errors: [] })))
    vi.stubGlobal('fetch', fetch)

    await getDashboardRestarts(10)
    await scanDashboardLogs({ window: '30m', tailLines: 200, maxPods: 20, maxConcurrentContainers: 4 }, 'scan-token')

    expect(fetch.mock.calls[0][0]).toBe('/api/v1/dashboard/restarts?limit=10')
    expect(fetch.mock.calls[1][0]).toBe('/api/v1/dashboard/log-scan')
    expect(fetch.mock.calls[1][1]).toEqual(expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'X-KubePeep-CSRF': 'scan-token' }),
      body: JSON.stringify({ window: '30m', tailLines: 200, maxPods: 20, maxConcurrentContainers: 4 }),
    }))
    expect(String(fetch.mock.calls[1][0])).not.toContain('scan-token')
  })
})
