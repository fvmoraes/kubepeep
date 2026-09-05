import type { ReactNode } from 'react'

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

/** Shared filter toolbar — identical structure across Pods, Workloads, Events, Network and Config. */
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
        className="flex flex-wrap items-end gap-2.5 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-3"
        onSubmit={(event) => { event.preventDefault(); onApply() }}
      >
        <label className="grid flex-1 gap-1 min-w-[220px]">
          <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Search this bounded page</span>
          <Input type="search" data-app-shortcut="search" aria-label="Search this bounded page" aria-keyshortcuts="Control+F Meta+F" value={search} maxLength={256} onChange={(event) => onSearchChange(event.target.value)} />
        </label>
        {children}
        <label className="grid gap-1 min-w-[160px]">
          <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Sort this bounded page</span>
          <Select aria-label="Sort this bounded page" value={sort} onChange={(event) => onSortChange(event.target.value)}>{sortOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select>
        </label>
        <label className="grid gap-1 w-[9.5rem]">
          <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Order</span>
          <Select aria-label="Order" value={order} onChange={(event) => onOrderChange(event.target.value as ListSortOrder)}><option value="asc">Ascending</option><option value="desc">Descending</option></Select>
        </label>
        <div className="flex gap-2">
          <Button type="submit">Apply filters</Button>
          <Button variant="secondary" aria-keyshortcuts="Control+R Meta+R" onClick={onRefresh}>Refresh</Button>
          <Button variant="secondary" disabled={!canClear} onClick={onClear}>Clear filters</Button>
        </div>
      </form>
      <div className="flex flex-wrap items-start gap-x-4 gap-y-1.5 px-1 py-1.5 text-xs text-kp-overlay-text" aria-label="Applied resource list state" aria-live="polite">
        <div className="flex flex-wrap items-center gap-1.5">
          <strong className="text-2xs uppercase tracking-wider text-kp-overlay-text">Active filters</strong>
          {appliedFilters.length === 0
            ? <span>None</span>
            : <ul className="flex flex-wrap gap-1.5">{appliedFilters.map((filter) => <li key={filter.id} className="inline-flex max-w-[280px] items-center gap-1.5 rounded-full border border-kp-overlay-0 bg-kp-surface-1 px-2 py-0.5"><span className="text-kp-overlay-text">{filter.label}</span><strong className="overflow-hidden text-ellipsis whitespace-nowrap font-normal text-kp-subtext">{filter.value}</strong></li>)}</ul>}
        </div>
        <p className="m-0 flex items-center gap-1.5"><span className="text-2xs uppercase tracking-wider">Order</span><strong className="font-normal text-kp-subtext">{sortLabel} · {appliedOrder === 'asc' ? 'ascending' : 'descending'}</strong></p>
        {hasPendingChanges ? <p className="pending-filter-change basis-full m-0 text-kp-yellow" role="status">Filter changes pending; apply filters to update the bounded result.</p> : null}
      </div>
    </section>
  )
}
