import type { ReactNode } from 'react'
import { RefreshCw, Search } from 'lucide-react'

import { Button, Input, Select } from './ui'

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

/**
 * Compact filter toolbar. Filters, search, sort and refresh fit one row of
 * h-7 controls (plus a thin chips row) so the filter area stays within
 * ~20-30% of the viewport and the table remains the protagonist.
 * Child filter fields must render label-less inline inputs/selects.
 */
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
    <section aria-label="Resource list controls" className="min-w-0">
      <form
        className="list-controls-form flex flex-wrap items-center gap-1.5 rounded-xl border border-kp-overlay-0 bg-kp-surface-0"
        onSubmit={(event) => { event.preventDefault(); onApply() }}
      >
        <div className="relative min-w-[170px] flex-1">
          <Search size={13} aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-kp-overlay-text" />
          <Input
            type="search"
            data-app-shortcut="search"
            aria-label="Search this bounded page"
            aria-keyshortcuts="Control+F Meta+F"
            placeholder="Search (Ctrl+F)"
            maxLength={256}
            className="!h-7 !pl-7 text-sm"
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
          />
        </div>
        {children}
        <Select aria-label="Sort this bounded page" className="!h-7 !w-auto max-w-[11rem] pr-6 text-sm" value={sort} onChange={(event) => onSortChange(event.target.value)}>{sortOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select>
        <Select aria-label="Order" className="!h-7 !w-[6.5rem] pr-6 text-sm" value={order} onChange={(event) => onOrderChange(event.target.value as ListSortOrder)}><option value="asc">Ascending</option><option value="desc">Descending</option></Select>
        <div className="flex items-center gap-1.5">
          <Button type="submit" size="sm">Apply filters</Button>
          <Button variant="secondary" size="sm" aria-keyshortcuts="Control+R Meta+R" onClick={onRefresh} data-tip="Refresh this page"><RefreshCw size={12} aria-hidden="true" /> Refresh</Button>
          <Button variant="ghost" size="sm" disabled={!canClear} onClick={onClear}>Clear filters</Button>
        </div>
      </form>
      <div className={`flex flex-wrap items-center gap-x-3 gap-y-1 px-1 text-xs text-kp-overlay-text ${appliedFilters.length > 0 || hasPendingChanges ? 'list-controls-chips py-1' : ''}`} aria-label="Applied resource list state" aria-live="polite">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-2xs uppercase tracking-wider">Filters</span>
          {appliedFilters.length === 0
            ? <span>None</span>
            : <ul className="flex flex-wrap gap-1.5">{appliedFilters.map((filter) => <li key={filter.id} className="inline-flex max-w-[260px] items-center gap-1 rounded-full border border-kp-overlay-0 bg-kp-surface-1 px-2 py-0"><span className="text-kp-overlay-text">{filter.label}</span><strong className="overflow-hidden text-ellipsis whitespace-nowrap font-normal text-kp-subtext">{filter.value}</strong></li>)}</ul>}
        </div>
        <span className="text-2xs uppercase tracking-wider">Order</span>
        <strong className="font-normal text-kp-subtext">{sortLabel} · {appliedOrder === 'asc' ? 'ascending' : 'descending'}</strong>
        {hasPendingChanges ? <p className="pending-filter-change m-0 text-kp-yellow" role="status">Filter changes pending; apply filters to update the bounded result.</p> : null}
      </div>
    </section>
  )
}
