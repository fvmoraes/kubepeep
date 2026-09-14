import { Cancel, Invoke, InvokeCancelable, PlatformInfo, type InvokeResult } from '../wailsjs/go/desktop/Bridge'

declare global {
  interface Window {
    go?: {
      desktop?: {
        Bridge?: {
          PlatformInfo(): Promise<DesktopPlatformInfo>
          Invoke(method: string, path: string, headers: Record<string, string>, body: string): Promise<InvokeResult>
          InvokeCancelable(requestID: string, method: string, path: string, headers: Record<string, string>, body: string): Promise<InvokeResult>
          Cancel(requestID: string): Promise<void>
        }
      }
    }
  }
}

export interface DesktopPlatformInfo {
  mode: string
  streamBase: string
  version: string
  commit: string
  buildDate: string
}

let cached: DesktopPlatformInfo | null = null
let unavailable = false

export async function desktopPlatform(): Promise<DesktopPlatformInfo | null> {
  if (cached) return cached
  if (unavailable) return null
  try {
    if (typeof window === 'undefined' || !window.go?.desktop?.Bridge) {
      unavailable = true
      return null
    }
    const info = await PlatformInfo()
    cached = info
    return info
  } catch {
    unavailable = true
    return null
  }
}

// streamURL resolves the absolute loopback URL for streaming transports
// (SSE and exec WebSocket) in desktop mode. JSON calls never use it; they go
// through the Wails bindings.
export async function streamURL(path: string): Promise<string> {
  const info = await desktopPlatform()
  return info ? `${info.streamBase}${path}` : path
}

export class DesktopResponse {
  readonly status: number
  readonly ok: boolean
  private readonly result: InvokeResult
  private readonly headerIndex: Record<string, string[]>

  constructor(result: InvokeResult) {
    this.result = result
    this.status = result.status
    this.ok = result.status >= 200 && result.status < 300
    // Bridge headers may arrive in any key case (Go canonicalizes them);
    // index by lowercase so lookups mirror the native Headers behavior.
    this.headerIndex = {}
    for (const [name, values] of Object.entries(result.headers ?? {})) {
      this.headerIndex[name.toLowerCase()] = values
    }
  }

  get headers(): { get(name: string): string | null } {
    return {
      get: (name: string) => {
        const values = this.headerIndex[name.toLowerCase()] ?? []
        return values.length > 0 ? values[0] : null
      },
    }
  }

  async json(): Promise<unknown> {
    return JSON.parse(this.result.body) as unknown
  }

  async text(): Promise<string> {
    return this.result.body
  }
}

// desktopRequest runs the request through the Wails binding when the desktop
// runtime is present, returning null so callers can fall back to fetch.
export async function desktopRequest(method: string, path: string, headers: Record<string, string>, body?: string, signal?: AbortSignal | null): Promise<DesktopResponse | null> {
  const info = await desktopPlatform()
  if (!info) return null
  if (!signal) return new DesktopResponse(await Invoke(method, path, headers, body ?? ''))
  if (signal.aborted) throw new DOMException('The operation was aborted.', 'AbortError')
  const requestID = crypto.randomUUID()
	const abort = () => { void Cancel(requestID).catch(() => undefined) }
	signal.addEventListener('abort', abort, { once: true })
	try {
		const result = await InvokeCancelable(requestID, method, path, headers, body ?? '')
		if (signal.aborted) throw new DOMException('The operation was aborted.', 'AbortError')
		return new DesktopResponse(result)
  } finally {
    signal.removeEventListener('abort', abort)
  }
}
