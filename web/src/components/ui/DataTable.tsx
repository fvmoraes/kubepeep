import type { ReactNode } from 'react'

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
}: DataTableProps<T>) {
  const cellPadding = compact ? 'px-2.5 py-2' : 'px-3 py-2.5'
  const headerPadding = compact ? 'px-2.5 py-2' : 'px-3 py-2.5'

  return (
    <div className={`min-w-0 overflow-x-auto ${className}`}>
      <table className="w-full border-collapse text-xs">
        {caption ? <caption className="px-3 py-2.5 text-left text-kp-overlay-text">{caption}</caption> : null}
        <thead>
          <tr>
            {columns.map((column) => (
              <th
                key={column.key}
                scope="col"
                className={`
                  ${headerPadding} border-b border-kp-overlay-0
                  text-left text-kp-overlay-text uppercase tracking-wide text-2xs whitespace-nowrap
                  ${column.align === 'right' ? 'text-right' : column.align === 'center' ? 'text-center' : ''}
                `}
                style={{ width: column.width }}
              >
                {column.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => {
            const key = getRowKey ? getRowKey(row, index) : index
            return (
              <tr
                key={key}
                onClick={onRowClick ? () => onRowClick(row, index) : undefined}
                className={onRowClick ? 'cursor-pointer hover:bg-kp-surface-1' : ''}
              >
                {columns.map((column) => (
                  <td
                    key={column.key}
                    className={`
                      ${cellPadding} border-b border-kp-overlay-0 align-top
                      ${column.align === 'right' ? 'text-right' : column.align === 'center' ? 'text-center' : ''}
                    `}
                  >
                    {column.cell(row, index)}
                  </td>
                ))}
              </tr>
            )
          })}
        </tbody>
        {footer ? <tfoot><tr><td colSpan={columns.length} className="px-3 py-3 border-t border-kp-overlay-0">{footer}</td></tr></tfoot> : null}
      </table>
    </div>
  )
}
