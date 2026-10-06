import { useEffect, useRef } from 'react'
import { Search } from 'lucide-react'

import { beginListInteraction, type ListInteractionID } from '../observability/uxMetrics'
import { Input } from './ui'

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
  onApply: (interactionId: ListInteractionID) => void

}

/** One debounced string search; ordering and value filters live in the table. */
export function ResourceListControls({ search, appliedSearch, onSearchChange, onApply }: ResourceListControlsProps) {
  const apply = useRef(onApply)
  useEffect(() => { apply.current = onApply }, [onApply])
  useEffect(() => {
    if (search === appliedSearch) return
    const timer = setTimeout(() => apply.current(beginListInteraction('filter')), 250)
    return () => clearTimeout(timer)
  }, [search, appliedSearch])
  return <form aria-label="Resource list controls" className="relative min-w-0" onSubmit={(event) => { event.preventDefault(); if (search !== appliedSearch) onApply(beginListInteraction('filter')) }}>
    <Search size={14} aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-kp-overlay-text" />
    <Input type="search" data-app-shortcut="search" aria-label="Search resources" aria-keyshortcuts="Control+F Meta+F" placeholder="Search (Ctrl+F)" maxLength={256} className="!pl-8 text-content" value={search} onChange={(event) => onSearchChange(event.target.value)} />
  </form>
}
