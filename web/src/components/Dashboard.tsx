import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  AlertTriangle,
  Boxes,
  Clock3,
  RefreshCw,
  RotateCcw,
  ScrollText,
  ShieldCheck,
} from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router'

import {
  APIError,
  getDashboardEvents,
  getDashboardMetrics,
  getDashboardNamespaceHealth,
  getDashboardProblems,
  getDashboardRestarts,
  getDashboardSummary,
  getSession,
  getStatus,
  scanDashboardLogs,
  type ComponentState,
  type DashboardBlock,
  type DashboardCounter,
  type DashboardEvent,
  type DashboardLogMatch,
  type DashboardMetrics,
  type DashboardNamespaceHealth,
  type DashboardProblem,
  type DashboardResponse,
  type DashboardRestart,
  type DashboardSummary,
  type LogScanRequest,
  type MetricRank,
  type SelectionSummary,
} from '../api/client'
import { StatePanel } from './StatePanel'
import { Badge, Button, DataTable, Select, type BadgeVariant } from './ui'
import { WarningBanner } from './ui/Banner'

const dashboardQueryDefaults = {
  staleTime: 30_000,
  refetchOnWindowFocus: false,
  retry: false,
} as const

const defaultLogScan: Omit<LogScanRequest, 'window'> = {
  tailLines: 200,
  maxPods: 20,
  maxConcurrentContainers: 4,
}

type LogScanState =
  | { kind: 'idle' }
  | { kind: 'pending' }
  | { kind: 'success'; response: DashboardResponse<DashboardLogMatch[]> }
  | { kind: 'error'; error: Error }

interface ResultBodyProps<T> {
  pending: boolean
  error: Error | null
  response?: DashboardResponse<T>
  isEmpty: (value: T) => boolean
  emptyCopy: string
  optional?: boolean
  children: (value: T) => ReactNode
}

const counterCopy: Record<DashboardCounter['state'], string> = {
  available: 'complete',
  denied: 'access denied',
  unavailable: 'unavailable',
  notCollected: 'not collected',
  collecting: 'collecting',
  truncated: 'bounded result',
}

function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return 'unknown'
  }
  if (seconds < 60) {
    return `${Math.floor(seconds)}s`
  }
  if (seconds < 3_600) {
    return `${Math.floor(seconds / 60)}m`
  }
  if (seconds < 86_400) {
    return `${Math.floor(seconds / 3_600)}h`
  }
  return `${Math.floor(seconds / 86_400)}d`
}

function formatTimestamp(value: string | null | undefined): string {
  if (!value) {
    return 'not reported'
  }
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? 'not reported' : parsed.toLocaleString()
}

function formatMemory(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return '—'
  }
  if (bytes < 1_048_576) {
    return `${Math.round(bytes / 1_024)} KiB`
  }
  if (bytes < 1_073_741_824) {
    return `${(bytes / 1_048_576).toFixed(1)} MiB`
  }
  return `${(bytes / 1_073_741_824).toFixed(2)} GiB`
}

function onlyDenied(block: DashboardBlock<unknown>): boolean {
  return block.errors.length > 0 && block.errors.every((error) => error.code === 'FORBIDDEN')
}

function featureUnavailable(block: DashboardBlock<unknown>): boolean {
  return block.errors.some((error) => error.code === 'FEATURE_UNAVAILABLE')
}

function blockStateBox(kind: 'loading' | 'offline' | 'unavailable' | 'denied' | 'empty' | 'idle'): string {
  const border = {
    loading: 'border-kp-blue-border',
    offline: 'border-kp-red-border',
    unavailable: 'border-kp-yellow-border',
    denied: 'border-kp-red-border',
    empty: 'border-kp-overlay-0',
    idle: 'border-kp-overlay-0',
  }[kind]
  return `flex flex-col justify-center gap-1 rounded-lg border border-dashed ${border} bg-kp-surface-1 px-4 py-3.5`
}

