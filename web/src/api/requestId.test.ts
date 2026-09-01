import { afterEach, describe, expect, it, vi } from 'vitest'

import { getStatus } from './client'

describe('request id correlation (O-04)', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('attaches the backend X-Request-ID header to APIError', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(JSON.stringify({
      error: { code: 'FORBIDDEN', message: 'Denied.' },
    }), {
      status: 403,
      headers: { 'Content-Type': 'application/json', 'X-Request-ID': 'req_test_123' },
    }))))
    try {
      await getStatus()
      throw new Error('expected getStatus to reject')
    } catch (error) {
      expect((error as { requestId?: string }).requestId).toBe('req_test_123')
    }
  })

  it('leaves requestId undefined when the backend omits the header', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response('not json', {
      status: 500,
      headers: { 'Content-Type': 'text/plain' },
    }))))
    try {
      await getStatus()
      throw new Error('expected getStatus to reject')
    } catch (error) {
      expect((error as { requestId?: string }).requestId).toBeUndefined()
    }
  })
})
