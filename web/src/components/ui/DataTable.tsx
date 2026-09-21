import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'

import { recordFirstRowRendered, recordRenderedRowCount } from '../../observability/uxMetrics'
import { Checkbox } from './Checkbox'

export interface DataTableColumn<T> {
  key: string
  header: ReactNode
  width?: string
  align?: 'left' | 'right' | 'center'
  sortable?: boolean
  sortKey?: string
  cell: (row: T, index: number) => ReactNode
}

export interface DataTableProps<T> {
  columns: DataTableColumn<T>[]
  rows: T[]
  caption?: ReactNode
  footer?: ReactNode
  compact?: boolean
  className?: string
  onRowClick?: (row: T, index: number) => void
  getRowKey?: (row: T, index: number) => string
  /** Row-selection support: renders a leading checkbox column. */
  selectable?: boolean
  selectedKeys?: ReadonlySet<string>
  onToggleRow?: (key: string, checked: boolean) => void
  onToggleAll?: (checked: boolean) => void
  stickyHeader?: boolean
  /** Opt out for a table whose rows cannot be measured in a scroll viewport. */
  virtualize?: boolean
  onScrollProgress?: (fraction: number) => void
}

export function DataTable<T>({
  columns,
  rows,
  caption,
  footer,
  compact = false,
  className = '',
  onRowClick,
  getRowKey,
  selectable = false,
  selectedKeys,
  onToggleRow,
  onToggleAll,
  stickyHeader = false,
  virtualize,
  onScrollProgress,
}: DataTableProps<T>) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const virtualized = virtualize ?? rows.length > 100
  // TanStack owns mutable scroll measurements; React Compiler must not memoize this hook.
  // eslint-disable-next-line react-hooks/incompatible-library
  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => compact ? 32 : 40,
    getItemKey: (index) => getRowKey ? getRowKey(rows[index], index) : index,
    overscan: 2,
    enabled: virtualized,
    initialRect: { width: 800, height: 440 },
  })
  const virtualRows = virtualized ? rowVirtualizer.getVirtualItems() : []
  // A zero-size viewport (SSR/tests or a temporarily hidden panel) should
  // still show the first bounded slice until the observer measures the pane.
  const visibleVirtualRows = virtualized && virtualRows.length === 0
    ? rows.slice(0, 20).map((_, index) => ({ index, start: index * (compact ? 32 : 40), end: (index + 1) * (compact ? 32 : 40) }))
    : virtualRows
  useEffect(() => {
    recordFirstRowRendered(rows)
  }, [rows])
  useEffect(() => {
    recordRenderedRowCount(virtualized ? visibleVirtualRows.length : rows.length)
  }, [rows.length, virtualized, visibleVirtualRows.length])

  const cellPadding = compact ? 'px-2.5 py-1.5' : 'px-3 py-2'
  const headerBase = `${cellPadding} border-b border-kp-overlay-0 text-left text-2xs font-medium text-kp-overlay-text uppercase tracking-wider whitespace-nowrap ${stickyHeader || virtualized ? 'sticky top-0 z-10 bg-kp-surface-0' : ''}`
  const selectedCount = useMemo(() => selectedKeys ? rows.filter((row, index) => selectedKeys.has(getRowKey ? getRowKey(row, index) : String(index))).length : 0, [getRowKey, rows, selectedKeys])
  const allSelected = rows.length > 0 && selectedCount === rows.length
  const before = visibleVirtualRows[0]?.start ?? 0
  const after = virtualized ? Math.max(0, rowVirtualizer.getTotalSize() - (visibleVirtualRows.at(-1)?.end ?? 0)) : 0

  const renderRow = (row: T, index: number, measure: boolean) => {
    const key = getRowKey ? getRowKey(row, index) : String(index)
    const isSelected = selectedKeys?.has(key) ?? false
    return (
      <tr
        key={key}
        ref={measure ? rowVirtualizer.measureElement : undefined}
        data-index={measure ? index : undefined}
        aria-rowindex={index + 2}
        onClick={onRowClick ? () => onRowClick(row, index) : undefined}
        data-selected={selectable && isSelected ? 'true' : undefined}
        className={`border-b border-kp-divider last:border-b-0 ${isSelected ? 'bg-kp-accent-bg/50' : ''} ${onRowClick ? 'cursor-pointer hover:bg-kp-surface-3' : 'hover:bg-kp-surface-2/50'}`}
      >
        {selectable ? (
          <td className={`${cellPadding} align-top pr-0`}>
            <Checkbox
              aria-label={`Select row ${key}`}
              checked={isSelected}
              onClick={(event) => event.stopPropagation()}
              onChange={(event) => onToggleRow?.(key, event.target.checked)}
            />
          </td>
        ) : null}
        {columns.map((column) => (
          <td key={column.key} className={`${cellPadding} align-top text-kp-subtext ${column.align === 'right' ? 'text-right' : column.align === 'center' ? 'text-center' : ''}`}>
            {column.cell(row, index)}
          </td>
        ))}
      </tr>
    )
  }

  return (
    <div
      ref={scrollRef}
      className={`min-w-0 overflow-x-auto ${virtualized ? 'max-h-[440px] overflow-y-auto' : ''} ${className}`}
      onScroll={onScrollProgress ? (event) => {
        const element = event.currentTarget
        const distance = element.scrollHeight - element.clientHeight
        onScrollProgress(distance <= 0 ? 0 : element.scrollTop / distance)
      } : undefined}
    >
      <table className="w-full border-collapse text-base" aria-rowcount={virtualized ? rows.length + 1 : undefined}>
        {caption ? <caption className="px-3 py-2 text-left text-xs text-kp-overlay-text">{caption}</caption> : null}
        <thead>
          <tr>
            {selectable ? (
              <th scope="col" className={`${headerBase} w-8 pr-0`}>
                <Checkbox
                  aria-label={allSelected ? 'Clear selection' : 'Select all rows on this page'}
                  checked={allSelected}
                  onChange={(event) => onToggleAll?.(event.target.checked)}
                />
              </th>
            ) : null}
            {columns.map((column) => (
              <th
                key={column.key}
                scope="col"
                className={`${headerBase} ${column.align === 'right' ? 'text-right' : column.align === 'center' ? 'text-center' : ''}`}
                style={{ width: column.width }}
              >
                {column.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {virtualized && before > 0 ? <tr aria-hidden="true" role="presentation"><td colSpan={columns.length + (selectable ? 1 : 0)} style={{ height: before, padding: 0, border: 0 }} /></tr> : null}
          {virtualized ? visibleVirtualRows.map((virtualRow) => renderRow(rows[virtualRow.index], virtualRow.index, true)) : rows.map((row, index) => renderRow(row, index, false))}
          {virtualized && after > 0 ? <tr aria-hidden="true" role="presentation"><td colSpan={columns.length + (selectable ? 1 : 0)} style={{ height: after, padding: 0, border: 0 }} /></tr> : null}
        </tbody>
        {footer ? (
          <tfoot>
            <tr>
              <td colSpan={columns.length + (selectable ? 1 : 0)} className="px-3 py-2.5 border-t border-kp-overlay-0">{footer}</td>
            </tr>
          </tfoot>
        ) : null}
      </table>
    </div>
  )
}