function queryFailure(error: Error, optional: boolean): ReactNode {
  if (error instanceof APIError && error.code === 'FORBIDDEN') {
    return <div className={blockStateBox('denied')} role="status"><strong className="text-sm text-kp-text">Access denied</strong><span className="text-xs text-kp-overlay-text">This block was not collected. No zero value is implied.</span></div>
  }
  if (optional && error instanceof APIError && error.code === 'FEATURE_UNAVAILABLE') {
    return <div className="rounded-r-md border-l-2 border-kp-blue-border bg-kp-blue-bg px-3 py-2 text-sm text-kp-sky" role="status">Metrics API is not available for this cluster.</div>
  }
  const offline = !(error instanceof APIError) || ['CLUSTER_UNAVAILABLE', 'UPSTREAM_TIMEOUT', 'AUTHENTICATION_UNAVAILABLE'].includes(error.code)
  return (
    <div className={blockStateBox(offline ? 'offline' : 'unavailable')} role="status">
      <strong className="text-sm text-kp-text">{offline ? 'Cluster data is offline' : 'This block is unavailable'}</strong>
      <span className="text-xs text-kp-overlay-text">{error instanceof APIError ? error.message : 'The local API could not complete this query.'}</span>
    </div>
  )
}

function PartialFeedback({ block }: { block: DashboardBlock<unknown> }) {
  const coverage = block.coverage
  return (
    <>
      {block.truncated || !block.complete ? (
        <WarningBanner className="mb-2.5">
          {block.truncated ? 'This is a bounded result. Totals and rankings may be incomplete.' : 'This block is partial; other dashboard blocks remain usable.'}
        </WarningBanner>
      ) : null}
      {coverage && coverage.requestedNamespaces > 0 && (!block.complete || block.truncated) ? (
        <p className="mb-2.5 text-xs text-kp-overlay-text">
          Coverage: {coverage.completedNamespaces} of {coverage.requestedNamespaces} namespaces.
          {coverage.deniedNamespaces.length > 0 ? ` ${coverage.deniedNamespaces.length} denied.` : ''}
        </p>
      ) : null}
      {block.errors.length > 0 ? (
        <ul className="mb-3 grid list-none gap-1 p-0 text-xs text-kp-subtext" aria-label="Partial collection errors">
          {block.errors.map((error, index) => (
            <li key={`${error.namespace ?? 'global'}-${error.code}-${index}`} className="rounded-md border border-kp-red-border bg-kp-red-bg px-2 py-1">
              <code>{error.code}</code>{error.namespace ? ` · ${error.namespace}` : ''}: {error.message}
            </li>
          ))}
        </ul>
      ) : null}
    </>
  )
}

function ResultBody<T>({ pending, error, response, isEmpty, emptyCopy, optional = false, children }: ResultBodyProps<T>) {
  if (pending) {
    return <div className={blockStateBox('loading')} role="status" aria-busy="true"><strong className="text-sm text-kp-text">Loading this block</strong><span className="text-xs text-kp-overlay-text">Other dashboard queries continue independently.</span></div>
  }
  if (error) {
    return queryFailure(error, optional)
  }
  if (!response) {
    return <div className={blockStateBox('unavailable')} role="status"><strong className="text-sm text-kp-text">No response</strong><span className="text-xs text-kp-overlay-text">This block has not returned data.</span></div>
  }
  const block = response.block
  if (optional && featureUnavailable(block as DashboardBlock<unknown>) && isEmpty(block.value)) {
    return <div className="rounded-r-md border-l-2 border-kp-blue-border bg-kp-blue-bg px-3 py-2 text-sm text-kp-sky" role="status">Metrics API is not available. The rest of the dashboard is unaffected.</div>
  }
  if (onlyDenied(block as DashboardBlock<unknown>) && isEmpty(block.value)) {
    return <div className={blockStateBox('denied')} role="status"><strong className="text-sm text-kp-text">Access denied</strong><span className="text-xs text-kp-overlay-text">This block was not collected. No zero value is implied.</span></div>
  }
  if (isEmpty(block.value) && !block.complete) {
    return (
      <>
        <PartialFeedback block={block as DashboardBlock<unknown>} />
        <div className={blockStateBox('unavailable')} role="status"><strong className="text-sm text-kp-text">No complete result</strong><span className="text-xs text-kp-overlay-text">The query ended before an authoritative empty result was available.</span></div>
      </>
    )
  }
  return (
    <>
      <PartialFeedback block={block as DashboardBlock<unknown>} />
      {isEmpty(block.value) ? <div className={blockStateBox('empty')} role="status"><strong className="text-sm text-kp-text">Nothing found</strong><span className="text-xs text-kp-overlay-text">{emptyCopy}</span></div> : children(block.value)}
    </>
  )
}

