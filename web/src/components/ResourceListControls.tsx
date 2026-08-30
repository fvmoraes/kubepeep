import type { ReactNode } from 'react'

export type ListSortOrder = 'asc' | 'desc'

export interface ActiveListFilter {
  id: string
  label: string
  value: string
}

export interface ListSortOption {
  value: string
  label: string
}

interface ResourceListControlsProps {
  search: string
  appliedSearch: string
  onSearchChange: (value: string) => void
  onApply: () => void
  onRefresh: () => void
  onClear: () => void
  activeFilters?: ActiveListFilter[]
  sort: string
  order: ListSortOrder
  appliedSort: string
  appliedOrder: ListSortOrder
  defaultSort: string
  defaultOrder: ListSortOrder
  hasPendingChanges: boolean
  sortOptions: readonly ListSortOption[]
  onSortChange: (value: string) => void
  onOrderChange: (value: ListSortOrder) => void
  children?: ReactNode
}

export function ResourceListControls({
  search,
  appliedSearch,
  onSearchChange,
  onApply,
  onRefresh,
  onClear,
  activeFilters = [],
  sort,
  order,
  appliedSort,
  appliedOrder,
  defaultSort,
  defaultOrder,
  hasPendingChanges,
  sortOptions,
  onSortChange,
  onOrderChange,
  children,
}: ResourceListControlsProps) {
  const appliedFilters = appliedSearch === ''
    ? activeFilters
    : [{ id: 'search', label: 'Search', value: appliedSearch }, ...activeFilters]
  const canClear = hasPendingChanges || search !== '' || appliedSearch !== '' || activeFilters.length > 0 || sort !== defaultSort || order !== defaultOrder || appliedSort !== defaultSort || appliedOrder !== defaultOrder
  const sortLabel = sortOptions.find((option) => option.value === appliedSort)?.label ?? appliedSort

  return (
    <section className="resource-list-controls" aria-label="Resource list controls">
      <form className="resource-filters" onSubmit={(event) => { event.preventDefault(); onApply() }}>
        <label className="resource-search">Search this bounded page<input value={search} maxLength={256} onChange={(event) => onSearchChange(event.target.value)} /></label>
        {children}
        <label>Sort this bounded page<select value={sort} onChange={(event) => onSortChange(event.target.value)}>{sortOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
        <label>Order<select value={order} onChange={(event) => onOrderChange(event.target.value as ListSortOrder)}><option value="asc">Ascending</option><option value="desc">Descending</option></select></label>
        <div className="resource-filter-actions">
          <button className="button" type="submit">Apply filters</button>
          <button className="button button--secondary" type="button" onClick={onRefresh}>Refresh</button>
          <button className="button button--secondary" type="button" disabled={!canClear} onClick={onClear}>Clear filters</button>
        </div>
      </form>
      <div className="active-filter-summary" aria-label="Applied resource list state" aria-live="polite">
        <div>
          <strong>Active filters</strong>
          {appliedFilters.length === 0
            ? <span className="active-filter-empty">None</span>
            : <ul>{appliedFilters.map((filter) => <li key={filter.id}><span>{filter.label}</span><strong>{filter.value}</strong></li>)}</ul>}
        </div>
        <p><span>Bounded-page order</span><strong>{sortLabel} · {appliedOrder === 'asc' ? 'ascending' : 'descending'}</strong></p>
        {hasPendingChanges ? <p className="pending-filter-change" role="status">Filter changes pending; apply filters to update the bounded result.</p> : null}
      </div>
    </section>
  )
}
