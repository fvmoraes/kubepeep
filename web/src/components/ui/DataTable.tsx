import { useEffect, type ReactNode } from 'react'

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
}: DataTableProps<T>) {
  useEffect(() => {
    recordRenderedRowCount(rows.length)
    recordFirstRowRendered(rows)
  }, [rows])

  const cellPadding = compact ? 'px-2.5 py-1.5' : 'px-3 py-2'
  const headerBase = `${cellPadding} border-b border-kp-overlay-0 text-left text-2xs font-medium text-kp-overlay-text uppercase tracking-wider whitespace-nowrap ${stickyHeader ? 'sticky top-0 z-10 bg-kp-surface-0' : ''}`
  const selectedCount = selectedKeys ? rows.filter((row, index) => selectedKeys.has(getRowKey ? getRowKey(row, index) : String(index))).length : 0
  const allSelected = rows.length > 0 && selectedCount === rows.length

  return (
    <div className={`min-w-0 overflow-x-auto ${className}`}>
      <table className="w-full border-collapse text-base">
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
          {rows.map((row, index) => {
            const key = getRowKey ? getRowKey(row, index) : String(index)
            const isSelected = selectedKeys?.has(key) ?? false
            return (
              <tr
                key={key}
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
                  <td
                    key={column.key}
                    className={`${cellPadding} align-top text-kp-subtext ${
                      column.align === 'right' ? 'text-right' : column.align === 'center' ? 'text-center' : ''
                    }`}
                  >
                    {column.cell(row, index)}
                  </td>
                ))}
              </tr>
            )
          })}
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
