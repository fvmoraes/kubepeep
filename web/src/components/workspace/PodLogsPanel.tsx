import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { APIError, getPod, getPodLogs, getSession } from '../../api/client'
import type { SelectionSummary } from '../../api/types'
import { appendLogBatch } from '../resource/boundedLogLines'
import { consumeAggregateStream, type AggregatedLogLine, type LogTarget } from '../resource/podLogStream'
import { followPodLogs, logAccessLost } from '../resource/livePodLogs'
import { formatLogLine, LogCapture, saveLogFile } from '../resource/logDownload'
import { Button, Input, Select } from '../ui'

export interface PodLogTarget { namespace: string; name: string }

/** Reads only explicit table targets; never scans the Pod/workload catalogs. */
export function PodLogsPanel({ pods, selection }: { pods: PodLogTarget[]; selection: SelectionSummary }) {
  const [container, setContainer] = useState('')
  const [previous, setPrevious] = useState(false)
  const identity = JSON.stringify(pods.slice(0, 5))
  const details = useQuery({
    queryKey: ['inline-log-targets', selection.generation, identity],
    queryFn: ({ signal }) => Promise.all((JSON.parse(identity) as PodLogTarget[]).map((pod) => getPod(pod.namespace, pod.name, signal, selection.generation))),
    retry: false,
  })
  const targets = useMemo(() => (details.data ?? []).flatMap((pod) => {
    const containers = [...pod.containers, ...pod.initContainers, ...pod.ephemeralContainers]
    const chosen = container === '*' ? containers : container ? containers.filter((item) => item.spec.name === container) : containers.slice(0, 1)
    return chosen.map((item) => ({ namespace: pod.metadata.namespace!, pod: pod.metadata.name, container: item.spec.name }))
  }), [details.data, container])
  const targetKey = JSON.stringify(targets.slice(0, 5))
  const containers = [...new Set((details.data ?? []).flatMap((pod) => [...pod.containers, ...pod.initContainers, ...pod.ephemeralContainers].map((item) => item.spec.name)))]
  return <PodLogSession key={`${selection.generation}|${targetKey}|${previous}`} targetKey={targetKey} generation={selection.generation} previous={previous} setPrevious={setPrevious} container={container} setContainer={setContainer} containers={containers} aggregate={pods.length > 1} limited={pods.length > 5 || targets.length > 5} detailError={details.error?.message} />
}