function NamespaceHealthTable({ values }: { values: DashboardNamespaceHealth[] }) {
  const columns = [
    {
      key: 'namespace',
      header: 'Namespace',
      cell: (row: DashboardNamespaceHealth) => row.namespace,
    },
    {
      key: 'problematic-pods',
      header: 'Problem pods',
      cell: (row: DashboardNamespaceHealth) => (
        <Link className="text-kp-mauve hover:underline" to={`/pods?namespace=${encodeURIComponent(row.namespace)}&problematic=true`}>{row.problematicPods}</Link>
      ),
    },
    {
      key: 'container-restarts',
      header: 'Restarts',
      cell: (row: DashboardNamespaceHealth) => (
        <Link className="text-kp-mauve hover:underline" to={`/pods?namespace=${encodeURIComponent(row.namespace)}&restarts=gte3`}>{row.containerRestarts}</Link>
      ),
    },
    {
      key: 'degraded-workloads',
      header: 'Degraded workloads',
      cell: (row: DashboardNamespaceHealth) => (
        <Link className="text-kp-mauve hover:underline" to={`/workloads?namespace=${encodeURIComponent(row.namespace)}&status=Degraded`}>{row.degradedWorkloads}</Link>
      ),
    },
  ]
  return (
    <DataTable
      caption="Per-namespace health; counts link to the filtered bounded list"
      columns={columns}
      rows={values}
      getRowKey={(row) => row.namespace}
    />
  )
}

