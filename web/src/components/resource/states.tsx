import type { ReactNode } from 'react'
import { Link } from 'react-router'

import type { CollectionResult } from '../../api/types'
import { Button, EmptyState } from '../ui'
import { StatePanel } from '../StatePanel'
import { errorCode, errorMessage } from './errors'
import { LoadingState } from '../ui/LoadingState'
import { useAutoRefresh } from './AutoRefreshProvider'
import { resourceRefreshInterval } from './autoRefresh'

function RecoveryNote({ error }: { error?: unknown }) {
  const { enabled } = useAutoRefresh()
  if (error && resourceRefreshInterval(error) === false) return null
  return <small className="block mt-2 text-kp-overlay-text">{enabled ? 'Automatic retry every 10 seconds while this view is active.' : 'Automatic refresh is off. Enable Auto · 10s or retry the collection to check again.'}</small>
}

function EmptySelection() {
  return (
    <StatePanel kind="empty" title="Choose a Kubernetes context">
      Select a context and namespace scope before querying cluster resources.
    </StatePanel>
  )
}

export function SelectionGate({ pending, error, selected, children }: { pending: boolean; error: unknown; selected: boolean; children: ReactNode }) {
  if (pending) return <StatePanel kind="loading" title="Loading active selection">The local service is resolving the current generation.</StatePanel>
  if (error) return <StatePanel kind="error" title="Selection unavailable" details={errorCode(error)}>{errorMessage(error)}</StatePanel>
  if (!selected) return <EmptySelection />
  return children
}

// Request timing is owned by the transport and DataTable commit so cache hits,
// background refetches and retries do not depend on isPending transitions.
export function QueryState({ pending, error, empty, children }: { pending: boolean; error: unknown; empty: boolean; children: ReactNode }) {
  if (pending) return <LoadingState label="Loading resources…" layout="table" />
  if (error) return <StatePanel kind="error" title="Resource request failed" details={errorCode(error)}>{errorMessage(error)}<RecoveryNote error={error} /></StatePanel>
  if (empty) {
    return (
      <EmptyState
        title="No matching resources"
        description="Nothing was returned for the current filters inside the active namespace scope."
      />
    )
  }
  return children
}

export function CollectionCoverage({ coverage }: { coverage: CollectionResult<unknown>['coverage'] }) {
  if (!coverage) return null
  const failed = coverage.failed.length
  const summary = <small className={`block ${failed ? 'text-kp-yellow' : ''}`} role={failed ? 'note' : undefined}>
    {failed ? 'Partial result · ' : ''}{coverage.requestedNamespaces === 0 ? 'Cluster-scoped result' : `${coverage.completedNamespaces}/${coverage.requestedNamespaces} namespaces completed · ${coverage.deniedNamespaces.length} denied`}{failed ? ` · ${failed} failed` : ''}
  </small>
  if (!failed) return summary
  return <details className="text-content"><summary className="cursor-pointer text-kp-yellow">{summary}</summary><ul className="my-2 pl-4">{coverage.failed.map((failure, index) => <li key={`${failure.namespace}/${failure.code}/${index}`}>{failure.namespace || 'Cluster'} · {failure.code}: {failure.message}</li>)}</ul>{coverage.failed.some((failure) => failure.code !== 'FORBIDDEN') ? <RecoveryNote /> : null}</details>
}

export function InfiniteCollectionFooter<T>({ result, itemCount, pageCount, firstPage, hasNextPage, loading, refreshing, nextPageError, onNext, onRestart }: {
  result: CollectionResult<T>
  itemCount: number
  pageCount: number
  firstPage: boolean
  hasNextPage: boolean
  loading: boolean
  refreshing: boolean
  nextPageError: boolean
  onNext: () => void
  onRestart: () => void
}) {
  const coverage = result.coverage
  return (
    <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-kp-overlay-0 px-3 py-2.5">
      <div className="min-w-0 text-content text-kp-overlay-text">
        <span className="block text-kp-subtext">{itemCount} item{itemCount === 1 ? '' : 's'} loaded · {pageCount} page{pageCount === 1 ? '' : 's'} retained</span>
        <small className="block">{result.page.complete ? 'Collection complete' : `Bounded ${result.page.filterScope} result`}{result.page.truncated ? ' · truncated' : ''}</small>
        {result.snapshotRenewed ? <small className="block text-kp-yellow" role="status">The list snapshot expired and was renewed from the first page.</small> : null}
        <CollectionCoverage coverage={coverage} />
      </div>
      <div className="flex gap-2">
        <Link className="self-center text-content text-kp-sky" to="/settings#performance">Performance</Link>
        {coverage?.failed.length ? <Button variant="secondary" disabled={loading} onClick={onRestart}>Retry collection</Button> : null}
        <Button variant="secondary" disabled={firstPage} disabledReason="Already on the first page." onClick={onRestart}>First page</Button>
        <Button disabled={!hasNextPage || loading || refreshing} disabledReason="The current result has no next page or is refreshing." onClick={onNext}>{loading ? 'Loading…' : 'Load next page'}</Button>
      </div>
      {nextPageError ? <p className="w-full text-content text-kp-red" role="alert">The next page could not be loaded. Loaded resources remain available; retry when ready.</p> : null}
    </footer>
  )
}
