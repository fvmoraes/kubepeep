import { describe, expect, it } from 'vitest'

import { DesktopResponse } from './desktop'
import type { InvokeResult } from '../wailsjs/go/desktop/Bridge'

function desktopResponse(headers: Record<string, string[]>): DesktopResponse {
  const result: InvokeResult = { status: 200, headers, body: '{"data":{"ok":true}}' }
  return new DesktopResponse(result)
}

describe('desktop bridge response headers', () => {
  it('resolves headers regardless of the key case produced by the Go bridge', () => {
    const canonical = desktopResponse({
      'Content-Type': ['application/json; charset=utf-8'],
      'X-Request-ID': ['req_case_1'],
    })
    expect(canonical.headers.get('content-type')).toBe('application/json; charset=utf-8')
    expect(canonical.headers.get('X-Request-ID')).toBe('req_case_1')

    const lowercase = desktopResponse({ 'content-type': ['application/json'] })
    expect(lowercase.headers.get('content-type')).toBe('application/json')
  })

  it('returns null for missing headers', () => {
    expect(desktopResponse({ 'Content-Type': ['application/json'] }).headers.get('X-Request-ID')).toBeNull()
  })
})
