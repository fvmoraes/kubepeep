import type { ReactNode } from 'react'

import type { CollectionResult } from '../../api/types'
import { Button, EmptyState } from '../ui'
import { StatePanel } from '../StatePanel'
import { errorCode, errorMessage } from './errors'

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

export function QueryState({ pending, error, empty, children }: { pending: boolean; error: unknown; empty: boolean; children: ReactNode }) {
  if (pending) return <StatePanel kind="loading" title="Loading resources">The request is tied to the active selection generation.</StatePanel>
  if (error) return <StatePanel kind="error" title="Resource request failed" details={errorCode(error)}>{errorMessage(error)}</StatePanel>
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

export function CollectionFooter<T>({ result, onNext, onRestart }: { result: CollectionResult<T>; onNext: (cursor: string) => void; onRestart: () => void }) {
  const coverage = result.coverage
  return (
    <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-kp-overlay-0 px-3 py-2.5">
      <div className="min-w-0 text-xs text-kp-overlay-text">
        <span className="block text-kp-subtext">{result.items.length} item{result.items.length === 1 ? '' : 's'} in this page</span>
        <small className="block">
          {result.page.complete ? 'Collection complete' : `Bounded ${result.page.filterScope} result`}{result.page.truncated ? ' · truncated' : ''}
        </small>
        {coverage ? (
          <small className="block">
            {coverage.requestedNamespaces === 0
              ? 'Cluster-scoped result'
              : `${coverage.completedNamespaces}/${coverage.requestedNamespaces} namespaces completed · ${coverage.deniedNamespaces.length} denied`}
            {coverage.failed.length ? ` · ${coverage.failed.length} failed` : ''}
          </small>
        ) : null}
      </div>
      <div className="flex gap-2">
        <Button variant="secondary" size="sm" onClick={onRestart}>First page</Button>
        <Button size="sm" disabled={!result.page.next} onClick={() => onNext(result.page.next)}>Next page</Button>
      </div>
    </footer>
  )
}
