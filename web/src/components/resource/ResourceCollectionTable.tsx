import { useQueryClient } from '@tanstack/react-query'

import { type DataTableColumn } from '../ui'
import { SelectableResourceTable } from './SelectableResourceTable'
import type { ColumnVisibilityState } from './columns'
import { InfiniteCollectionFooter, QueryState } from './states'
import type { useInfiniteCollection } from './useInfiniteCollection'

/** The same bounded, continuously updated inventory presentation as Pods. */
export function ResourceCollectionTable<T>({ collection, caption, columns, getRowKey, columnVisibility }: {
  collection: ReturnType<typeof useInfiniteCollection<T>>
  caption: string
  columns: DataTableColumn<T>[]
  getRowKey: (row: T) => string
  columnVisibility?: ColumnVisibilityState
}) {
  const client = useQueryClient()
  const { query, items, lastPage } = collection
  return (
    <QueryState pending={query.isPending} error={!query.data || collection.authorizationFailed ? query.error : null} empty={items.length === 0 && !query.hasNextPage}>
        <div className="resource-collection min-w-0 rounded-xl border border-kp-overlay-0 bg-kp-surface-0">
        <SelectableResourceTable selectionIdentity={collection.queryKey} caption={caption} rows={items} columns={columns} getRowKey={getRowKey} columnVisibility={columnVisibility} onScrollProgress={collection.onScrollProgress} stickyHeader virtualize />
        {lastPage ? <InfiniteCollectionFooter result={lastPage} itemCount={items.length} pageCount={query.data?.pages.length ?? 0} firstPage={query.data?.pageParams[0] === '' && query.data.pages.length === 1} hasNextPage={Boolean(query.hasNextPage)} loading={query.isFetching} refreshing={query.isPlaceholderData} nextPageError={collection.nextPageError} onNext={() => void collection.loadNextPage()} onRestart={() => void client.resetQueries({ queryKey: collection.queryKey })} /> : null}
      </div>
    </QueryState>
  )
}