function DashboardSection({ id, title, action, children }: { id: string; title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section id={id} aria-labelledby={`${id}-title`} className="rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-4">
      <div className="mb-3 flex items-center justify-between gap-4">
        <h2 id={`${id}-title`} className="text-base text-kp-text">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}

function counterBadgeVariant(state: DashboardCounter['state']): BadgeVariant {
  switch (state) {
    case 'available':
      return 'healthy'
    case 'truncated':
      return 'warning'
    case 'collecting':
      return 'info'
    case 'denied':
    case 'unavailable':
      return 'danger'
    case 'notCollected':
    default:
      return 'unknown'
  }
}

function CounterCard({ label, counter, href, icon }: { label: string; counter: DashboardCounter; href: string; icon: ReactNode }) {
  const card = (
    <>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-kp-overlay-text">{label}</span>
        <span className="text-kp-overlay-text" aria-hidden="true">{icon}</span>
      </div>
      <strong className="text-3xl text-kp-text">{counter.value === null ? '—' : counter.value.toLocaleString()}</strong>
      {counter.state !== 'available' ? <Badge variant={counterBadgeVariant(counter.state)} className="justify-self-start">{counterCopy[counter.state]}</Badge> : null}
    </>
  )
  if (counter.state === 'available' || counter.state === 'truncated') {
    return <Link className="grid content-between gap-1.5 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-3.5 hover:border-kp-overlay-2" to={href} aria-label={`${label}: ${counter.value ?? 0}, ${counterCopy[counter.state]}`}>{card}</Link>
  }
  return <div className="grid content-between gap-1.5 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-3.5 opacity-75" aria-label={`${label}: ${counterCopy[counter.state]}`}>{card}</div>
}

function SummaryCards({ summary, logCounter }: { summary: DashboardSummary; logCounter: DashboardCounter }) {
  const cards = [
    ['Namespaces', summary.namespaces, '/namespaces', <Boxes size={15} key="namespaces" />],
    ['Pods', summary.podsTotal, '/pods', <Activity size={15} key="pods" />],
    ['Healthy pods', summary.podsHealthy, '/pods?status=Running', <ShieldCheck size={15} key="healthy" />],
    ['Problem pods', summary.podsProblematic, '/pods?problematic=true', <AlertTriangle size={15} key="problems" />],
    ['Degraded workloads', summary.workloadsDegraded, '/workloads?status=Degraded', <Boxes size={15} key="workloads" />],
    ['Restarts', summary.restarts, '/pods?restarts=gte3', <RotateCcw size={15} key="restarts" />],
    ['Warning events', summary.warningEvents, '/events?status=Warning', <Clock3 size={15} key="events" />],
    ['Possible log matches', logCounter, '#log-scan', <ScrollText size={15} key="logs" />],
  ] as const
  return <div className="grid grid-cols-2 gap-2.5 md:grid-cols-4">{cards.map(([label, counter, href, icon]) => <CounterCard key={label} label={label} counter={counter} href={href} icon={icon} />)}</div>
}

function severityBadgeVariant(severity: DashboardProblem['severity']): BadgeVariant {
  switch (severity) {
    case 'critical':
      return 'danger'
    case 'warning':
    default:
      return 'warning'
  }
}

function ProblemsTable({ values }: { values: DashboardProblem[] }) {
  const columns = [
    {
      key: 'severity',
      header: 'Severity',
      cell: (problem: DashboardProblem) => <Badge variant={severityBadgeVariant(problem.severity)}>{problem.severity}</Badge>,
    },
    {
      key: 'pod',
      header: 'Pod',
      cell: (problem: DashboardProblem) => <><strong className="block text-kp-text">{problem.pod}</strong><small className="block text-xs text-kp-overlay-text">{problem.namespace}{problem.container ? ` · ${problem.container}` : ''}</small></>,
    },
    {
      key: 'diagnosis',
      header: 'Diagnosis',
      cell: (problem: DashboardProblem) => <><strong className="block text-kp-text">{problem.reason ?? 'No diagnosis reported'}</strong><small className="block text-xs text-kp-overlay-text">{problem.message ?? `Source: ${problem.source}`}</small></>,
    },
    {
      key: 'status',
      header: 'Status',
      cell: (problem: DashboardProblem) => problem.status,
    },
    {
      key: 'age',
      header: 'Age',
      cell: (problem: DashboardProblem) => formatDuration(problem.ageSeconds),
    },
  ]
  return (
    <DataTable
      caption="At most one prioritized diagnosis per pod"
      columns={columns}
      rows={values}
      getRowKey={(problem) => `${problem.namespace}/${problem.pod}`}
    />
  )
}

function restartSeverityBadgeVariant(severity: DashboardRestart['severity']): BadgeVariant {
  switch (severity) {
    case 'critical':
      return 'danger'
    case 'warning':
      return 'warning'
    case 'attention':
      return 'info'
    case 'healthy':
    default:
      return 'healthy'
  }
}

function RestartsTable({ values }: { values: DashboardRestart[] }) {
  const columns = [
    {
      key: 'restarts',
      header: 'Restarts',
      cell: (restart: DashboardRestart) => <Badge variant={restartSeverityBadgeVariant(restart.severity)}>{restart.restarts}</Badge>,
    },
    {
      key: 'pod',
      header: 'Pod / owner',
      cell: (restart: DashboardRestart) => <><strong className="block text-kp-text">{restart.pod}</strong><small className="block text-xs text-kp-overlay-text">{restart.namespace}{restart.owner ? ` · ${restart.owner.kind}/${restart.owner.name}` : ' · owner unavailable'}</small></>,
    },
    {
      key: 'container',
      header: 'Container',
      cell: (restart: DashboardRestart) => <>{restart.container}<small className="block text-xs text-kp-overlay-text">{restart.containerType}</small></>,
    },
    {
      key: 'status',
      header: 'Status',
      cell: (restart: DashboardRestart) => restart.status || restart.lastReason || 'not reported',
    },
    {
      key: 'age',
      header: 'Age',
      cell: (restart: DashboardRestart) => formatDuration(restart.ageSeconds),
    },
  ]
  return (
    <DataTable
      caption="Container restart ranking, highest count first"
      columns={columns}
      rows={values}
      getRowKey={(restart) => `${restart.namespace}/${restart.pod}/${restart.containerType}/${restart.container}`}
    />
  )
}

function EventsTable({ values }: { values: DashboardEvent[] }) {
  const columns = [
    {
      key: 'last-seen',
      header: 'Last seen',
      cell: (event: DashboardEvent) => formatTimestamp(event.timestamp),
    },
    {
      key: 'object',
      header: 'Object',
      cell: (event: DashboardEvent) => <><strong className="block text-kp-text">{event.objectKind}/{event.objectName}</strong><small className="block text-xs text-kp-overlay-text">{event.namespace}</small></>,
    },
    {
      key: 'reason',
      header: 'Reason',
      cell: (event: DashboardEvent) => <><strong className="block text-kp-text">{event.reason}</strong><small className="block text-xs text-kp-overlay-text">{event.message}</small></>,
    },
    {
      key: 'count',
      header: 'Count',
      cell: (event: DashboardEvent) => event.count,
    },
    {
      key: 'source',
      header: 'Source',
      cell: (event: DashboardEvent) => event.source ?? 'not reported',
    },
  ]
  return (
    <DataTable
      caption="Grouped Kubernetes Warning events"
      columns={columns}
      rows={values}
      getRowKey={(event, index) => `${event.namespace}/${event.objectKind}/${event.objectName}/${event.reason}/${index}`}
    />
  )
}

function MetricsTable({ title, values, metric }: { title: string; values: MetricRank[]; metric: 'cpu' | 'memory' }) {
  const columns = [
    {
      key: 'pod',
      header: 'Pod',
      cell: (value: MetricRank) => <><strong className="block text-kp-text">{value.pod}</strong><small className="block text-xs text-kp-overlay-text">{value.namespace}</small></>,
    },
    {
      key: 'cpu',
      header: 'CPU',
      cell: (value: MetricRank) => `${value.cpuMillicores.toLocaleString()}m`,
    },
    {
      key: 'memory',
      header: 'Memory',
      cell: (value: MetricRank) => formatMemory(value.memoryBytes),
    },
  ]
  return (
    <div className="min-w-0 rounded-lg border border-kp-overlay-0 bg-kp-surface-1 p-3">
      <h3 className="mb-2.5 text-sm text-kp-text">{title}</h3>
      <DataTable
        compact
        columns={columns}
        rows={values}
        getRowKey={(value) => `${metric}/${value.namespace}/${value.pod}`}
      />
    </div>
  )
}

function MetricsView({ value }: { value: DashboardMetrics }) {
  return (
    <>
      <p className="mb-2.5 text-xs text-kp-overlay-text">Metrics window: {formatDuration(value.windowSeconds)} · collected {formatTimestamp(value.collectedAt)}</p>
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <MetricsTable title="Top CPU" values={value.topCPU} metric="cpu" />
        <MetricsTable title="Top memory" values={value.topMemory} metric="memory" />
      </div>
    </>
  )
}

function LogMatchesTable({ values }: { values: DashboardLogMatch[] }) {
  const columns = [
    {
      key: 'detected',
      header: 'Detected',
      cell: (match: DashboardLogMatch) => formatTimestamp(match.timestamp),
    },
    {
      key: 'target',
      header: 'Target',
      cell: (match: DashboardLogMatch) => <><strong className="block text-kp-text">{match.pod}/{match.container}</strong><small className="block text-xs text-kp-overlay-text">{match.namespace}{match.workload ? ` · ${match.workload.kind}/${match.workload.name}` : ''}</small></>,
    },
    {
      key: 'reason',
      header: 'Reason',
      cell: (match: DashboardLogMatch) => <><code>{match.reasonCode}</code><small className="block text-xs text-kp-overlay-text">{match.redacted ? 'sensitive value redacted' : 'no redaction needed'}{match.truncated ? ' · excerpt truncated' : ''}</small></>,
    },
    {
      key: 'excerpt',
      header: 'Sanitized excerpt',
      cell: (match: DashboardLogMatch) => <code className="block max-w-[540px] text-kp-subtext break-all whitespace-pre-wrap">{match.excerpt}</code>,
    },
  ]
  return (
    <DataTable
      caption="Possible matches only; excerpts are bounded and sanitized by the backend"
      columns={columns}
      rows={values}
      getRowKey={(match, index) => `${match.namespace}/${match.pod}/${match.container}/${match.timestamp ?? index}`}
    />
  )
}

function latestCollection(responses: Array<DashboardResponse<unknown> | undefined>): string {
  const timestamps = responses
    .map((response) => response?.meta?.collectedAt)
    .filter((value): value is string => Boolean(value))
    .map((value) => new Date(value))
    .filter((value) => !Number.isNaN(value.getTime()))
    .sort((left, right) => right.getTime() - left.getTime())
  return timestamps[0] ? timestamps[0].toLocaleString() : 'waiting for the first completed block'
}

const staleThresholdSeconds = 60

function BlockAge({ response }: { response?: DashboardResponse<unknown> }) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 5_000)
    return () => window.clearInterval(timer)
  }, [])
  const collectedAt = response?.meta?.collectedAt
  if (!collectedAt) {
    return null
  }
  const collected = new Date(collectedAt).getTime()
  if (Number.isNaN(collected)) {
    return null
  }
  const ageSeconds = Math.max(0, Math.floor((now - collected) / 1_000))
  const stale = ageSeconds > staleThresholdSeconds
  return (
    <Badge variant={stale ? 'warning' : 'unknown'} aria-label={`Data age ${formatDuration(ageSeconds)}${stale ? ', stale' : ''}`}>
      {formatDuration(ageSeconds)} old{stale ? ' · stale' : ''}
    </Badge>
  )
}

