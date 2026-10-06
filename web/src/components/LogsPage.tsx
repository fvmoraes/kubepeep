import { appendLogBatch } from './resource/boundedLogLines'
import { workloadKindPath } from '../navigation/paths'
import { ResourceTabStrip } from './resource/ResourceTabStrip'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'

import { APIError, getPermissions, getPod, getPodLogs, getPods, getPreferences, getSession, getStatus, getWorkload, getWorkloads } from '../api/client'
import { streamURL } from '../api/desktop'
import type { APIErrorPayload, CollectionResult, LogLine, Pod, Preferences, SelectionSummary, Workload } from '../api/types'
import { Badge, Button, Checkbox, Input, Select, type BadgeVariant } from '../components/ui'
import { ErrorBanner, InfoBanner, WarningBanner } from '../components/ui/Banner'
import { SavedFilterControls } from './SavedFilterControls'
import { StatePanel } from './StatePanel'
import { PanelErrorBoundary } from './PanelErrorBoundary'

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

function appendBounded<T extends LogLine>(lines: T[], line: T): T[] { return appendLogBatch(lines, [line]) }

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

type LogLevel = 'error' | 'warn' | 'info' | 'debug'

function isJSONObjectLike(text: string): unknown {
  const trimmed = text.trim()
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return null
  try {
    const value = JSON.parse(trimmed)
    if (value !== null && typeof value === 'object') return value
  } catch {
    // Not valid JSON; render as plain text.
  }
  return null
}

function HighlightedJSON({ value }: { value: unknown }) {
  if (value === null) return <span className="text-kp-red">null</span>
  if (typeof value === 'boolean') return <span className="text-kp-sky">{String(value)}</span>
  if (typeof value === 'number') return <span className="text-kp-peach">{String(value)}</span>
  if (typeof value === 'string') {
    return (
      <>
        <span className="text-kp-overlay-text">"</span>
        <span className="text-kp-green">{value}</span>
        <span className="text-kp-overlay-text">"</span>
      </>
    )
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return <span className="text-kp-overlay-text">[]</span>
    return (
      <span className="text-kp-overlay-text">
        [{value.map((item, index) => (
          <span key={index}>
            <HighlightedJSON value={item} />
            {index < value.length - 1 ? ', ' : ''}
          </span>
        ))}]
      </span>
    )
  }
  const entries = Object.entries(value as Record<string, unknown>)
  if (entries.length === 0) return <span className="text-kp-overlay-text">{'{}'}</span>
  return (
    <span className="text-kp-overlay-text">
      {'{ '}
      {entries.map(([key, item], index) => (
        <span key={key}>
          <span className="text-kp-overlay-text">"</span>
          <span className="text-kp-mauve">{key}</span>
          <span className="text-kp-overlay-text">"</span>
          : <HighlightedJSON value={item} />
          {index < entries.length - 1 ? ', ' : ''}
        </span>
      ))}
      {' }'}
    </span>
  )
}

function detectLogLevel(text: string): LogLevel | null {
  const match = /\b(err(?:or)?|warn(?:ing)?|info|debug)\b/i.exec(text)
  if (!match) return null
  const raw = match[1].toLowerCase()
  if (raw.startsWith('err')) return 'error'
  if (raw.startsWith('warn')) return 'warn'
  if (raw === 'info') return 'info'
  return 'debug'
}

