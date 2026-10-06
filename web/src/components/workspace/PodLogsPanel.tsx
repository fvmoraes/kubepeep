import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { APIError, getPod, getPodLogs, getSession } from '../../api/client'
import type { SelectionSummary } from '../../api/types'
import { appendLogBatch } from '../resource/boundedLogLines'
import { consumeAggregateStream, type AggregatedLogLine } from '../resource/podLogStream'
import { Button, Input, Select } from '../ui'

export interface PodLogTarget { namespace: string; name: string }

/** Reads only explicit table targets; never scans the Pod/workload catalogs. */
export function PodLogsPanel({ pods, selection }: { pods: PodLogTarget[]; selection: SelectionSummary }) {
  const [container, setContainer] = useState('')
  const [search, setSearch] = useState('')
  const [paused, setPaused] = useState(false)
  const [previous, setPrevious] = useState(false)
  const [wrap, setWrap] = useState(false)
  const [lines, setLines] = useState<AggregatedLogLine[]>([])
  const [status, setStatus] = useState('Opening logs…')
  const [error, setError] = useState('')
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
  const generation = selection.generation
  useEffect(() => {
    if (paused || targetKey === '[]') return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    let flushTimer: ReturnType<typeof setTimeout> | undefined
    let pending: AggregatedLogLine[] = []
    const activeTargets = JSON.parse(targetKey) as Array<{ namespace: string; pod: string; container: string }>
    const flush = () => {
      flushTimer = undefined
      if (controller.signal.aborted || pending.length === 0) return
      const batch = pending; pending = []
      setLines((current) => appendLogBatch(current, batch))
    }
    const fail = (cause: unknown) => {
      if (controller.signal.aborted) return
      setError(cause instanceof Error ? cause.message : 'Logs unavailable.')
      setStatus('Logs unavailable')
      controller.abort()
    }
    const snapshot = async () => {
      try {
        const responses = await Promise.all(activeTargets.map(async (target) => {
          const result = await getPodLogs(target.namespace, target.pod, { container: target.container, previous, timestamps: true, tailLines: 200 }, controller.signal, generation)
          return result.lines.map((line) => ({ ...line, pod: `${target.namespace}/${target.pod}`, container: target.container }))
        }))
        if (controller.signal.aborted) return
        setLines(appendLogBatch([], responses.flat().sort((a, b) => (a.timestamp ?? '').localeCompare(b.timestamp ?? ''))))
        setStatus(previous ? 'Previous container' : 'Auto · 5s')
        if (!previous) timer = setTimeout(() => void snapshot(), 5_000)
      } catch (cause) { fail(cause) }
    }
    const streamController = new AbortController()
    const abortStreams = () => streamController.abort()
    controller.signal.addEventListener('abort', abortStreams, { once: true })
    const follow = async () => {
      setStatus('Connecting…')
      try {
        const session = await getSession(controller.signal)
        if (session.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
        controller.signal.throwIfAborted()
        setStatus('Live')
        await Promise.all(activeTargets.map((target) => consumeAggregateStream(target, { generation }, session, { timestamps: true, tailLines: 200, since: '' }, streamController.signal, (line) => {
          if (controller.signal.aborted) return
          pending = appendLogBatch(pending, [{ ...line, pod: `${target.namespace}/${target.pod}` }])
          if (!flushTimer) flushTimer = setTimeout(flush, 75)
        })))
        flush()
        if (!controller.signal.aborted) await snapshot()
      } catch (cause) {
        streamController.abort()
        if (controller.signal.aborted) return
        if (cause instanceof APIError && (cause.status === 403 || cause.status === 401 || cause.code === 'GENERATION_CHANGED' || cause.code === 'FORBIDDEN' || cause.code === 'AUTHORIZATION_UNAVAILABLE')) fail(cause)
        else { flush(); await snapshot() }
      }
    }
    if (previous) void snapshot()
    else void follow()
    return () => { controller.abort(); controller.signal.removeEventListener('abort', abortStreams); clearTimeout(timer); clearTimeout(flushTimer); pending = [] }
  }, [targetKey, generation, paused, previous])
  const containers = [...new Set((details.data ?? []).flatMap((pod) => [...pod.containers, ...pod.initContainers, ...pod.ephemeralContainers].map((item) => item.spec.name)))]
  const visible = lines.filter((line) => line.text.toLocaleLowerCase().includes(search.toLocaleLowerCase()))
  return <section className="grid min-w-0 gap-2" aria-label={pods.length === 1 ? `Logs for ${pods[0].name}` : 'Selected Pod logs'}>
    <div className="flex flex-wrap items-center gap-2">
      <Select className="!w-auto max-w-full" aria-label="Log container" value={container} onChange={(event) => { setLines([]); setError(''); setContainer(event.target.value) }}>
        <option value="">Main container</option><option value="*">All containers</option>{containers.map((value) => <option key={value}>{value}</option>)}
      </Select>
      <Input type="search" className="min-w-32 flex-1" aria-label="Search Pod logs" placeholder="Search logs" value={search} onChange={(event) => setSearch(event.target.value)} />
      <Button variant="secondary" onClick={() => { if (paused) { setLines([]); setError('') }; setPaused(!paused) }}>{paused ? 'Resume' : 'Pause'}</Button>
      <Button variant="ghost" aria-pressed={previous} onClick={() => { setLines([]); setError(''); setPrevious(!previous) }}>Previous</Button>
      <Button variant="ghost" aria-pressed={wrap} onClick={() => setWrap(!wrap)}>Wrap</Button>
      <span className="text-content text-kp-overlay-text" role="status">{paused ? 'Paused' : status} · {visible.length} lines</span>
    </div>
    {pods.length > 5 || targets.length > 5 ? <p role="note" className="m-0 text-content text-kp-yellow">Showing the first 5 Pod/container streams. Narrow the selection to view other streams.</p> : null}
    {details.isError || error ? <p role="alert" className="m-0 text-content text-kp-red">{error || (details.error as Error)?.message}</p> : null}
    <pre className={`log-output mono m-0 overflow-auto rounded-md border border-kp-overlay-0 bg-kp-crust p-3 text-content ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre'}`} aria-label={pods.length === 1 ? 'Log output' : 'Aggregated log output'}>{visible.map((line, index) => <span key={index}><time className="text-kp-overlay-text">{line.timestamp ?? ''} </time>{pods.length > 1 ? <span className="text-kp-mauve">{line.pod} </span> : null}<span className="text-kp-sky">{line.container} </span>{line.text}{line.truncated ? ' [truncated]' : ''}{'\n'}</span>)}</pre>
  </section>
}
