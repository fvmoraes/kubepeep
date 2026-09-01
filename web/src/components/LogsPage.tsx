import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'

import { APIError, getPermissions, getPod, getPodLogs, getPods, getPreferences, getSession, getStatus } from '../api/client'
import { streamURL } from '../api/desktop'
import type { APIErrorPayload, CollectionResult, LogLine, Pod, Preferences, SelectionSummary } from '../api/types'
import { Button, Input, Select } from '../components/ui'
import { SavedFilterControls } from './SavedFilterControls'
import { StatePanel } from './StatePanel'

interface FollowState {
  status: 'idle' | 'connecting' | 'following' | 'ended' | 'error'
  message: string
}

function message(error: unknown): string {
  if (error instanceof APIError) return `${error.code}: ${error.message}`
  return error instanceof Error ? error.message : 'The log request failed.'
}

function logURL(namespace: string, pod: string, container: string, timestamps: boolean, tailLines: number, since: string): string {
  const query = new URLSearchParams({ container, timestamps: String(timestamps), tailLines: String(tailLines) })
  if (since !== '') query.set('since', since)
  return `/api/v1/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/logs/stream?${query.toString()}`
}

function appendBounded(lines: LogLine[], line: LogLine): LogLine[] {
  const encoder = new TextEncoder()
  const next = [...lines, line]
  let bytes = next.reduce((total, value) => total + encoder.encode(JSON.stringify(value)).byteLength, 0)
  while (next.length > 1_000 || bytes > 1 << 20) {
    const removed = next.shift()
    if (!removed) break
    bytes -= encoder.encode(JSON.stringify(removed)).byteLength
  }
  return next
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

function validSince(value: string): boolean {
  if (value === '') return true
  const match = /^([1-9][0-9]*)(s|m|h)$/.exec(value)
  if (!match) return false
  const amount = Number(match[1])
  const seconds = amount * (match[2] === 'h' ? 3_600 : match[2] === 'm' ? 60 : 1)
  return Number.isSafeInteger(seconds) && seconds <= 4 * 3_600
}

const defaultLogPreferences: Preferences['logs'] = { wrap: false, timestamps: true, tailLines: 200 }

interface LogCatalog {
  pods: Pod[]
  denied: number
  unknown: number
  permissionErrors: number
  complete: boolean
}

const CatalogPageSize = 500
const MaximumCatalogPods = 4000

function pageCoverageComplete(coverage: CollectionResult<Pod>['coverage']): boolean {
  return coverage === null || coverage.completedNamespaces === coverage.requestedNamespaces && coverage.deniedNamespaces.length === 0 && coverage.failed.length === 0
}

async function loadLogCatalog(selection: SelectionSummary, signal?: AbortSignal): Promise<LogCatalog> {
  const source: Pod[] = []
  let pagesComplete = true
  let coverageComplete = true
  let withinBudget = true
  let continueToken: string | undefined = undefined
  do {
    const page = await getPods({ limit: CatalogPageSize, ...(continueToken ? { continueToken } : {}) }, signal, selection.generation)
    source.push(...page.items)
    if (!page.page.complete || page.page.truncated) pagesComplete = false
    if (!pageCoverageComplete(page.coverage)) coverageComplete = false
    continueToken = page.page.next === '' ? undefined : page.page.next
    if (continueToken && source.length >= MaximumCatalogPods) withinBudget = false
  } while (continueToken && withinBudget)
  const namesByNamespace = new Map<string, Set<string>>()
  for (const pod of source) {
    const names = namesByNamespace.get(pod.namespace) ?? new Set<string>()
    names.add(pod.name)
    namesByNamespace.set(pod.namespace, names)
  }
  const batches: Array<{ namespace: string; names: string[] }> = []
  for (const [namespace, names] of namesByNamespace) {
    const values = [...names]
    for (let index = 0; index < values.length; index += 25) batches.push({ namespace, names: values.slice(index, index + 25) })
  }
  const decisions = new Map<string, 'allowed' | 'denied' | 'unknown'>()
  let permissionErrors = 0
  let permissionsComplete = true
  let nextBatch = 0
  const worker = async () => {
    while (nextBatch < batches.length) {
      const batch = batches[nextBatch++]
      try {
        const matrix = await getPermissions({ namespaces: [batch.namespace], capabilityIds: ['pods.logs.get'], resourceNames: batch.names }, signal, selection.generation)
        if (!matrix.complete || matrix.truncated || matrix.errors.length > 0) permissionsComplete = false
        permissionErrors += matrix.errors.length
        for (const name of batch.names) {
          const capability = matrix.decisions.find((value) => value.capabilityId === 'pods.logs.get' && value.namespace === batch.namespace && value.resourceName === name)
          decisions.set(`${batch.namespace}\0${name}`, capability?.decision ?? 'unknown')
        }
      } catch (error) {
        if (signal?.aborted || error instanceof APIError && error.code === 'GENERATION_CHANGED') throw error
        permissionsComplete = false
        permissionErrors += 1
        for (const name of batch.names) decisions.set(`${batch.namespace}\0${name}`, 'unknown')
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(4, batches.length) }, () => worker()))
  const pods = source.filter((pod) => decisions.get(`${pod.namespace}\0${pod.name}`) === 'allowed')
  const denied = source.filter((pod) => decisions.get(`${pod.namespace}\0${pod.name}`) === 'denied').length
  const unknown = source.length - pods.length - denied
  return {
    pods,
    denied,
    unknown,
    permissionErrors,
    complete: pagesComplete && coverageComplete && withinBudget && permissionsComplete,
  }
}

export function LogsPage() {
  const [params] = useSearchParams()
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal) })
  const selection = status.data?.selection ?? null

  return (
    <div className="resource-page logs-page">
      <header className="resource-header"><div><span className="eyebrow">cluster resources</span><h1>Logs</h1><p>Current, previous and bounded follow logs. Content stays in memory and is never persisted by the UI.</p></div></header>
      {status.isPending ? <StatePanel kind="loading" title="Loading active selection">The local service is resolving the current generation.</StatePanel>
        : status.isError ? <StatePanel kind="error" title="Selection unavailable">{message(status.error)}</StatePanel>
          : !selection ? <StatePanel kind="empty" title="Choose a Kubernetes context">Select a context and namespace scope before reading logs.</StatePanel>
            : <LogsWorkspace key={selection.generation} selection={selection} params={params} defaults={preferences.data?.logs ?? defaultLogPreferences} preferencesUnavailable={preferences.isError} />}
    </div>
  )
}

