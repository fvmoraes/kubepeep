import { APIError } from '../../api/client'
import { streamURL } from '../../api/desktop'
import type { APIErrorPayload, LogLine, SelectionSummary } from '../../api/types'

export interface LogTarget { namespace: string; pod: string; container: string }
export interface AggregatedLogLine extends LogLine { pod: string; container: string }
export interface LogTerminal { reason: string; truncated: boolean }
interface StreamOptions {
  timestamps: boolean
  tailLines: number
  since: string
  download?: boolean
  previous?: boolean
  onOpen?: () => void
  onActivity?: () => void
}

function logURL(target: LogTarget, options: StreamOptions): string {
  const query = new URLSearchParams({ container: target.container, timestamps: String(options.timestamps) })
  if (options.download) query.set('previous', String(options.previous ?? false))
  else {
    query.set('tailLines', String(options.tailLines))
    if (options.since !== '') query.set('since', options.since)
  }
  return `/api/v1/pods/${encodeURIComponent(target.namespace)}/${encodeURIComponent(target.pod)}/logs/${options.download ? 'download/' : ''}stream?${query}`
}

function parseSSEBlock(block: string): { event: string; data: string } | null {
  let event = 'message'
  const data: string[] = []
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith('event:')) event = line.slice(6).trimStart()
    if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
  }
  return data.length > 0 ? { event, data: data.join('\n') } : null
}

export async function consumeAggregateStream(target: LogTarget, selection: Pick<SelectionSummary, 'generation'>, session: { csrfToken: string }, options: StreamOptions, signal: AbortSignal, onLine: (line: AggregatedLogLine) => void): Promise<LogTerminal> {
  const url = await streamURL(logURL(target, options))
  signal.throwIfAborted()
  const response = await fetch(url, { method: 'GET', headers: { Accept: 'text/event-stream', 'X-KubePeep-CSRF': session.csrfToken }, cache: 'no-store', credentials: 'same-origin', signal })
  if (!response.ok) {
    const payload = response.headers.get('content-type')?.toLowerCase().startsWith('application/json')
      ? await response.json() as APIErrorPayload
      : { code: 'STREAM_ERROR', message: 'The log stream could not be opened.' }
    throw new APIError(response.status, payload)
  }
  if (!response.body) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The log stream has no response body.' })
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let metaSeen = false
  const invalid = (message: string) => new APIError(502, { code: 'INVALID_RESPONSE', message })
  const cancelReader = () => { void reader.cancel().catch(() => {}) }
  signal.addEventListener('abort', cancelReader, { once: true })
  try {
    if (!response.headers.get('content-type')?.toLowerCase().startsWith('text/event-stream')) throw invalid('Unexpected log stream content type.')
    while (true) {
      signal.throwIfAborted()
      const chunk = await reader.read()
      signal.throwIfAborted()
      if (chunk.done) return { reason: 'upstream_eof', truncated: false }
      options.onActivity?.()
      // Bound incomplete events, not fetch chunks containing many valid events.
      for (let offset = 0; offset < chunk.value.length; offset += 32 * 1024) {
        buffer += decoder.decode(chunk.value.subarray(offset, offset + 32 * 1024), { stream: true })
        while (true) {
          signal.throwIfAborted()
          const separator = /\r?\n\r?\n/.exec(buffer)
          if (!separator) break
          if (new TextEncoder().encode(buffer.slice(0, separator.index)).byteLength > 136 * 1024) throw invalid('The log stream exceeded the event buffer.')
          const event = parseSSEBlock(buffer.slice(0, separator.index))
          buffer = buffer.slice(separator.index + separator[0].length)
          if (!event) continue
          let payload: Record<string, unknown>
          try { payload = JSON.parse(event.data) as Record<string, unknown> } catch { throw invalid('Invalid log stream event.') }
          if (!payload || typeof payload !== 'object') throw invalid('Invalid log stream event.')
          if (event.event === 'meta' || event.event === 'heartbeat' || event.event === 'end') {
            if (payload.generation !== selection.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The log stream generation changed.' })
          }
          if (event.event === 'meta') { metaSeen = true; options.onOpen?.() }
          if (event.event === 'line') {
            if (!metaSeen) throw invalid('The log stream sent data before metadata.')
            onLine({ pod: target.pod, container: target.container, timestamp: typeof payload.timestamp === 'string' ? payload.timestamp : null, text: typeof payload.text === 'string' ? payload.text : '', truncated: payload.truncated === true })
          }
          if (event.event === 'error') throw new APIError(502, { code: String(payload.code ?? 'STREAM_ERROR'), message: String(payload.message ?? 'The log stream ended.') })
          if (event.event === 'end') {
            if (!metaSeen) throw invalid('The log stream ended before metadata.')
            return { reason: String(payload.reason ?? 'upstream_eof'), truncated: payload.truncated === true }
          }
        }
        if (new TextEncoder().encode(buffer).byteLength > 136 * 1024) throw invalid('The log stream exceeded the event buffer.')
      }
    }
  } finally {
    signal.removeEventListener('abort', cancelReader)
    await reader.cancel().catch(() => {})
    reader.releaseLock()
  }
}
