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
  type DashboardProblem,
  type DashboardResponse,
  type DashboardRestart,
  type DashboardSummary,
  type LogScanRequest,
  type MetricRank,
  type SelectionSummary,
} from '../api/client'
import { StatePanel } from './StatePanel'

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

function queryFailure(error: Error, optional: boolean): ReactNode {
  if (error instanceof APIError && error.code === 'FORBIDDEN') {
    return <div className="dashboard-state dashboard-state--denied" role="status"><strong>Access denied</strong><span>This block was not collected. No zero value is implied.</span></div>
  }
  if (optional && error instanceof APIError && error.code === 'FEATURE_UNAVAILABLE') {
    return <div className="dashboard-optional" role="status">Metrics API is not available for this cluster.</div>
  }
  const offline = !(error instanceof APIError) || ['CLUSTER_UNAVAILABLE', 'UPSTREAM_TIMEOUT', 'AUTHENTICATION_UNAVAILABLE'].includes(error.code)
  return (
    <div className={`dashboard-state dashboard-state--${offline ? 'offline' : 'unavailable'}`} role="status">
      <strong>{offline ? 'Cluster data is offline' : 'This block is unavailable'}</strong>
      <span>{error instanceof APIError ? error.message : 'The local API could not complete this query.'}</span>
    </div>
  )
}