function PodLogSession({ targetKey, generation, previous, setPrevious, container, setContainer, containers, aggregate, limited, detailError }: {
  targetKey: string; generation: string; previous: boolean; setPrevious: (value: boolean) => void
  container: string; setContainer: (value: string) => void; containers: string[]; aggregate: boolean; limited: boolean; detailError?: string
}) {
  const targets = useMemo(() => JSON.parse(targetKey) as LogTarget[], [targetKey])
  const [search, setSearch] = useState('')
  const [paused, setPaused] = useState(false)
  const pausedRef = useRef(false)
  const [wrap, setWrap] = useState(false)
  const [followTail, setFollowTail] = useState(true)
  const [lines, setLines] = useState<AggregatedLogLine[]>([])
  const buffer = useRef<AggregatedLogLine[]>([])
  const capture = useRef(new LogCapture())
  const [captureState, setCaptureState] = useState({ lines: 0, truncated: false })
  const [status, setStatus] = useState('Connecting…')
  const [error, setError] = useState('')
  const [downloadScope, setDownloadScope] = useState('session')
  const [downloading, setDownloading] = useState(false)
  const [downloadNote, setDownloadNote] = useState('')
  const downloadController = useRef<AbortController | null>(null)
  const output = useRef<HTMLPreElement>(null)
  const visible = useMemo(() => lines.filter((line) => line.text.toLocaleLowerCase().includes(search.toLocaleLowerCase())), [lines, search])
  useEffect(() => {
    if (!targets.length) return
    const controller = new AbortController()
    const states = new Map<string, string>()
    let timer: ReturnType<typeof setTimeout> | undefined
    let pending: AggregatedLogLine[] = []
    const flush = () => {
      timer = undefined
      if (controller.signal.aborted || !pending.length) return
      buffer.current = appendLogBatch(buffer.current, pending)
      pending = []
      setCaptureState({ lines: capture.current.lines, truncated: capture.current.truncated })
      if (!pausedRef.current) setLines(buffer.current)
    }
    const accept = (target: LogTarget, line: AggregatedLogLine) => {
      if (controller.signal.aborted) return
      const identified = { ...line, pod: `${target.namespace}/${target.pod}` }
      capture.current.append(identified)
      pending = appendLogBatch(pending, [identified])
      if (!timer) timer = setTimeout(flush, 75)
    }
    const updateStatus = (target: LogTarget, value: string) => {
      states.set(JSON.stringify(target), value)
      const all = [...states.values()]
      setStatus(all.length < targets.length ? 'Connecting…' : all.find((state) => state !== 'Live') ?? 'Live')
    }
    const clearCapture = () => {
      pending = []; buffer.current = []; capture.current = new LogCapture(); setLines([])
      setCaptureState({ lines: 0, truncated: false })
      downloadController.current?.abort(); downloadController.current = null; setDownloading(false)
    }
    const run = async (target: LogTarget) => {
      if (previous) {
        const result = await getPodLogs(target.namespace, target.pod, { container: target.container, previous: true, timestamps: true, tailLines: 2000 }, controller.signal, generation)
        controller.signal.throwIfAborted()
        result.lines.forEach((line) => accept(target, { ...line, pod: target.pod, container: target.container }))
        updateStatus(target, 'Previous container')
      } else await followPodLogs(target, generation, controller.signal, (line) => accept(target, line), (value) => updateStatus(target, value), clearCapture)
    }
    void Promise.all(targets.map(run)).catch((cause: unknown) => {
      if (controller.signal.aborted) return
      flush()
      if (logAccessLost(cause)) {
        clearCapture()
      }
      setError(cause instanceof Error ? cause.message : 'Logs unavailable.')
      setStatus('Disconnected')
      controller.abort()
    })
    return () => { controller.abort(); clearTimeout(timer); pending = []; downloadController.current?.abort() }
  }, [targets, generation, previous])
  useLayoutEffect(() => {
    if (followTail && !paused && output.current) output.current.scrollTop = output.current.scrollHeight
  }, [visible, followTail, paused, wrap])
  useEffect(() => {
    const element = output.current
    if (!element || !followTail || paused || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(() => { element.scrollTop = element.scrollHeight })
    observer.observe(element)
    return () => observer.disconnect()
  }, [followTail, paused])

  async function download() {
    const stamp = new Date().toISOString().replace(/[:]/g, '-')
    const name = `${targets.length === 1 ? targets[0].pod : 'pods'}-${downloadScope}-${stamp}`
    setDownloadNote('')
    if (downloadScope === 'visible') {
      saveLogFile(new Blob(visible.map(formatLogLine), { type: 'text/plain;charset=utf-8' }), `${name}.log`)
      return
    }
    if (downloadScope === 'session') {
      saveLogFile(capture.current.blob(), `${name}${capture.current.truncated ? '-partial' : ''}.log`)
      if (capture.current.truncated) setDownloadNote('Session download is partial: a capture or line limit was reached.')
      return
    }
    const controller = new AbortController()
    downloadController.current = controller
    setDownloading(true)
    const exported = new LogCapture()
    let partial = false
    try {
      const session = await getSession(controller.signal)
      if (session.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
      for (const target of targets) {
        const terminal = await consumeAggregateStream(target, { generation }, session, { download: true, previous: downloadScope === 'previous', timestamps: true, tailLines: 200, since: '' }, controller.signal, (line) => {
          if (!exported.append({ ...line, pod: `${target.namespace}/${target.pod}` })) throw new Error('Download exceeds 64 MiB. Select fewer containers or download the current session.')
        })
        if (!['completed', 'limit_reached'].includes(terminal.reason)) throw new Error('The download was interrupted. Try again.')
        partial ||= terminal.truncated
      }
      controller.signal.throwIfAborted()
      partial ||= exported.truncated
      saveLogFile(exported.blob(), `${name}${partial ? '-partial' : ''}.log`)
      if (partial) setDownloadNote('Download is partial: the server reported a size or line limit.')
    } catch (cause) {
      if (!controller.signal.aborted) setDownloadNote(cause instanceof Error ? cause.message : 'Download failed.')
    } finally {
      if (downloadController.current === controller) { downloadController.current = null; if (!controller.signal.aborted) setDownloading(false) }
    }
  }
  return <section className="pod-logs-panel grid min-w-0 gap-2" aria-label={aggregate ? 'Selected Pod logs' : `Logs for ${targets[0]?.pod ?? 'Pod'}`}>
    <div className="pod-logs-controls flex items-center gap-2">
      <Select className="!w-auto max-w-full" aria-label="Log container" value={container} onChange={(event) => setContainer(event.target.value)}>
        <option value="">Main container</option><option value="*">All containers</option>{containers.map((value) => <option key={value}>{value}</option>)}
      </Select>
      <Input type="search" className="min-w-32 flex-1" aria-label="Search Pod logs" placeholder="Search logs" value={search} onChange={(event) => setSearch(event.target.value)} />
      <Button variant="secondary" aria-pressed={paused} title="Pause the display; live session capture continues" onClick={() => { pausedRef.current = !paused; setPaused(!paused); if (paused) setLines(buffer.current) }}>{paused ? 'Resume' : 'Pause'}</Button>
      <Button variant="ghost" aria-pressed={followTail} title="Follow the newest log line" onClick={() => setFollowTail(!followTail)}>Follow</Button>
      <Button variant="ghost" aria-pressed={previous} onClick={() => setPrevious(!previous)}>Previous</Button>
      <Button variant="ghost" aria-pressed={wrap} onClick={() => setWrap(!wrap)}>Wrap</Button>
      <Select className="!w-auto" disabled={downloading} aria-label="Download log scope" title="Session: captured in this panel. Visible: filtered lines. All / Previous: logs retained by Kubernetes for the selected containers." value={downloadScope} onChange={(event) => setDownloadScope(event.target.value)}>
        <option value="session">Session logs</option><option value="visible">Visible logs</option><option value="all">All logs</option><option value="previous">Previous logs</option>
      </Select>
      <Button variant="secondary" disabled={!targets.length || Boolean(error) || (downloadScope === 'visible' && !visible.length) || (downloadScope === 'session' && !captureState.lines)} onClick={() => { if (downloading) { downloadController.current?.abort(); downloadController.current = null; setDownloading(false) } else void download() }}>{downloading ? 'Cancel download' : 'Download'}</Button>
      <span className={`text-content ${status === 'Live' ? 'text-kp-green' : 'text-kp-overlay-text'}`} role="status" title="Live connections reconnect automatically. Reconnection may leave gaps if Kubernetes has rotated logs.">{status}{paused ? ' · display paused' : ''} · {visible.length} lines</span>
    </div>
    {limited ? <p role="note" className="m-0 text-content text-kp-yellow">Showing the first 5 Pod/container streams. Downloads use these same targets.</p> : null}
    {captureState.truncated ? <p role="note" className="m-0 text-content text-kp-yellow">Session capture is partial (64 MiB or line limit). Live viewing continues.</p> : null}
    {downloadNote ? <p role="status" className="m-0 text-content text-kp-yellow">{downloadNote}</p> : null}
    {detailError || error ? <p role="alert" className="m-0 text-content text-kp-red">{error || detailError}</p> : null}
    <pre ref={output} tabIndex={0} onScroll={(event) => { const element = event.currentTarget; setFollowTail(element.scrollHeight - element.scrollTop - element.clientHeight < 24) }} className={`log-output mono m-0 overflow-auto rounded-md border border-kp-overlay-0 bg-kp-crust p-3 text-content ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre'}`} aria-label={aggregate ? 'Aggregated log output' : 'Log output'}>{visible.map((line, index) => <span key={index}><time className="text-kp-overlay-text">{line.timestamp ?? ''} </time>{aggregate ? <span className="text-kp-mauve">{line.pod} </span> : null}<span className="text-kp-sky">{line.container} </span>{line.text}{line.truncated ? ' [truncated]' : ''}{'\n'}</span>)}</pre>
  </section>
}