function DashboardContent({ selection, cluster }: { selection: SelectionSummary; cluster: ComponentState }) {
	const summary = useQuery({ queryKey: ['dashboard', 'summary', selection.generation], queryFn: ({ signal }) => getDashboardSummary(signal, selection.generation), ...dashboardQueryDefaults })
	const problems = useQuery({ queryKey: ['dashboard', 'problems', selection.generation], queryFn: ({ signal }) => getDashboardProblems(signal, selection.generation), ...dashboardQueryDefaults })
	const restarts = useQuery({ queryKey: ['dashboard', 'restarts', selection.generation, 10], queryFn: ({ signal }) => getDashboardRestarts(10, signal, selection.generation), ...dashboardQueryDefaults })
	const events = useQuery({ queryKey: ['dashboard', 'events', selection.generation], queryFn: ({ signal }) => getDashboardEvents(signal, selection.generation), ...dashboardQueryDefaults })
	const metrics = useQuery({ queryKey: ['dashboard', 'metrics', selection.generation], queryFn: ({ signal }) => getDashboardMetrics(signal, selection.generation), ...dashboardQueryDefaults })
	const namespaceHealth = useQuery({ queryKey: ['dashboard', 'namespace-health', selection.generation], queryFn: ({ signal }) => getDashboardNamespaceHealth(signal, selection.generation), ...dashboardQueryDefaults })
  const session = useQuery({ queryKey: ['session', selection.generation], queryFn: ({ signal }) => getSession(signal), staleTime: 5 * 60_000, retry: false })
  const [scanWindow, setScanWindow] = useState<LogScanRequest['window']>('15m')
  const [logScan, setLogScan] = useState<LogScanState>({ kind: 'idle' })
  const logController = useRef<AbortController | null>(null)
  const logIntent = useRef(0)

  useEffect(() => () => logController.current?.abort(), [])

  const runLogScan = async () => {
    if (!session.data) {
      return
    }
    logController.current?.abort()
    const controller = new AbortController()
    const intent = ++logIntent.current
    logController.current = controller
    setLogScan({ kind: 'pending' })
    try {
		const response = await scanDashboardLogs({ window: scanWindow, ...defaultLogScan }, session.data.csrfToken, controller.signal, selection.generation)
      if (intent === logIntent.current) {
        setLogScan({ kind: 'success', response })
      }
    } catch (error) {
      if (intent !== logIntent.current || (error instanceof DOMException && error.name === 'AbortError')) {
        return
      }
      setLogScan({ kind: 'error', error: error as Error })
    }
  }

  const refreshAll = () => {
    void Promise.allSettled([
      summary.refetch({ cancelRefetch: true }),
      problems.refetch({ cancelRefetch: true }),
      restarts.refetch({ cancelRefetch: true }),
      events.refetch({ cancelRefetch: true }),
      metrics.refetch({ cancelRefetch: true }),
      namespaceHealth.refetch({ cancelRefetch: true }),
    ])
  }

  const isRefreshing = [summary, problems, restarts, events, metrics, namespaceHealth].some((query) => query.isFetching)
  const logCounter: DashboardCounter = logScan.kind === 'pending'
    ? { state: 'collecting', value: null }
    : logScan.kind === 'success'
      ? onlyDenied(logScan.response.block as DashboardBlock<unknown>)
        ? { state: 'denied', value: null }
        : logScan.response.block.complete
          ? { state: 'available', value: logScan.response.block.value.length }
          : logScan.response.block.value.length > 0 || logScan.response.block.truncated
            ? { state: 'truncated', value: logScan.response.block.value.length }
            : { state: 'unavailable', value: null }
      : summary.data?.block.value.possibleLogMatches ?? { state: 'notCollected', value: null }

  const collectedAt = latestCollection([
    summary.data as DashboardResponse<unknown> | undefined,
    problems.data as DashboardResponse<unknown> | undefined,
    restarts.data as DashboardResponse<unknown> | undefined,
    events.data as DashboardResponse<unknown> | undefined,
    metrics.data as DashboardResponse<unknown> | undefined,
  ])

  return (
    <div className="grid gap-4">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl text-kp-text">Cluster overview</h1>
          <p className="mt-0.5 text-sm text-kp-overlay-text">A quick view of your Kubernetes cluster, workloads and relevant events.</p>
        </div>
        <Button variant="secondary" onClick={refreshAll} disabled={isRefreshing}>
          <RefreshCw size={14} aria-hidden="true" className={isRefreshing ? 'animate-spin-slow' : ''} /> {isRefreshing ? 'Refreshing…' : 'Refresh'}
        </Button>
      </header>

      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-kp-overlay-0 bg-kp-overlay-0 md:grid-cols-5" aria-label="Dashboard selection">
        <div className="min-w-0 bg-kp-surface-1 px-3 py-2.5"><span className="block text-2xs uppercase tracking-wider text-kp-overlay-text">Context</span><strong className="block truncate text-sm text-kp-text">{selection.context}</strong><small className="block truncate text-xs text-kp-overlay-text">{selection.cluster}</small></div>
        <div className="min-w-0 bg-kp-surface-1 px-3 py-2.5"><span className="block text-2xs uppercase tracking-wider text-kp-overlay-text">Scope</span><strong className="block truncate text-sm text-kp-text">{selection.scopeName ?? 'No saved scope'}</strong><small className="block truncate text-xs text-kp-overlay-text">{selection.namespaceCount} namespace{selection.namespaceCount === 1 ? '' : 's'}{selection.defaultNamespace ? ` · default ${selection.defaultNamespace}` : ''}</small></div>
        <div className="min-w-0 bg-kp-surface-1 px-3 py-2.5"><span className="block text-2xs uppercase tracking-wider text-kp-overlay-text">Connection</span><strong className={`block text-sm ${cluster.status === 'healthy' ? 'text-kp-green' : cluster.status === 'unhealthy' ? 'text-kp-red' : 'text-kp-yellow'}`}>{cluster.status}</strong><small className="block truncate text-xs text-kp-overlay-text">{cluster.message}</small></div>
        <div className="min-w-0 bg-kp-surface-1 px-3 py-2.5"><span className="block text-2xs uppercase tracking-wider text-kp-overlay-text">Last update</span><strong className="block truncate text-sm text-kp-text">{collectedAt}</strong><small className="block truncate text-xs text-kp-overlay-text">generation {selection.generation}</small></div>
        <div className="flex items-center justify-center gap-3 bg-kp-surface-1 px-3 py-2.5 text-xs"><Link className="text-kp-mauve hover:underline" to="/namespaces">Edit scope</Link><Link className="text-kp-mauve hover:underline" to="/permissions">View RBAC</Link></div>
      </div>

      <DashboardSection id="summary" title="Summary" action={<BlockAge response={summary.data as DashboardResponse<unknown> | undefined} />}>
        <ResultBody
          pending={summary.isPending}
          error={summary.error}
          response={summary.data}
          isEmpty={() => false}
          emptyCopy="No summary is available."
        >
          {(value) => <SummaryCards summary={value} logCounter={logCounter} />}
        </ResultBody>
      </DashboardSection>

      <div className="grid items-start gap-4 lg:grid-cols-2">
        <DashboardSection id="problems" title="Problem pods" action={<BlockAge response={problems.data as DashboardResponse<unknown> | undefined} />}>
          <ResultBody pending={problems.isPending} error={problems.error} response={problems.data} isEmpty={(value) => value.length === 0} emptyCopy="No problematic pod was found in the completed coverage.">
            {(value) => <ProblemsTable values={value} />}
          </ResultBody>
        </DashboardSection>

        <DashboardSection id="restarts" title="Container restarts" action={<BlockAge response={restarts.data as DashboardResponse<unknown> | undefined} />}>
          <ResultBody pending={restarts.isPending} error={restarts.error} response={restarts.data} isEmpty={(value) => value.length === 0} emptyCopy="No container restart was found in the completed coverage.">
            {(value) => <RestartsTable values={value} />}
          </ResultBody>
        </DashboardSection>
      </div>

      <div className="grid items-start gap-4 lg:grid-cols-2">
        <DashboardSection id="warning-events" title="Warning events" action={<BlockAge response={events.data as DashboardResponse<unknown> | undefined} />}>
          <ResultBody pending={events.isPending} error={events.error} response={events.data} isEmpty={(value) => value.length === 0} emptyCopy="No Warning event was found in the completed coverage.">
            {(value) => <EventsTable values={value} />}
          </ResultBody>
        </DashboardSection>

        <DashboardSection id="namespace-health" title="Namespace health" action={<BlockAge response={namespaceHealth.data as DashboardResponse<unknown> | undefined} />}>
          <ResultBody pending={namespaceHealth.isPending} error={namespaceHealth.error} response={namespaceHealth.data} isEmpty={(value) => value.length === 0} emptyCopy="No namespace health was collected for the active scope.">
            {(value) => <NamespaceHealthTable values={value} />}
          </ResultBody>
        </DashboardSection>
      </div>

      <DashboardSection id="metrics" title="Pod metrics" action={<BlockAge response={metrics.data as DashboardResponse<unknown> | undefined} />}>
        <ResultBody pending={metrics.isPending} error={metrics.error} response={metrics.data} isEmpty={(value) => value.pods.length === 0} emptyCopy="The Metrics API returned no pod metrics for the completed coverage." optional>
          {(value) => <MetricsView value={value} />}
        </ResultBody>
      </DashboardSection>

      <DashboardSection
        id="log-scan"
        title="Possible errors in logs"
        action={(
          <div className="flex items-end justify-end gap-2">
            <label className="grid gap-1">
              <span className="sr-only">Log scan window</span>
              <Select aria-label="Log scan window" value={scanWindow} onChange={(event) => setScanWindow(event.target.value as LogScanRequest['window'])}>
                <option value="15m">15 min</option><option value="30m">30 min</option><option value="1h">1 hour</option><option value="4h">4 hours</option>
              </Select>
            </label>
            <Button onClick={() => void runLogScan()} disabled={!session.data || session.isError}>
              <ScrollText size={14} aria-hidden="true" /> {logScan.kind === 'pending' ? 'Restart scan' : 'Scan logs now'}
            </Button>
          </div>
        )}
      >
        <p className="mb-2.5 text-xs text-kp-overlay-text">Scans at most 20 pods, 200 lines each, with four concurrent container reads. Results are never saved by this interface.</p>
        {session.isError ? <div className={blockStateBox('unavailable')} role="status"><strong className="text-sm text-kp-text">Scan session unavailable</strong><span className="text-xs text-kp-overlay-text">The CSRF bootstrap could not be completed.</span></div> : null}
        {logScan.kind === 'idle' ? <div className={blockStateBox('idle')} role="status"><strong className="text-sm text-kp-text">Scan has not been run</strong><span className="text-xs text-kp-overlay-text">Run it explicitly when recent, bounded log inspection is useful.</span></div> : null}
        {logScan.kind === 'pending' ? <div className={blockStateBox('loading')} role="status" aria-busy="true"><strong className="text-sm text-kp-text">Scanning selected targets</strong><span className="text-xs text-kp-overlay-text">Starting another scan, switching generation, or leaving this page cancels it.</span></div> : null}
        {logScan.kind === 'error' ? queryFailure(logScan.error, false) : null}
        {logScan.kind === 'success' ? (
          <ResultBody pending={false} error={null} response={logScan.response} isEmpty={(value) => value.length === 0} emptyCopy="The completed scan found no possible matches.">
            {(value) => <LogMatchesTable values={value} />}
          </ResultBody>
        ) : null}
      </DashboardSection>
    </div>
  )
}

export function DashboardPage() {
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000, refetchOnWindowFocus: false, retry: false })

  if (status.isPending) {
    return <StatePanel kind="loading" title="Preparing the local workspace">Checking application, selection, and cluster health.</StatePanel>
  }
  if (status.isError) {
    if (status.error instanceof APIError) {
      return <StatePanel kind="error" title="The local API returned an error">The response was rejected safely. Retry after checking the local status or running KubePeep doctor.</StatePanel>
    }
    return <StatePanel kind="offline" title="The local API is unavailable">Reload after the KubePeep process is ready.</StatePanel>
  }
  if (!status.data.selection) {
    return <StatePanel kind="empty" title="Choose a Kubernetes context">The local application is ready. Use the context selector in the header to connect a kubeconfig.</StatePanel>
  }
  return <DashboardContent key={status.data.selection.generation} selection={status.data.selection} cluster={status.data.components.cluster} />
}