function PartialFeedback({ block }: { block: DashboardBlock<unknown> }) {
  const coverage = block.coverage
  return (
    <>
      {block.truncated ? <p className="dashboard-notice" role="status">This is a bounded result. Totals and rankings may be incomplete.</p> : null}
      {!block.complete && !block.truncated ? <p className="dashboard-notice" role="status">This block is partial; other dashboard blocks remain usable.</p> : null}
      {coverage && coverage.requestedNamespaces > 0 && (!block.complete || block.truncated) ? (
        <p className="dashboard-coverage">
          Coverage: {coverage.completedNamespaces} of {coverage.requestedNamespaces} namespaces.
          {coverage.deniedNamespaces.length > 0 ? ` ${coverage.deniedNamespaces.length} denied.` : ''}
        </p>
      ) : null}
      {block.errors.length > 0 ? (
        <ul className="dashboard-errors" aria-label="Partial collection errors">
          {block.errors.map((error, index) => (
            <li key={`${error.namespace ?? 'global'}-${error.code}-${index}`}>
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
    return <div className="dashboard-state dashboard-state--loading" role="status" aria-busy="true"><strong>Loading this block</strong><span>Other dashboard queries continue independently.</span></div>
  }
  if (error) {
    return queryFailure(error, optional)
  }
  if (!response) {
    return <div className="dashboard-state dashboard-state--unavailable" role="status"><strong>No response</strong><span>This block has not returned data.</span></div>
  }
  const block = response.block
  if (optional && featureUnavailable(block as DashboardBlock<unknown>) && isEmpty(block.value)) {
    return <div className="dashboard-optional" role="status">Metrics API is not available. The rest of the dashboard is unaffected.</div>
  }
  if (onlyDenied(block as DashboardBlock<unknown>) && isEmpty(block.value)) {
    return <div className="dashboard-state dashboard-state--denied" role="status"><strong>Access denied</strong><span>This block was not collected. No zero value is implied.</span></div>
  }
  if (isEmpty(block.value) && !block.complete) {
    return (
      <>
        <PartialFeedback block={block as DashboardBlock<unknown>} />
        <div className="dashboard-state dashboard-state--unavailable" role="status"><strong>No complete result</strong><span>The query ended before an authoritative empty result was available.</span></div>
      </>
    )
  }
  return (
    <>
      <PartialFeedback block={block as DashboardBlock<unknown>} />
      {isEmpty(block.value) ? <div className="dashboard-state dashboard-state--empty" role="status"><strong>Nothing found</strong><span>{emptyCopy}</span></div> : children(block.value)}
    </>
  )
}

function DashboardSection({ id, eyebrow, title, action, children }: { id: string; eyebrow: string; title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="dashboard-section" id={id} aria-labelledby={`${id}-title`}>
      <div className="dashboard-section-heading">
        <div><span className="eyebrow">{eyebrow}</span><h2 id={`${id}-title`}>{title}</h2></div>
        {action}
      </div>
      {children}
    </section>
  )
}

function CounterCard({ label, counter, href, icon }: { label: string; counter: DashboardCounter; href: string; icon: ReactNode }) {
  const card = (
    <>
      <span className="summary-card-icon" aria-hidden="true">{icon}</span>
      <span className="summary-card-label">{label}</span>
      <strong>{counter.value === null ? '—' : counter.value.toLocaleString()}</strong>
      <small className={`counter-state counter-state--${counter.state}`}>{counterCopy[counter.state]}</small>
    </>
  )
  if (counter.state === 'available' || counter.state === 'truncated') {
    return <Link className="summary-card" to={href} aria-label={`${label}: ${counter.value ?? 0}, ${counterCopy[counter.state]}`}>{card}</Link>
  }
  return <div className="summary-card summary-card--disabled" aria-label={`${label}: ${counterCopy[counter.state]}`}>{card}</div>
}

function SummaryCards({ summary, logCounter }: { summary: DashboardSummary; logCounter: DashboardCounter }) {
  const cards = [
    ['Namespaces', summary.namespaces, '/namespaces', <Boxes size={16} key="namespaces" />],
    ['Pods', summary.podsTotal, '/pods', <Activity size={16} key="pods" />],
    ['Healthy pods', summary.podsHealthy, '/pods?status=healthy', <ShieldCheck size={16} key="healthy" />],
    ['Problem pods', summary.podsProblematic, '/pods?status=problematic', <AlertTriangle size={16} key="problems" />],
    ['Degraded workloads', summary.workloadsDegraded, '/workloads?status=Degraded', <Boxes size={16} key="workloads" />],
    ['Restarts', summary.restarts, '/pods?sort=restarts&order=desc', <RotateCcw size={16} key="restarts" />],
    ['Warning events', summary.warningEvents, '/events?type=Warning', <Clock3 size={16} key="events" />],
    ['Possible log matches', logCounter, '#log-scan', <ScrollText size={16} key="logs" />],
  ] as const
  return <div className="summary-grid">{cards.map(([label, counter, href, icon]) => <CounterCard key={label} label={label} counter={counter} href={href} icon={icon} />)}</div>
}

function ProblemsTable({ values }: { values: DashboardProblem[] }) {
  return (
    <div className="table-scroll">
      <table className="dashboard-table">
        <caption>At most one prioritized diagnosis per pod</caption>
        <thead><tr><th>Severity</th><th>Pod</th><th>Diagnosis</th><th>Status</th><th>Age</th></tr></thead>
        <tbody>{values.map((problem) => (
          <tr key={`${problem.namespace}/${problem.pod}`}>
            <td><span className={`severity severity--${problem.severity}`}>{problem.severity}</span></td>
            <td><strong>{problem.pod}</strong><small>{problem.namespace}{problem.container ? ` · ${problem.container}` : ''}</small></td>
            <td><strong>{problem.reason ?? 'No diagnosis reported'}</strong><small>{problem.message ?? `Source: ${problem.source}`}</small></td>
            <td>{problem.status}</td>
            <td>{formatDuration(problem.ageSeconds)}</td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  )
}

function RestartsTable({ values }: { values: DashboardRestart[] }) {
  return (
    <div className="table-scroll">
      <table className="dashboard-table">
        <caption>Container restart ranking, highest count first</caption>
        <thead><tr><th>Restarts</th><th>Pod / owner</th><th>Container</th><th>Status</th><th>Age</th></tr></thead>
        <tbody>{values.map((restart) => (
          <tr key={`${restart.namespace}/${restart.pod}/${restart.containerType}/${restart.container}`}>
            <td><span className={`severity severity--${restart.severity}`}>{restart.restarts}</span></td>
            <td><strong>{restart.pod}</strong><small>{restart.namespace}{restart.owner ? ` · ${restart.owner.kind}/${restart.owner.name}` : ' · owner unavailable'}</small></td>
            <td>{restart.container}<small>{restart.containerType}</small></td>
            <td>{restart.status || restart.lastReason || 'not reported'}</td>
            <td>{formatDuration(restart.ageSeconds)}</td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  )
}

function EventsTable({ values }: { values: DashboardEvent[] }) {
  return (
    <div className="table-scroll">
      <table className="dashboard-table">
        <caption>Grouped Kubernetes Warning events</caption>
        <thead><tr><th>Last seen</th><th>Object</th><th>Reason</th><th>Count</th><th>Source</th></tr></thead>
        <tbody>{values.map((event, index) => (
          <tr key={`${event.namespace}/${event.objectKind}/${event.objectName}/${event.reason}/${index}`}>
            <td>{formatTimestamp(event.timestamp)}</td>
            <td><strong>{event.objectKind}/{event.objectName}</strong><small>{event.namespace}</small></td>
            <td><strong>{event.reason}</strong><small>{event.message}</small></td>
            <td>{event.count}</td>
            <td>{event.source ?? 'not reported'}</td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  )
}

function MetricsTable({ title, values, metric }: { title: string; values: MetricRank[]; metric: 'cpu' | 'memory' }) {
  return (
    <div className="metrics-ranking">
      <h3>{title}</h3>
      <div className="table-scroll">
        <table className="dashboard-table dashboard-table--compact">
          <thead><tr><th>Pod</th><th>CPU</th><th>Memory</th></tr></thead>
          <tbody>{values.map((value) => (
            <tr key={`${metric}/${value.namespace}/${value.pod}`}>
              <td><strong>{value.pod}</strong><small>{value.namespace}</small></td>
              <td>{value.cpuMillicores.toLocaleString()}m</td>
              <td>{formatMemory(value.memoryBytes)}</td>
            </tr>
          ))}</tbody>
        </table>
      </div>
    </div>
  )
}

function MetricsView({ value }: { value: DashboardMetrics }) {
  return (
    <>
      <p className="dashboard-subtle">Metrics window: {formatDuration(value.windowSeconds)} · collected {formatTimestamp(value.collectedAt)}</p>
      <div className="metrics-grid">
        <MetricsTable title="Top CPU" values={value.topCPU} metric="cpu" />
        <MetricsTable title="Top memory" values={value.topMemory} metric="memory" />
      </div>
    </>
  )
}

function LogMatchesTable({ values }: { values: DashboardLogMatch[] }) {
  return (
    <div className="table-scroll">
      <table className="dashboard-table">
        <caption>Possible matches only; excerpts are bounded and sanitized by the backend</caption>
        <thead><tr><th>Detected</th><th>Target</th><th>Reason</th><th>Sanitized excerpt</th></tr></thead>
        <tbody>{values.map((match, index) => (
          <tr key={`${match.namespace}/${match.pod}/${match.container}/${match.timestamp ?? index}`}>
            <td>{formatTimestamp(match.timestamp)}</td>
            <td><strong>{match.pod}/{match.container}</strong><small>{match.namespace}{match.workload ? ` · ${match.workload.kind}/${match.workload.name}` : ''}</small></td>
            <td><code>{match.reasonCode}</code><small>{match.redacted ? 'sensitive value redacted' : 'no redaction needed'}{match.truncated ? ' · excerpt truncated' : ''}</small></td>
            <td><code className="log-excerpt">{match.excerpt}</code></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
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

function DashboardContent({ selection, cluster }: { selection: SelectionSummary; cluster: ComponentState }) {
	const summary = useQuery({ queryKey: ['dashboard', 'summary', selection.generation], queryFn: ({ signal }) => getDashboardSummary(signal, selection.generation), ...dashboardQueryDefaults })
	const problems = useQuery({ queryKey: ['dashboard', 'problems', selection.generation], queryFn: ({ signal }) => getDashboardProblems(signal, selection.generation), ...dashboardQueryDefaults })
	const restarts = useQuery({ queryKey: ['dashboard', 'restarts', selection.generation, 10], queryFn: ({ signal }) => getDashboardRestarts(10, signal, selection.generation), ...dashboardQueryDefaults })
	const events = useQuery({ queryKey: ['dashboard', 'events', selection.generation], queryFn: ({ signal }) => getDashboardEvents(signal, selection.generation), ...dashboardQueryDefaults })
	const metrics = useQuery({ queryKey: ['dashboard', 'metrics', selection.generation], queryFn: ({ signal }) => getDashboardMetrics(signal, selection.generation), ...dashboardQueryDefaults })
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
    ])
  }

  const isRefreshing = [summary, problems, restarts, events, metrics].some((query) => query.isFetching)
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
    <div className="dashboard-page">
      <header className="dashboard-header">
        <div>
          <span className="eyebrow">progressive overview</span>
          <h1>Cluster overview</h1>
          <p>Each block has its own budget, cancellation, and authorization result.</p>
        </div>
        <button type="button" className="button button--secondary" onClick={refreshAll} disabled={isRefreshing}>
          <RefreshCw size={15} aria-hidden="true" /> {isRefreshing ? 'Refreshing…' : 'Refresh dashboard'}
        </button>
      </header>

      <div className="dashboard-context" aria-label="Dashboard selection">
        <div><span>Context</span><strong>{selection.context}</strong><small>{selection.cluster}</small></div>
        <div><span>Scope</span><strong>{selection.scopeName ?? 'No saved scope'}</strong><small>{selection.namespaceCount} namespace{selection.namespaceCount === 1 ? '' : 's'}{selection.defaultNamespace ? ` · default ${selection.defaultNamespace}` : ''}</small></div>
        <div><span>Connection</span><strong className={`connection connection--${cluster.status}`}>{cluster.status}</strong><small>{cluster.message}</small></div>
        <div><span>Last update</span><strong>{collectedAt}</strong><small>generation {selection.generation}</small></div>
        <div className="dashboard-shortcuts"><Link to="/namespaces">Edit scope</Link><Link to="/permissions">View RBAC</Link></div>
      </div>

      <DashboardSection id="summary" eyebrow="at a glance" title="Summary">
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

      <div className="dashboard-columns">
        <DashboardSection id="problems" eyebrow="prioritized" title="Problem pods">
          <ResultBody pending={problems.isPending} error={problems.error} response={problems.data} isEmpty={(value) => value.length === 0} emptyCopy="No problematic pod was found in the completed coverage.">
            {(value) => <ProblemsTable values={value} />}
          </ResultBody>
        </DashboardSection>

        <DashboardSection id="restarts" eyebrow="top 10" title="Container restarts">
          <ResultBody pending={restarts.isPending} error={restarts.error} response={restarts.data} isEmpty={(value) => value.length === 0} emptyCopy="No container restart was found in the completed coverage.">
            {(value) => <RestartsTable values={value} />}
          </ResultBody>
        </DashboardSection>
      </div>

      <DashboardSection id="warning-events" eyebrow="recent signals" title="Warning events">
        <ResultBody pending={events.isPending} error={events.error} response={events.data} isEmpty={(value) => value.length === 0} emptyCopy="No Warning event was found in the completed coverage.">
          {(value) => <EventsTable values={value} />}
        </ResultBody>
      </DashboardSection>

      <DashboardSection id="metrics" eyebrow="optional API" title="Pod metrics">
        <ResultBody pending={metrics.isPending} error={metrics.error} response={metrics.data} isEmpty={(value) => value.pods.length === 0} emptyCopy="The Metrics API returned no pod metrics for the completed coverage." optional>
          {(value) => <MetricsView value={value} />}
        </ResultBody>
      </DashboardSection>

      <DashboardSection
        id="log-scan"
        eyebrow="explicit, bounded, in memory"
        title="Possible errors in logs"
        action={(
          <div className="log-scan-controls">
            <label>Window<select aria-label="Log scan window" value={scanWindow} onChange={(event) => setScanWindow(event.target.value as LogScanRequest['window'])}>
              <option value="15m">15 min</option><option value="30m">30 min</option><option value="1h">1 hour</option><option value="4h">4 hours</option>
            </select></label>
            <button type="button" className="button" onClick={() => void runLogScan()} disabled={!session.data || session.isError}>
              <ScrollText size={15} aria-hidden="true" /> {logScan.kind === 'pending' ? 'Restart scan' : 'Scan logs now'}
            </button>
          </div>
        )}
      >
        <p className="dashboard-subtle">Scans at most 20 pods, 200 lines each, with four concurrent container reads. Results are never saved by this interface.</p>
        {session.isError ? <div className="dashboard-state dashboard-state--unavailable" role="status"><strong>Scan session unavailable</strong><span>The CSRF bootstrap could not be completed.</span></div> : null}
        {logScan.kind === 'idle' ? <div className="dashboard-state dashboard-state--idle" role="status"><strong>Scan has not been run</strong><span>Run it explicitly when recent, bounded log inspection is useful.</span></div> : null}
        {logScan.kind === 'pending' ? <div className="dashboard-state dashboard-state--loading" role="status" aria-busy="true"><strong>Scanning selected targets</strong><span>Starting another scan, switching generation, or leaving this page cancels it.</span></div> : null}
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
      return <StatePanel kind="error" title="The local API returned an error">The response was rejected safely. Retry after checking the local status or running kubePeep doctor.</StatePanel>
    }
    return <StatePanel kind="offline" title="The local API is unavailable">Reload after the kubePeep process is ready.</StatePanel>
  }
  if (!status.data.selection) {
    return <StatePanel kind="empty" title="Choose a Kubernetes context">The local application is ready. Use the context selector in the header to connect a kubeconfig.</StatePanel>
  }
  return <DashboardContent key={status.data.selection.generation} selection={status.data.selection} cluster={status.data.components.cluster} />
}