function levelBadgeVariant(level: LogLevel | 'all'): BadgeVariant {
  switch (level) {
    case 'error':
      return 'danger'
    case 'warn':
      return 'warning'
    case 'info':
      return 'info'
    case 'debug':
      return 'default'
    default:
      return 'default'
  }
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
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal), staleTime: 60_000 })
  const selection = status.data?.selection ?? null
	const [mode, setMode] = useState<'pod' | 'workload'>(() => params.has('workload') ? 'workload' : 'pod')

  return (
    <div className="flex w-full min-w-0 flex-col gap-4">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl text-kp-text">Logs</h1>
          <p className="mt-0.5 text-sm text-kp-overlay-text">Current, previous and bounded follow logs. Content stays in memory and is never persisted by the UI.</p>
		</div>
		<ResourceTabStrip ariaLabel="Log target type" panelId="log-target-panel" tabs={[{ id: 'pod', label: 'Pod' }, { id: 'workload', label: 'Workload aggregate' }]} active={mode} onChange={(value) => setMode(value as 'pod' | 'workload')} />
      </header>
      <div id="log-target-panel" role="tabpanel" aria-label={mode === 'pod' ? 'Pod' : 'Workload aggregate'}>
      {status.isPending ? <StatePanel kind="loading" title="Loading active selection">The local service is resolving the current generation.</StatePanel>
        : status.isError ? <StatePanel kind="error" title="Selection unavailable">{message(status.error)}</StatePanel>
          : !selection ? <StatePanel kind="empty" title="Choose a Kubernetes context">Select a context and namespace scope before reading logs.</StatePanel>
			: <PanelErrorBoundary key={`${selection.generation}-${mode}`} name="Logs">{mode === 'pod' ? <LogsWorkspace selection={selection} params={params} defaults={preferences.data?.logs ?? defaultLogPreferences} preferencesUnavailable={preferences.isError} /> : <WorkloadLogsWorkspace selection={selection} params={params} defaults={preferences.data?.logs ?? defaultLogPreferences} />}</PanelErrorBoundary>}
      </div>
    </div>
  )
}

const MaximumAggregateStreams = 5

interface AggregatedLogLine extends LogLine { pod: string; container: string }

async function loadWorkloadCatalog(selection: SelectionSummary, signal?: AbortSignal): Promise<{ values: Workload[]; complete: boolean }> {
	const values: Workload[] = []
	let complete = true
	let continueToken: string | undefined
	do {
		const page = await getWorkloads({ limit: 500, ...(continueToken ? { continueToken } : {}) }, signal, selection.generation)
		values.push(...page.items)
		complete = complete && page.page.complete && !page.page.truncated && (page.coverage === null || page.coverage.deniedNamespaces.length === 0 && page.coverage.failed.length === 0)
		continueToken = page.page.next || undefined
		if (values.length >= 4_000 && continueToken) { complete = false; break }
	} while (continueToken)
	return { values, complete }
}