function LogsWorkspace({ selection, params, defaults, preferencesUnavailable }: { selection: SelectionSummary; params: URLSearchParams; defaults: Preferences['logs']; preferencesUnavailable: boolean }) {
  const [namespace, setNamespace] = useState(params.get('namespace') ?? '')
  const [pod, setPod] = useState(params.get('pod') ?? '')
  const [container, setContainer] = useState(params.get('container') ?? '')
  const [previous, setPrevious] = useState(false)
  const [timestamps, setTimestamps] = useState(defaults.timestamps)
  const [tailLines, setTailLines] = useState(defaults.tailLines)
  const [since, setSince] = useState('')
  const [search, setSearch] = useState('')
  const [wrap, setWrap] = useState(defaults.wrap)
  const [paused, setPaused] = useState(false)
  const [pausedAt, setPausedAt] = useState(0)
  const [followLines, setFollowLines] = useState<LogLine[]>([])
  const [follow, setFollow] = useState<FollowState>({ status: 'idle', message: 'Follow is stopped.' })
  const [isFollowing, setIsFollowing] = useState(false)
  const [clipboardMessage, setClipboardMessage] = useState('')
  const followAbortRef = useRef<AbortController | null>(null)
  const readAbortRef = useRef<AbortController | null>(null)
  const mountedRef = useRef(true)

  const catalog = useQuery({
    queryKey: ['resources', 'log-target-catalog', selection.generation],
    queryFn: ({ signal }) => loadLogCatalog(selection, signal),
  })
  const namespaces = [...new Set(catalog.data?.pods.map((value) => value.namespace) ?? [])].sort()
  const catalogPods = (catalog.data?.pods ?? []).filter((value) => value.namespace === namespace)
  const selectedPod = catalogPods.find((value) => value.name === pod)
  const podDetail = useQuery({
    queryKey: ['resources', 'pod-detail', selection.generation, selectedPod?.namespace, selectedPod?.name],
    queryFn: ({ signal }) => getPod(selectedPod!.namespace, selectedPod!.name, signal, selection.generation),
    enabled: Boolean(selectedPod),
  })
  const containerRecords = podDetail.data ? [...podDetail.data.containers, ...podDetail.data.initContainers, ...podDetail.data.ephemeralContainers] : []
  const containers = [...new Set(containerRecords.map((value) => value.spec.name))]
  const selectedContainer = containerRecords.find((value) => value.spec.name === container)
  const previousAvailable = Boolean(selectedContainer && selectedContainer.restartCount > 0)

  const read = useMutation({
    mutationFn: async () => {
      readAbortRef.current?.abort()
      const controller = new AbortController()
      readAbortRef.current = controller
      try {
        return await getPodLogs(namespace, pod, { container, previous, timestamps, tailLines, since: since || undefined }, controller.signal, selection.generation)
      } finally {
        if (readAbortRef.current === controller) readAbortRef.current = null
      }
    },
  })

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      followAbortRef.current?.abort()
      readAbortRef.current?.abort()
    }
  }, [])

  function stopFollow(reason = 'Follow stopped by the user.') {
    followAbortRef.current?.abort()
    followAbortRef.current = null
    setIsFollowing(false)
    setFollow({ status: 'ended', message: reason })
  }

  function changeTarget(update: () => void) {
    if (followAbortRef.current) stopFollow('Follow stopped because the target changed.')
    readAbortRef.current?.abort()
    read.reset()
    setFollowLines([])
    setPaused(false)
    update()
  }

  async function startFollow() {
    followAbortRef.current?.abort()
    setFollowLines([])
    setPaused(false)
    const controller = new AbortController()
    followAbortRef.current = controller
    setIsFollowing(true)
    setFollow({ status: 'connecting', message: 'Authorizing log follow…' })
    try {
      const session = await getSession(controller.signal)
      if (session.generation !== selection.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
      const response = await fetch(await streamURL(logURL(namespace, pod, container, timestamps, tailLines, since)), {
        method: 'GET',
        headers: { Accept: 'text/event-stream', 'X-KubePeep-CSRF': session.csrfToken },
        cache: 'no-store',
        credentials: 'same-origin',
        signal: controller.signal,
      })
      if (!response.ok) {
        const contentType = response.headers.get('content-type')?.toLowerCase() ?? ''
        const payload = contentType.startsWith('application/json') ? await response.json() as APIErrorPayload : { code: 'INVALID_RESPONSE', message: 'The stream guard returned an invalid response.' }
        throw new APIError(response.status, payload)
      }
      if (!response.headers.get('content-type')?.toLowerCase().startsWith('text/event-stream')) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The log stream used an unexpected content type.' })
      const reader = response.body?.getReader()
      if (!reader) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The log stream has no response body.' })
      setFollow({ status: 'following', message: 'Following sanitized log lines. Reconnects may contain a gap or duplicate.' })
      const decoder = new TextDecoder()
      let buffer = ''
      let metaSeen = false
      while (true) {
        const chunk = await reader.read()
        if (chunk.done) break
        buffer += decoder.decode(chunk.value, { stream: true })
        if (new TextEncoder().encode(buffer).byteLength > 136 * 1_024) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The log stream exceeded the bounded event buffer.' })
        while (true) {
          const separator = /\r?\n\r?\n/.exec(buffer)
          if (!separator) break
          const raw = buffer.slice(0, separator.index)
          buffer = buffer.slice(separator.index + separator[0].length)
          const event = parseSSEBlock(raw)
          if (!event) continue
          let payload: Record<string, unknown>
          try { payload = JSON.parse(event.data) as Record<string, unknown> } catch { throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The log stream sent invalid JSON.' }) }
          if (event.event === 'meta') {
            if (payload.generation !== selection.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The log stream belongs to another generation.' })
            metaSeen = true
          } else if (event.event === 'heartbeat') {
            if (payload.generation !== selection.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The log stream generation changed.' })
          } else if (event.event === 'line') {
            if (!metaSeen) throw new APIError(502, { code: 'INVALID_RESPONSE', message: 'The log stream sent data before metadata.' })
            const line: LogLine = { timestamp: typeof payload.timestamp === 'string' ? payload.timestamp : null, text: typeof payload.text === 'string' ? payload.text : '', truncated: payload.truncated === true }
            setFollowLines((lines) => appendBounded(lines, line))
          } else if (event.event === 'end') {
            setFollow({ status: 'ended', message: `Stream ended: ${String(payload.reason ?? 'upstream_eof')}.` })
            return
          } else if (event.event === 'error') {
            setFollow({ status: 'error', message: `${String(payload.code ?? 'STREAM_ERROR')}: ${String(payload.message ?? 'The stream ended.')}` })
            return
          }
        }
      }
      setFollow({ status: 'ended', message: 'The upstream stream closed.' })
    } catch (error) {
      if (!controller.signal.aborted && mountedRef.current) setFollow({ status: 'error', message: message(error) })
    } finally {
      if (followAbortRef.current === controller) followAbortRef.current = null
      if (mountedRef.current) setIsFollowing(false)
    }
  }

  const lines = follow.status === 'following' || followLines.length > 0 ? followLines : (read.data?.lines ?? [])
  const pausedLines = paused ? lines.slice(0, pausedAt) : lines
  const normalizedSearch = search.toLocaleLowerCase()
  const visibleLines = normalizedSearch === '' ? pausedLines : pausedLines.filter((line) => line.text.toLocaleLowerCase().includes(normalizedSearch))
  const ready = Boolean(selectedPod && containers.includes(container)) && Number.isInteger(tailLines) && tailLines >= 1 && tailLines <= 2_000 && validSince(since)

  function textValue(): string {
    return visibleLines.map((line) => `${timestamps && line.timestamp ? `${line.timestamp} ` : ''}${line.text}`).join('\n')
  }

  async function copyLogs() {
    try {
      await navigator.clipboard.writeText(textValue())
      setClipboardMessage('Visible logs copied.')
    } catch {
      setClipboardMessage('Clipboard access is unavailable.')
    }
  }

  function downloadLogs() {
    const url = URL.createObjectURL(new Blob([textValue()], { type: 'text/plain;charset=utf-8' }))
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${pod}-${container}.log`
    anchor.click()
    URL.revokeObjectURL(url)
  }

  return <>
    {preferencesUnavailable ? <p className="permission-notice">Saved log preferences are unavailable; safe in-memory defaults are active.</p> : null}
    {catalog.isPending ? <p className="permission-notice" role="status">Loading the bounded Pod catalog and exact pods.logs.get capabilities…</p>
      : catalog.isError ? <p className="field-error">Authorized log target catalog unavailable: {message(catalog.error)}</p>
        : catalog.data ? <p className={catalog.data.complete ? 'field-help' : 'permission-notice'} role="status">{catalog.data.complete
          ? `${catalog.data.pods.length} log-authorized Pod${catalog.data.pods.length === 1 ? '' : 's'} available in the complete bounded catalog.`
          : `Partial catalog: ${catalog.data.pods.length} log-authorized Pod${catalog.data.pods.length === 1 ? '' : 's'} from the bounded catalog; ${catalog.data.denied} denied and ${catalog.data.unknown} unknown. Some authorized targets may be absent.`}</p> : null}
    <section className="log-controls" aria-label="Log query">
      <label>Namespace<Select value={namespaces.includes(namespace) ? namespace : ''} disabled={catalog.isPending || namespaces.length === 0} onChange={(event) => changeTarget(() => { setNamespace(event.target.value); setPod(''); setContainer(''); setPrevious(false) })}><option value="">Choose an authorized namespace</option>{namespaces.map((value) => <option key={value}>{value}</option>)}</Select></label>
      <label>Pod<Select value={selectedPod?.name ?? ''} disabled={namespace === '' || catalogPods.length === 0} onChange={(event) => changeTarget(() => { setPod(event.target.value); setContainer(''); setPrevious(false) })}><option value="">Choose a log-authorized Pod</option>{catalogPods.map((value) => <option key={value.name}>{value.name}</option>)}</Select></label>
      <label>Container<Select value={containers.includes(container) ? container : ''} disabled={!selectedPod || podDetail.isPending || containers.length === 0} onChange={(event) => changeTarget(() => { setContainer(event.target.value); setPrevious(false) })}><option value="">Choose an authorized Pod container</option>{containers.map((value) => <option key={value}>{value}</option>)}</Select></label>
      <label>Tail lines<Input type="number" min="1" max="2000" value={tailLines} onChange={(event) => setTailLines(Number(event.target.value))} /></label>
      <label>Since<Input aria-invalid={!validSince(since)} placeholder="15m" pattern="[1-9][0-9]*(s|m|h)" value={since} onChange={(event) => setSince(event.target.value)} /></label>
      <label className="confirmation-check"><input type="checkbox" checked={previous} disabled={!previousAvailable} onChange={(event) => { if (followAbortRef.current) stopFollow('Follow stopped because previous logs were selected.'); setPrevious(event.target.checked) }} />Previous container</label>
      <label className="confirmation-check"><input type="checkbox" checked={timestamps} onChange={(event) => setTimestamps(event.target.checked)} />Timestamps</label>
      <div className="form-actions"><Button disabled={!ready || read.isPending} onClick={() => read.mutate()}>{read.isPending ? 'Reading…' : 'Read logs'}</Button><Button variant="secondary" disabled={!ready || previous || isFollowing} onClick={() => void startFollow()}>Follow</Button><Button variant="danger" disabled={!isFollowing} onClick={() => stopFollow()}>Stop</Button></div>
      {!validSince(since) ? <p className="field-error">Since must use one unit (s, m or h) and cannot exceed 4 hours.</p> : null}
      {podDetail.isError ? <p className="field-error">Container catalog unavailable: {message(podDetail.error)}</p> : null}
      {selectedContainer && !previousAvailable ? <p className="field-help">The authorized Pod detail reports no previous instance for this container.</p> : null}
    </section>
    <SavedFilterControls collection="logs" generation={selection.generation} currentQuery={{
      ...(namespace ? { namespace: [namespace] } : {}),
      ...(search ? { search } : {}),
    }} onApply={(query) => {
      const savedNamespaces = query.namespace
      const savedNamespace = Array.isArray(savedNamespaces) && typeof savedNamespaces[0] === 'string' ? savedNamespaces[0] : ''
      const savedSearch = typeof query.search === 'string' ? query.search : ''
      changeTarget(() => { setNamespace(savedNamespace); setPod(''); setContainer(''); setPrevious(false) })
      setSearch(savedSearch)
    }} />
    {read.isError ? <StatePanel kind="error" title="Log request failed">{message(read.error)}</StatePanel> : null}
    {read.data?.truncated ? <p className="permission-notice">The bounded log response was truncated by the server.</p> : null}
    <section className="log-viewer">
      <header><div><strong className={`follow-state follow-state--${follow.status}`} aria-live="polite">{follow.message}</strong><small>{visibleLines.length} visible of {pausedLines.length} line{pausedLines.length === 1 ? '' : 's'} kept in the bounded in-memory viewer.</small></div><div><Button size="compact" variant="secondary" onClick={() => { if (!paused) setPausedAt(lines.length); setPaused(!paused) }}>{paused ? 'Resume view' : 'Pause view'}</Button><Button size="compact" variant="secondary" onClick={() => setWrap(!wrap)}>{wrap ? 'Disable wrap' : 'Wrap lines'}</Button><Button size="compact" variant="secondary" disabled={visibleLines.length === 0} onClick={() => void copyLogs()}>Copy</Button><Button size="compact" variant="secondary" disabled={visibleLines.length === 0} onClick={downloadLogs}>Download</Button><Button size="compact" variant="secondary" disabled={lines.length === 0} onClick={() => { setFollowLines([]); read.reset(); setPaused(false) }}>Clear</Button></div></header>
      <label className="log-search">Search visible logs<Input type="search" data-app-shortcut="search" aria-keyshortcuts="Control+F Meta+F" value={search} maxLength={256} onChange={(event) => setSearch(event.target.value)} /></label>
      <pre className={`${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre'} min-h-[280px] max-h-[62vh] overflow-auto bg-kp-crust border border-kp-overlay-0 rounded-md p-3.5 text-kp-text text-xs leading-relaxed`} aria-label="Log output">{visibleLines.map((line, index) => <span key={`${index}-${line.timestamp ?? ''}`}>{timestamps && line.timestamp ? <time className="text-kp-overlay-text">{line.timestamp} </time> : null}{line.text}{line.truncated ? ' [truncated]' : ''}{'\n'}</span>)}</pre>
      {clipboardMessage ? <p className="field-help" role="status">{clipboardMessage}</p> : null}
    </section>
  </>
}
