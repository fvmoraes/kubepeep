import { beforeEach, describe, expect, it, vi } from 'vitest'

const bridge = vi.hoisted(() => ({
  PlatformInfo: vi.fn(),
  Invoke: vi.fn(),
  InvokeCancelable: vi.fn(),
  Cancel: vi.fn(),
}))

vi.mock('../wailsjs/go/desktop/Bridge', () => bridge)

import { DesktopResponse, desktopRequest } from './desktop'
import type { InvokeResult } from '../wailsjs/go/desktop/Bridge'

function desktopResponse(headers: Record<string, string[]>): DesktopResponse {
  const result: InvokeResult = { status: 200, headers, body: '{"data":{"ok":true}}' }
  return new DesktopResponse(result)
}

describe('desktop bridge response headers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })
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

describe('desktop request cancellation', () => {
	it('does not start a native request for an already aborted signal', async () => {
		window.go = { desktop: { Bridge: {
			PlatformInfo: bridge.PlatformInfo,
			Invoke: bridge.Invoke,
			InvokeCancelable: bridge.InvokeCancelable,
			Cancel: bridge.Cancel,
		} } }
		bridge.PlatformInfo.mockResolvedValue({ mode: 'desktop', streamBase: 'http://127.0.0.1', version: 'dev', commit: 'test', buildDate: '' })
		const controller = new AbortController()
		controller.abort()
		await expect(desktopRequest('GET', '/api/v1/pods', {}, undefined, controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
		expect(bridge.InvokeCancelable).not.toHaveBeenCalled()
	})

  it('forwards AbortSignal cancellation to the Wails bridge', async () => {
    window.go = { desktop: { Bridge: {
      PlatformInfo: bridge.PlatformInfo,
      Invoke: bridge.Invoke,
      InvokeCancelable: bridge.InvokeCancelable,
      Cancel: bridge.Cancel,
    } } }
    bridge.PlatformInfo.mockResolvedValue({ mode: 'desktop', streamBase: 'http://127.0.0.1', version: 'dev', commit: 'test', buildDate: '' })
    let rejectInvoke: ((reason: unknown) => void) | undefined
    bridge.InvokeCancelable.mockImplementation(() => new Promise((_resolve, reject) => { rejectInvoke = reject }))
    bridge.Cancel.mockImplementation(async () => { rejectInvoke?.(new DOMException('aborted', 'AbortError')) })
    const controller = new AbortController()
    const pending = desktopRequest('GET', '/api/v1/pods', {}, undefined, controller.signal)
    await vi.waitFor(() => expect(bridge.InvokeCancelable).toHaveBeenCalledOnce())
    controller.abort()
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    expect(bridge.Cancel).toHaveBeenCalledOnce()
  })
})
