import { APIError } from '../../api/client'
import { streamURL } from '../../api/desktop'
import type { APIErrorPayload, LogLine, SelectionSummary } from '../../api/types'

function logURL(namespace: string, pod: string, container: string, timestamps: boolean, tailLines: number, since: string): string {
  const query = new URLSearchParams({ container, timestamps: String(timestamps), tailLines: String(tailLines) })
  if (since !== '') query.set('since', since)
  return `/api/v1/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/logs/stream?${query.toString()}`
}

function parseSSEBlock(block: string): { event: string; data: string } | null {
  let event = 'message'
  const data: string[] = []
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith(':')) continue
    if (line.startsWith('event:')) event = line.slice(6).trimStart()
    if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
  }
  return data.length > 0 ? { event, data: data.join('\n') } : null
}

export interface AggregatedLogLine extends LogLine { pod: string; container: string }


export async function consumeAggregateStream(target: { namespace: string; pod: string; container: string }, selection: Pick<SelectionSummary, 'generation'>, session: { csrfToken: string }, options: { timestamps: boolean; tailLines: number; since: string }, signal: AbortSignal, onLine: (line: AggregatedLogLine) => void) {
	const url = await streamURL(logURL(target.namespace, target.pod, target.container, options.timestamps, options.tailLines, options.since))
	signal.throwIfAborted()
	const response = await fetch(url, { method: 'GET', headers: { Accept: 'text/event-stream', 'X-KubePeep-CSRF': session.csrfToken }, cache: 'no-store', credentials: 'same-origin', signal })
	if (!response.ok) {
		const payload = response.headers.get('content-type')?.toLowerCase().startsWith('application/json')
			? await response.json() as APIErrorPayload
			: { code: 'STREAM_ERROR', message: 'The log stream could not be opened.' }
		throw new APIError(response.status, payload)
	}
	if (!response.body) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The log stream has no response body.' })
	if (!response.headers.get('content-type')?.toLowerCase().startsWith('text/event-stream')) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'Unexpected log stream content type.' })
	const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ''; let metaSeen = false
	const cancelReader = () => { void reader.cancel().catch(() => {}) }
	signal.addEventListener('abort', cancelReader, { once: true })
	try {
		signal.throwIfAborted()
		while (true) {
			const chunk = await reader.read()
			signal.throwIfAborted()
			if (chunk.done) return
			buffer += decoder.decode(chunk.value, { stream: true })
			if (new TextEncoder().encode(buffer).byteLength > 136 * 1_024) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'An aggregate stream exceeded the bounded event buffer.' })
			while (true) {
				const separator = /\r?\n\r?\n/.exec(buffer); if (!separator) break
				const event = parseSSEBlock(buffer.slice(0, separator.index)); buffer = buffer.slice(separator.index + separator[0].length); if (!event) continue
				const payload = JSON.parse(event.data) as Record<string, unknown>
				if (event.event === 'meta') { if (payload.generation !== selection.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'An aggregate stream belongs to another generation.' }); metaSeen = true }
				if (event.event === 'heartbeat' && payload.generation !== selection.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The log stream generation changed.' })
				if (event.event === 'line') { if (!metaSeen) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'An aggregate stream sent data before metadata.' }); onLine({ pod: target.pod, container: target.container, timestamp: typeof payload.timestamp === 'string' ? payload.timestamp : null, text: typeof payload.text === 'string' ? payload.text : '', truncated: payload.truncated === true }) }
				if (event.event === 'error') throw new APIError(502, { code: String(payload.code ?? 'STREAM_ERROR'), message: String(payload.message ?? 'An aggregate stream ended.') })
				if (event.event === 'end') return
			}
		}
	} finally {
		signal.removeEventListener('abort', cancelReader)
		await reader.cancel().catch(() => {})
		reader.releaseLock()
	}
}