async function consumeAggregateStream(target: { namespace: string; pod: string; container: string }, selection: SelectionSummary, session: { csrfToken: string }, options: { timestamps: boolean; tailLines: number; since: string }, signal: AbortSignal, onLine: (line: AggregatedLogLine) => void) {
	const url = await streamURL(logURL(target.namespace, target.pod, target.container, options.timestamps, options.tailLines, options.since))
	signal.throwIfAborted()
	const response = await fetch(url, { method: 'GET', headers: { Accept: 'text/event-stream', 'X-KubePeep-CSRF': session.csrfToken }, cache: 'no-store', credentials: 'same-origin', signal })
	if (!response.ok || !response.body) throw new APIError(response.status, { code: 'STREAM_ERROR', message: 'An aggregate log stream could not be opened.' })
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

function WorkloadLogsWorkspace({ selection, params, defaults }: { selection: SelectionSummary; params: URLSearchParams; defaults: Preferences['logs'] }) {
	const [workloadKey, setWorkloadKey] = useState(() => params.get('workload')?.split('/').join('\0') ?? '')
	const [selectedPods, setSelectedPods] = useState<string[] | null>(null)
	const [selectedContainers, setSelectedContainers] = useState<string[] | null>(null)
	const [previous, setPrevious] = useState(false)
	const [timestamps, setTimestamps] = useState(defaults.timestamps)
	const [tailLines, setTailLines] = useState(defaults.tailLines)
	const [since, setSince] = useState('')
	const [search, setSearch] = useState('')
	const [regex, setRegex] = useState(false)
	const [lines, setLines] = useState<AggregatedLogLine[]>([])
	const [state, setState] = useState<FollowState>({ status: 'idle', message: 'Aggregate follow is stopped.' })
	const controllerRef = useRef<AbortController | null>(null)
	const pendingRef = useRef<AggregatedLogLine[]>([])
	const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
	const workloads = useQuery({ queryKey: ['resources', 'workload-log-catalog', selection.generation], queryFn: ({ signal }) => loadWorkloadCatalog(selection, signal) })
	const podCatalog = useQuery({ queryKey: ['resources', 'log-target-catalog', selection.generation], queryFn: ({ signal }) => loadLogCatalog(selection, signal) })
	const selectedWorkload = workloads.data?.values.find((value) => `${value.kind}\0${value.namespace}\0${value.name}` === workloadKey)
	const detail = useQuery({ queryKey: ['resources', 'workload-log-detail', selection.generation, workloadKey], queryFn: ({ signal }) => getWorkload(workloadKindPath(selectedWorkload!.kind)!, selectedWorkload!.namespace, selectedWorkload!.name, signal, selection.generation), enabled: Boolean(selectedWorkload) })
	const relatedPods = (detail.data?.related ?? []).filter((value) => value.kind === 'Pod' && value.namespace === selectedWorkload?.namespace).map((value) => value.name).filter((name) => podCatalog.data?.pods.some((pod) => pod.namespace === selectedWorkload?.namespace && pod.name === name)).sort()
	const activePods = (selectedPods ?? relatedPods.slice(0, MaximumAggregateStreams)).filter((name) => relatedPods.includes(name))
	const podDetails = useQuery({ queryKey: ['resources', 'aggregate-pod-details', selection.generation, selectedWorkload?.namespace, activePods], queryFn: ({ signal }) => Promise.all(activePods.map((name) => getPod(selectedWorkload!.namespace, name, signal, selection.generation))), enabled: Boolean(selectedWorkload && activePods.length > 0) })
	const containers = [...new Set((podDetails.data ?? []).flatMap((value) => [...value.containers, ...value.initContainers, ...value.ephemeralContainers].map((container) => container.spec.name)))].sort()
	const activeContainers = (selectedContainers ?? containers).filter((value) => containers.includes(value))

	function flush() { if (timerRef.current) clearTimeout(timerRef.current); timerRef.current=null; const batch=pendingRef.current;pendingRef.current=[];if(batch.length>0)setLines((current)=>appendLogBatch(current, batch)) }
	function stop(reason='Aggregate follow stopped.') {controllerRef.current?.abort();controllerRef.current=null;flush();setState({status:'ended',message:reason})}
	useEffect(() => () => {controllerRef.current?.abort();controllerRef.current=null;if(timerRef.current)clearTimeout(timerRef.current);pendingRef.current=[]}, [])
	const compatibleTargets = activePods.flatMap((pod) => {
		const detail = podDetails.data?.find((value) => value.metadata.namespace === selectedWorkload?.namespace && value.metadata.name === pod)
		const available = new Set(detail ? [...detail.containers, ...detail.initContainers, ...detail.ephemeralContainers].map((value) => value.spec.name) : [])
		return activeContainers.filter((container) => available.has(container)).map((container) => ({ namespace: selectedWorkload?.namespace ?? '', pod, container }))
	})
	const targets = compatibleTargets.slice(0, MaximumAggregateStreams)
	const targetsTruncated = compatibleTargets.length > MaximumAggregateStreams

	async function readAggregate() {
		stop('Starting bounded aggregate read.')
		setState({ status: 'connecting', message: 'Reading selected streams…' })
		setLines([])
		const controller = new AbortController()
		controllerRef.current = controller
		try {
			const responses = await Promise.all(targets.map(async (target) => ({ target, response: await getPodLogs(target.namespace, target.pod, { container: target.container, previous, timestamps, tailLines, since: since || undefined }, controller.signal, selection.generation) })))
			if (controller.signal.aborted || controllerRef.current !== controller) return
			const next = responses.flatMap(({ target, response }) => response.lines.map((line) => ({ ...line, pod: target.pod, container: target.container })))
			next.sort((left, right) => (left.timestamp ?? '').localeCompare(right.timestamp ?? ''))
			setLines(appendLogBatch([], next))
			setState({ status: 'ended', message: `Read ${targets.length} bounded stream${targets.length === 1 ? '' : 's'}.` })
		} catch (error) {
			if (!controller.signal.aborted && controllerRef.current === controller) {
				controller.abort()
				setState({ status: 'error', message: message(error) })
			}
		} finally {
			controller.abort()
			if (controllerRef.current === controller) controllerRef.current = null
		}
	}
	async function followAggregate() {
		stop('Restarting aggregate follow.')
		setLines([])
		const controller = new AbortController()
		controllerRef.current = controller
		setState({ status: 'connecting', message: 'Authorizing aggregate streams…' })
		try {
			const session = await getSession(controller.signal)
			if (controller.signal.aborted || controllerRef.current !== controller) return
			if (session.generation !== selection.generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
			setState({ status: 'following', message: `Following ${targets.length} stream${targets.length === 1 ? '' : 's'} in real time.` })
			await Promise.all(targets.map((target) => consumeAggregateStream(target, selection, session, { timestamps, tailLines, since }, controller.signal, (line) => {
				if (controller.signal.aborted || controllerRef.current !== controller) return
				pendingRef.current = appendBounded(pendingRef.current, line)
				if (!timerRef.current) timerRef.current = setTimeout(flush, 75)
			})))
			if (controller.signal.aborted || controllerRef.current !== controller) return
			flush()
			setState({ status: 'ended', message: 'All aggregate streams ended.' })
		} catch (error) {
			if (!controller.signal.aborted && controllerRef.current === controller) {
				controller.abort()
				flush()
				setState({ status: 'error', message: message(error) })
			}
		} finally {
			controller.abort()
			if (controllerRef.current === controller) controllerRef.current = null
		}
	}
	let expression: RegExp | null=null;let regexError='';if(regex&&search){try{expression=new RegExp(search,'i')}catch{regexError='Invalid regular expression.'}}
	const visible=search===''?lines:regex?(expression?lines.filter((line)=>expression!.test(line.text)):[]):lines.filter((line)=>line.text.toLocaleLowerCase().includes(search.toLocaleLowerCase()))
	const ready=Boolean(selectedWorkload&&targets.length>0&&validSince(since)&&tailLines>=1&&tailLines<=2_000)
	return <div className="grid gap-3">
		{workloads.data&&!workloads.data.complete?<WarningBanner>Workload catalog coverage is partial; absent workloads may exist outside the loaded pages.</WarningBanner>:null}
		{podCatalog.data&&!podCatalog.data.complete?<WarningBanner>Only Pods with confirmed logs permission in the bounded catalog are offered.</WarningBanner>:null}
		<section className="grid gap-3 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-3">
			<label className="grid gap-1"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Workload</span><Select value={workloadKey} onChange={(event)=>{stop('Aggregate target changed; all streams were canceled.');setLines([]);setSelectedPods(null);setSelectedContainers(null);setWorkloadKey(event.target.value)}}><option value="">Choose a workload</option>{workloads.data?.values.map((value)=><option key={`${value.kind}\0${value.namespace}\0${value.name}`} value={`${value.kind}\0${value.namespace}\0${value.name}`}>{value.kind} · {value.namespace} · {value.name}</option>)}</Select></label>
			<div><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Pods ({activePods.length}/{relatedPods.length})</span><div className="mt-1 flex flex-wrap gap-2">{relatedPods.map((pod)=><Checkbox key={pod} checked={activePods.includes(pod)} disabled={!activePods.includes(pod)&&activePods.length>=MaximumAggregateStreams} onChange={(event)=>{stop();setSelectedPods(event.target.checked?[...activePods,pod]:activePods.filter((value)=>value!==pod))}}>{pod}</Checkbox>)}</div></div>
			<div><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Containers</span><div className="mt-1 flex flex-wrap gap-2">{containers.map((container)=><Checkbox key={container} checked={activeContainers.includes(container)} onChange={(event)=>{stop();setSelectedContainers(event.target.checked?[...activeContainers,container]:activeContainers.filter((value)=>value!==container))}}>{container}</Checkbox>)}</div></div>
			<div className="flex flex-wrap items-end gap-2"><label className="grid gap-1 w-24"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Tail</span><Input type="number" min="1" max="2000" value={tailLines} onChange={(event)=>setTailLines(Number(event.target.value))}/></label><label className="grid gap-1 w-28"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Since</span><Input value={since} placeholder="15m" onChange={(event)=>setSince(event.target.value)}/></label><Checkbox checked={previous} onChange={(event)=>{stop();setPrevious(event.target.checked)}}>Previous</Checkbox><Checkbox checked={timestamps} onChange={(event)=>setTimestamps(event.target.checked)}>Timestamps</Checkbox></div>
			{targetsTruncated?<WarningBanner>Selection creates more than {MaximumAggregateStreams} streams. Only the first {MaximumAggregateStreams} deterministic Pod/container pairs will run.</WarningBanner>:null}
			<div className="flex gap-2"><Button disabled={!ready} onClick={()=>void readAggregate()}>Read aggregate</Button><Button variant="success" disabled={!ready||previous||state.status==='following'} onClick={()=>void followAggregate()}>Follow aggregate</Button><Button variant="danger" disabled={state.status!=='connecting'&&state.status!=='following'} onClick={()=>stop()}>Stop all</Button></div>
		</section>
		<section className="grid gap-2 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-3"><header><strong className="text-sm text-kp-text">{state.message}</strong><small className="block text-xs text-kp-overlay-text">{visible.length} visible of {lines.length} bounded in-memory lines.</small></header><div className="flex gap-2"><Input type="search" aria-label="Search aggregate logs" placeholder="Text or regular expression" value={search} onChange={(event)=>setSearch(event.target.value)}/><Checkbox checked={regex} onChange={(event)=>setRegex(event.target.checked)}>Regex</Checkbox></div>{regexError?<p className="text-xs text-kp-red">{regexError}</p>:null}<pre className={`mono min-h-[280px] max-h-[62vh] overflow-auto rounded-lg border border-kp-overlay-0 bg-kp-crust p-3 text-xs ${defaults.wrap?'whitespace-pre-wrap break-words':'whitespace-pre'}`} aria-label="Aggregated log output">{visible.map((line,index)=><span key={`${index}-${line.pod}-${line.container}`}><time className="text-kp-overlay-text">{line.timestamp??'no-timestamp'} </time><strong className="text-kp-mauve">{line.pod}</strong> <span className="text-kp-sky">{line.container}</span> │ {line.text}{line.truncated?' [truncated]':''}{'\n'}</span>)}</pre></section>
	</div>
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
  const [followLines, setFollowLines] = useState<LogLine[]>([])
  const [followBuffer, setFollowBuffer] = useState<LogLine[]>([])
  const [follow, setFollow] = useState<FollowState>({ status: 'idle', message: 'Follow is stopped.' })
  const [isFollowing, setIsFollowing] = useState(false)
  const [clipboardMessage, setClipboardMessage] = useState('')
  const [levelFilter, setLevelFilter] = useState<LogLevel | 'all'>('all')
  const followAbortRef = useRef<AbortController | null>(null)
  const readAbortRef = useRef<AbortController | null>(null)
  const mountedRef = useRef(true)
  const followBufferRef = useRef<LogLine[]>([])
  const pendingFollowLinesRef = useRef<LogLine[]>([])
  const followFlushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pausedRef = useRef(paused)

  useEffect(() => { pausedRef.current = paused }, [paused])

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
      if (followFlushTimerRef.current) clearTimeout(followFlushTimerRef.current)
    }
  }, [])

  function clearBuffer() {
    if (followFlushTimerRef.current) clearTimeout(followFlushTimerRef.current)
    followFlushTimerRef.current = null
    pendingFollowLinesRef.current = []
    followBufferRef.current = []
    setFollowBuffer([])
  }

  function flushPendingLines() {
    if (followFlushTimerRef.current) clearTimeout(followFlushTimerRef.current)
    followFlushTimerRef.current = null
    const batch = pendingFollowLinesRef.current
    if (batch.length === 0) return
    pendingFollowLinesRef.current = []
    if (pausedRef.current) {
      const next = appendLogBatch(followBufferRef.current, batch)
      followBufferRef.current = next
      setFollowBuffer(next)
    } else {
      setFollowLines((lines) => appendLogBatch(lines, batch))
    }
  }

  function flushBuffer() {
    flushPendingLines()
    const buffer = followBufferRef.current
    if (buffer.length === 0) return
    setFollowLines((lines) => appendLogBatch(lines, buffer))
    followBufferRef.current = []
    setFollowBuffer([])
  }

  function stopFollow(reason = 'Follow stopped by the user.') {
    followAbortRef.current?.abort()
    followAbortRef.current = null
    flushBuffer()
    setPaused(false)
    setIsFollowing(false)
    setFollow({ status: 'ended', message: reason })
  }

  function changeTarget(update: () => void) {
    if (followAbortRef.current) stopFollow('Follow stopped because the target changed.')
    readAbortRef.current?.abort()
    read.reset()
    setFollowLines([])
    clearBuffer()
    setPaused(false)
    update()
  }

  async function startFollow() {
    followAbortRef.current?.abort()
    setFollowLines([])
    clearBuffer()
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
            pendingFollowLinesRef.current = appendBounded(pendingFollowLinesRef.current, line)
            if (!followFlushTimerRef.current) followFlushTimerRef.current = setTimeout(flushPendingLines, 75)
          } else if (event.event === 'end') {
            flushBuffer()
            setPaused(false)
            setFollow({ status: 'ended', message: `Stream ended: ${String(payload.reason ?? 'upstream_eof')}.` })
            return
          } else if (event.event === 'error') {
            flushBuffer()
            setPaused(false)
            setFollow({ status: 'error', message: `${String(payload.code ?? 'STREAM_ERROR')}: ${String(payload.message ?? 'The stream ended.')}` })
            return
          }
        }
      }
      setFollow({ status: 'ended', message: 'The upstream stream closed.' })
    } catch (error) {
      if (!controller.signal.aborted && mountedRef.current) {
        flushBuffer()
        setPaused(false)
        setFollow({ status: 'error', message: message(error) })
      }
    } finally {
      if (followAbortRef.current === controller) followAbortRef.current = null
      if (mountedRef.current) setIsFollowing(false)
    }
  }

  const lines = follow.status === 'following' || followLines.length > 0 ? followLines : (read.data?.lines ?? [])
  const keptLines = lines
  const normalizedSearch = search.toLocaleLowerCase()
  const searchMatchedLines = normalizedSearch === '' ? keptLines : keptLines.filter((line) => line.text.toLocaleLowerCase().includes(normalizedSearch))
  const visibleLines = levelFilter === 'all' ? searchMatchedLines : searchMatchedLines.filter((line) => detectLogLevel(line.text) === levelFilter)
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
    {preferencesUnavailable ? <InfoBanner className="mb-3">Saved log preferences are unavailable; safe in-memory defaults are active.</InfoBanner> : null}
    {catalog.isPending ? <p className="rounded-r-md border-l-2 border-kp-blue-border bg-kp-blue-bg px-3 py-2 text-sm text-kp-sky" role="status">Loading the bounded Pod catalog and exact pods.logs.get capabilities…</p>
      : catalog.isError ? <ErrorBanner title="Authorized log target catalog unavailable">{message(catalog.error)}</ErrorBanner>
        : catalog.data ? <p className={catalog.data.complete ? 'text-xs text-kp-overlay-text' : 'rounded-r-md border-l-2 border-kp-yellow-border bg-kp-yellow-bg px-3 py-2 text-sm text-kp-yellow'} role="status">{catalog.data.complete
          ? `${catalog.data.pods.length} log-authorized Pod${catalog.data.pods.length === 1 ? '' : 's'} available in the complete bounded catalog.`
          : `Partial catalog: ${catalog.data.pods.length} log-authorized Pod${catalog.data.pods.length === 1 ? '' : 's'} from the bounded catalog; ${catalog.data.denied} denied and ${catalog.data.unknown} unknown. Some authorized targets may be absent.`}</p> : null}
    <section aria-label="Log query" className="flex flex-wrap items-end gap-2.5 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-3">
      <label className="grid flex-1 gap-1 min-w-[150px]"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Namespace</span><Select value={namespaces.includes(namespace) ? namespace : ''} disabled={catalog.isPending || namespaces.length === 0} onChange={(event) => changeTarget(() => { setNamespace(event.target.value); setPod(''); setContainer(''); setPrevious(false) })}><option value="">Choose an authorized namespace</option>{namespaces.map((value) => <option key={value}>{value}</option>)}</Select></label>
      <label className="grid flex-1 gap-1 min-w-[150px]"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Pod</span><Select value={selectedPod?.name ?? ''} disabled={namespace === '' || catalogPods.length === 0} onChange={(event) => changeTarget(() => { setPod(event.target.value); setContainer(''); setPrevious(false) })}><option value="">Choose a log-authorized Pod</option>{catalogPods.map((value) => <option key={value.name}>{value.name}</option>)}</Select></label>
      <label className="grid flex-1 gap-1 min-w-[150px]"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Container</span><Select value={containers.includes(container) ? container : ''} disabled={!selectedPod || podDetail.isPending || containers.length === 0} onChange={(event) => changeTarget(() => { setContainer(event.target.value); setPrevious(false) })}><option value="">Choose an authorized Pod container</option>{containers.map((value) => <option key={value}>{value}</option>)}</Select></label>
      <label className="grid gap-1 w-24"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Tail lines</span><Input type="number" min="1" max="2000" value={tailLines} onChange={(event) => setTailLines(Number(event.target.value))} /></label>
      <label className="grid gap-1 w-28"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Since</span><Input aria-invalid={!validSince(since)} placeholder="15m" pattern="[1-9][0-9]*(s|m|h)" value={since} onChange={(event) => setSince(event.target.value)} /></label>
      <Checkbox checked={previous} disabled={!previousAvailable} onChange={(event) => { if (followAbortRef.current) stopFollow('Follow stopped because previous logs were selected.'); setPrevious(event.target.checked) }}>Previous container</Checkbox>
      <Checkbox checked={timestamps} onChange={(event) => setTimestamps(event.target.checked)}>Timestamps</Checkbox>
      <div className="flex w-full flex-wrap gap-2">
        <Button disabled={!ready || read.isPending} onClick={() => read.mutate()}>{read.isPending ? 'Reading…' : 'Read logs'}</Button>
        <Button variant="success" disabled={!ready || previous || isFollowing} onClick={() => void startFollow()}>Follow</Button>
        <Button variant="danger" disabled={!isFollowing} onClick={() => stopFollow()}>Stop</Button>
      </div>
      {!validSince(since) ? <p className="w-full text-xs text-kp-red">Since must use one unit (s, m or h) and cannot exceed 4 hours.</p> : null}
      {podDetail.isError ? <p className="w-full text-xs text-kp-red">Container catalog unavailable: {message(podDetail.error)}</p> : null}
      {selectedContainer && !previousAvailable ? <p className="w-full text-xs text-kp-overlay-text">The authorized Pod detail reports no previous instance for this container.</p> : null}
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
    {read.data?.truncated ? <WarningBanner>The bounded log response was truncated by the server.</WarningBanner> : null}
    <section className="grid gap-2.5 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-3.5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div><strong className={`block text-sm ${follow.status === 'following' ? 'text-kp-green' : follow.status === 'connecting' ? 'text-kp-sky' : follow.status === 'error' ? 'text-kp-red' : 'text-kp-subtext'}`} aria-live="polite">{paused ? `${follow.message} (paused)` : follow.message}</strong><small className="mt-0.5 block text-xs text-kp-overlay-text">{visibleLines.length} visible of {keptLines.length} line{keptLines.length === 1 ? '' : 's'} kept in the bounded in-memory viewer{paused && followBuffer.length > 0 ? ` · ${followBuffer.length} buffered` : ''}.</small></div>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          <Button size="sm" variant="secondary" disabled={!isFollowing && !paused} aria-label={paused ? 'Continue following logs' : 'Pause following logs'} onClick={() => { if (paused) { flushBuffer(); setPaused(false) } else { setPaused(true) } }}>{paused ? 'Continue' : 'Pause'}</Button>
          <Button size="sm" variant="secondary" onClick={() => setWrap(!wrap)}>{wrap ? 'Disable wrap' : 'Wrap lines'}</Button>
          <Button size="sm" variant="secondary" disabled={visibleLines.length === 0} onClick={() => void copyLogs()}>Copy</Button>
          <Button size="sm" variant="secondary" disabled={visibleLines.length === 0} onClick={downloadLogs}>Download</Button>
          <Button size="sm" variant="secondary" disabled={lines.length === 0} onClick={() => { setFollowLines([]); clearBuffer(); read.reset(); setPaused(false) }}>Clear</Button>
          <label className="grid gap-1 w-24"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Level</span><Select value={levelFilter} aria-label="Filter by log level" onChange={(event) => setLevelFilter(event.target.value as LogLevel | 'all')}><option value="all">all</option><option value="error">error</option><option value="warn">warn</option><option value="info">info</option><option value="debug">debug</option></Select></label>
          {levelFilter !== 'all' ? <Badge variant={levelBadgeVariant(levelFilter)} className={levelFilter === 'debug' ? 'text-kp-mauve border-kp-mauve-muted' : ''}>{levelFilter}</Badge> : null}
        </div>
      </header>
      <label className="grid max-w-[420px] gap-1"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Search visible logs</span><Input type="search" data-app-shortcut="search" aria-label="Search visible logs" aria-keyshortcuts="Control+F Meta+F" value={search} maxLength={256} onChange={(event) => setSearch(event.target.value)} /></label>
      <pre className={`mono min-h-[280px] max-h-[62vh] overflow-auto rounded-lg border border-kp-overlay-0 bg-kp-crust p-3.5 text-xs leading-relaxed text-kp-text ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre'}`} aria-label="Log output">{visibleLines.map((line, index) => {
        const jsonValue = isJSONObjectLike(line.text)
        return <span key={`${index}-${line.timestamp ?? ''}`}>{timestamps && line.timestamp ? <time className="text-kp-overlay-text">{line.timestamp} </time> : null}{jsonValue ? <HighlightedJSON value={jsonValue} /> : line.text}{line.truncated ? ' [truncated]' : ''}{'\n'}</span>
      })}</pre>
      {clipboardMessage ? <p className="text-xs text-kp-overlay-text" role="status">{clipboardMessage}</p> : null}
    </section>
  </>
}
