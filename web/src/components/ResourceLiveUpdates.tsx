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

export interface ResourceStreamProgress {
  topic: ResourceTopic
  snapshotId: string
  items: unknown[]
  completedNamespaces: number
  requestedNamespaces: number
}

const topicOrder: ResourceTopic[] = ['pods', 'events', 'workloads', 'services', 'ingresses', 'endpoint-slices', 'configmaps']

function appendPreview(current: ResourceStreamProgress | null, next: ResourceStreamProgress): ResourceStreamProgress {
  return current?.snapshotId === next.snapshotId
    ? { ...next, items: [...current.items, ...next.items].slice(0, 500) }
    : next
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

function streamURLPath(topics: ResourceTopic[]): string {
  const query = new URLSearchParams()
  for (const topic of topicOrder) if (topics.includes(topic)) query.append('topic', topic)
  return `/api/v1/stream?${query.toString()}`
}

export function ResourceLiveUpdates({ generation, topics, queryKeys, autoStart = false, onProgress, onPreviewReset }: { generation: string; topics: ResourceTopic[]; queryKeys: ReadonlyArray<readonly unknown[]>; autoStart?: boolean; onProgress?: (progress: ResourceStreamProgress) => void; onPreviewReset?: () => void }) {
  const queryClient = useQueryClient()
  const [state, setState] = useState<LiveState>({ mode: 'idle', message: 'Live updates are off; use Refresh for an HTTP snapshot.' })
  const controllerRef = useRef<AbortController | null>(null)
  const invalidateTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const statusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pendingDeltaCountRef = useRef(0)
  const snapshotItemCountRef = useRef(0)
  const namespaceProgressRef = useRef({ completed: 0, requested: 0 })
  const pendingPreviewRef = useRef<ResourceStreamProgress | null>(null)
  const hasProgressRef = useRef(false)
  const mountedRef = useRef(true)
  const autoAttemptedRef = useRef(false)
  const onProgressRef = useRef(onProgress)
  const onPreviewResetRef = useRef(onPreviewReset)
  useEffect(() => {
    onProgressRef.current = onProgress
    onPreviewResetRef.current = onPreviewReset
  }, [onProgress, onPreviewReset])

  const invalidate = useCallback(async () => {
    await Promise.all(queryKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey })))
  }, [queryClient, queryKeys])

  function scheduleInvalidate(delayMs: number) {
    if (invalidateTimerRef.current) return
    invalidateTimerRef.current = setTimeout(() => {
      invalidateTimerRef.current = null
      void invalidate()
    }, delayMs)
  }

  function scheduleProgressStatus() {
    if (statusTimerRef.current) return
    statusTimerRef.current = setTimeout(() => {
      statusTimerRef.current = null
      const deltaCount = pendingDeltaCountRef.current
      pendingDeltaCountRef.current = 0
      const preview = pendingPreviewRef.current
      pendingPreviewRef.current = null
      if (preview) onProgressRef.current?.(preview)
      if (mountedRef.current) setState({
        mode: 'live',
        message: `Live updates active for ${topics.join(', ')}; ${namespaceProgressRef.current.requested ? `✓ ${namespaceProgressRef.current.completed}/${namespaceProgressRef.current.requested} namespaces · ` : ''}${snapshotItemCountRef.current} snapshot items received${deltaCount ? ` · ${deltaCount} watch changes batched` : ''}. Stream rows are previews until the HTTP list confirms them.`,
      })
    }, 75)
  }

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      controllerRef.current?.abort()
      if (invalidateTimerRef.current) clearTimeout(invalidateTimerRef.current)
      if (statusTimerRef.current) clearTimeout(statusTimerRef.current)
    }
  }, [generation])

  function fallbackToManualRefresh(message: string) {
    onPreviewResetRef.current?.()
    if (mountedRef.current) setState({ mode: 'error', message })
  }

  function stop() {
    autoAttemptedRef.current = true
    controllerRef.current?.abort()
    controllerRef.current = null
    if (invalidateTimerRef.current) clearTimeout(invalidateTimerRef.current)
    invalidateTimerRef.current = null
    if (statusTimerRef.current) clearTimeout(statusTimerRef.current)
    statusTimerRef.current = null
    pendingDeltaCountRef.current = 0
    snapshotItemCountRef.current = 0
    pendingPreviewRef.current = null
    onPreviewResetRef.current?.()
    setState({ mode: 'idle', message: 'Live updates are off; use Refresh for an HTTP snapshot.' })
  }

  async function start() {
    autoAttemptedRef.current = true
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    pendingDeltaCountRef.current = 0
    snapshotItemCountRef.current = 0
    namespaceProgressRef.current = { completed: 0, requested: 0 }
    hasProgressRef.current = false
    pendingPreviewRef.current = null
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
      setState({ mode: 'live', message: `Live updates active for ${topics.join(', ')}; HTTP confirms the final list.` })
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
          if (event.event === 'progress') {
            if (typeof payload.snapshotId !== 'string' || !topics.includes(payload.topic as ResourceTopic) || !Array.isArray(payload.items)) continue
            hasProgressRef.current = true
            const items = payload.items.slice(0, 500)
            snapshotItemCountRef.current += items.length
            const completedNamespaces = typeof payload.completedNamespaces === 'number' ? payload.completedNamespaces : 0
            const requestedNamespaces = typeof payload.requestedNamespaces === 'number' ? payload.requestedNamespaces : 0
            namespaceProgressRef.current = { completed: completedNamespaces, requested: requestedNamespaces }
            pendingPreviewRef.current = appendPreview(pendingPreviewRef.current, { topic: payload.topic as ResourceTopic, snapshotId: payload.snapshotId, items, completedNamespaces, requestedNamespaces })
            scheduleProgressStatus()
          } else if (event.event === 'snapshot') {
            if (!hasProgressRef.current && Array.isArray(payload.items)) snapshotItemCountRef.current += payload.items.length
            scheduleProgressStatus()
            if (payload.final === true) scheduleInvalidate(250)
          } else if (event.event === 'refreshed') {
            setState({ mode: 'live', message: 'The Kubernetes snapshot expired and was renewed automatically; live updates remain active.' })
            scheduleInvalidate(250)
          } else if (event.event === 'added' || event.event === 'modified' || event.event === 'deleted') {
            pendingDeltaCountRef.current += 1
            scheduleProgressStatus()
            // A sustained watch burst must not turn into a LIST every 250 ms.
            // Coalesce all deltas in one bounded screen-refresh window.
            scheduleInvalidate(2_000)
          } else if (event.event === 'reset') {
            onPreviewResetRef.current?.()
            await invalidate()
            if (payload.reason === 'generation_changed') throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
            fallbackToManualRefresh(`Stream reset (${String(payload.reason ?? 'state_lost')}). Automatic polling is disabled; use Refresh now.`)
            return
          } else if (event.event === 'error') {
            onPreviewResetRef.current?.()
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

  const startRef = useRef(start)
  useEffect(() => { startRef.current = start })
  useEffect(() => {
    if (!autoStart || state.mode !== 'idle' || autoAttemptedRef.current) return
    const timer = setTimeout(() => {
      if (!autoAttemptedRef.current) void startRef.current()
    }, 100)
    return () => clearTimeout(timer)
  }, [autoStart, generation, state.mode])

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
