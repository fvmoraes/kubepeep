import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'

import { APIError, getSession } from '../api/client'
import { streamURL } from '../api/desktop'
import type { APIErrorPayload } from '../api/types'
import { Button } from './ui'

export type ResourceTopic = 'pods' | 'events' | 'workloads' | 'services' | 'ingresses' | 'endpoint-slices' | 'configmaps'

type LiveMode = 'idle' | 'connecting' | 'live' | 'error'

interface LiveState {
  mode: LiveMode
  message: string
}

const topicOrder: ResourceTopic[] = ['pods', 'events', 'workloads', 'services', 'ingresses', 'endpoint-slices', 'configmaps']

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

function streamURLPath(topics: ResourceTopic[]): string {
  const query = new URLSearchParams()
  for (const topic of topicOrder) if (topics.includes(topic)) query.append('topic', topic)
  return `/api/v1/stream?${query.toString()}`
}

export function ResourceLiveUpdates({ generation, topics, queryKeys }: { generation: string; topics: ResourceTopic[]; queryKeys: ReadonlyArray<readonly unknown[]> }) {
  const queryClient = useQueryClient()
  const [state, setState] = useState<LiveState>({ mode: 'idle', message: 'Live updates are off; use Refresh for an HTTP snapshot.' })
  const controllerRef = useRef<AbortController | null>(null)
  const invalidateTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(true)

  const invalidate = useCallback(async () => {
    await Promise.all(queryKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey })))
  }, [queryClient, queryKeys])

  function scheduleInvalidate() {
    if (invalidateTimerRef.current) return
    invalidateTimerRef.current = setTimeout(() => {
      invalidateTimerRef.current = null
      void invalidate()
    }, 250)
  }

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      controllerRef.current?.abort()
      if (invalidateTimerRef.current) clearTimeout(invalidateTimerRef.current)
    }
  }, [generation])

  function fallbackToManualRefresh(message: string) {
    if (mountedRef.current) setState({ mode: 'error', message })
  }

  function stop() {
    controllerRef.current?.abort()
    controllerRef.current = null
    setState({ mode: 'idle', message: 'Live updates are off; use Refresh for an HTTP snapshot.' })
  }

  async function start() {
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    setState({ mode: 'connecting', message: 'Authorizing the bounded resource stream…' })
    try {
      const session = await getSession(controller.signal)
      if (session.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
      const response = await fetch(await streamURL(streamURLPath(topics)), {
        method: 'GET',
        headers: { Accept: 'text/event-stream', 'X-KubePeep-CSRF': session.csrfToken },
        cache: 'no-store',
        credentials: 'same-origin',
        signal: controller.signal,
      })
      if (!response.ok) {
        const contentType = response.headers.get('content-type')?.toLowerCase() ?? ''
        const payload = contentType.startsWith('application/json') ? await response.json() as APIErrorPayload : { code: 'INVALID_RESPONSE', message: 'The stream guard returned an invalid response.' }
        if (response.status === 403 || response.status === 503) {
          fallbackToManualRefresh(`${payload.code ?? response.status}: live watch is unavailable. Automatic polling is disabled; use Refresh now for a bounded HTTP snapshot.`)
          return
        }
        throw new APIError(response.status, payload)
      }
      if (!response.headers.get('content-type')?.toLowerCase().startsWith('text/event-stream')) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The resource stream used an unexpected content type.' })
      const reader = response.body?.getReader()
      if (!reader) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The resource stream has no response body.' })
      setState({ mode: 'live', message: `Live updates active for ${topics.join(', ')}; HTTP remains the source of displayed snapshots.` })
      const decoder = new TextDecoder()
      let buffer = ''
      while (true) {
        const chunk = await reader.read()
        if (chunk.done) break
        buffer += decoder.decode(chunk.value, { stream: true })
        if (new TextEncoder().encode(buffer).byteLength > 128 * 1_024) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The resource stream exceeded the bounded event buffer.' })
        while (true) {
          const separator = /\r?\n\r?\n/.exec(buffer)
          if (!separator) break
          const raw = buffer.slice(0, separator.index)
          buffer = buffer.slice(separator.index + separator[0].length)
          const event = parseSSEBlock(raw)
          if (!event) continue
          let payload: Record<string, unknown>
          try { payload = JSON.parse(event.data) as Record<string, unknown> } catch { throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The resource stream sent invalid JSON.' }) }
          if (typeof payload.generation === 'string' && payload.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The resource stream belongs to another generation.' })
          if (event.event === 'snapshot') {
            if (payload.final === true) scheduleInvalidate()
          } else if (event.event === 'added' || event.event === 'modified' || event.event === 'deleted') {
            scheduleInvalidate()
          } else if (event.event === 'reset') {
            await invalidate()
            if (payload.reason === 'generation_changed') throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
            fallbackToManualRefresh(`Stream reset (${String(payload.reason ?? 'state_lost')}). Automatic polling is disabled; use Refresh now.`)
            return
          } else if (event.event === 'error') {
            const code = String(payload.code ?? 'STREAM_ERROR')
            if (code === 'FORBIDDEN' || code === 'AUTHORIZATION_UNAVAILABLE') {
              fallbackToManualRefresh(`${code}: live watch is unavailable. Automatic polling is disabled; use Refresh now.`)
              return
            }
            throw new APIError(502, { code, message: String(payload.message ?? 'The resource stream ended.') })
          }
        }
      }
      fallbackToManualRefresh('The live stream closed. Automatic polling is disabled; use Refresh now.')
    } catch (error) {
      if (controller.signal.aborted || !mountedRef.current) return
      if (error instanceof APIError && error.code === 'GENERATION_CHANGED') {
        setState({ mode: 'error', message: `${error.code}: ${error.message}` })
      } else {
        fallbackToManualRefresh(`${error instanceof APIError ? error.code : 'STREAM_UNAVAILABLE'}: live updates failed. Automatic polling is disabled; use Refresh now.`)
      }
    } finally {
      if (controllerRef.current === controller) controllerRef.current = null
    }
  }

  const stateStyles: Record<LiveMode, string> = {
    idle: 'border-kp-overlay-0 text-kp-overlay-text',
    connecting: 'border-kp-blue-border text-kp-sky',
    live: 'border-kp-green-border text-kp-green',
    error: 'border-kp-red-border text-kp-red',
  }

  return (
    <section className={`flex flex-wrap items-center justify-between gap-2 rounded-xl border bg-kp-surface-0 px-3 py-2 ${stateStyles[state.mode]}`} aria-label="Resource live updates">
      <span aria-live="polite" className="text-xs leading-snug max-w-[540px]">{state.message}</span>
      <div className="flex flex-wrap justify-end gap-1.5">
        <Button size="sm" variant="secondary" disabled={state.mode === 'connecting' || state.mode === 'live'} onClick={() => void start()}>{state.mode === 'error' ? 'Retry live updates' : 'Start live updates'}</Button>
        <Button size="sm" variant="secondary" onClick={() => void invalidate()}>Refresh now</Button>
        {state.mode === 'live' || state.mode === 'connecting' ? <Button size="sm" variant="danger" onClick={stop}>Stop live updates</Button> : null}
      </div>
    </section>
  )
}
