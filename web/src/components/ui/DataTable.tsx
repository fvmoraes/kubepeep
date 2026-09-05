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
  const cellPadding = compact ? 'px-2.5 py-1.5' : 'px-3 py-2'

  return (
    <div className={`min-w-0 overflow-x-auto ${className}`}>
      <table className="w-full border-collapse text-base">
        {caption ? <caption className="px-3 py-2 text-left text-xs text-kp-overlay-text">{caption}</caption> : null}
        <thead>
          <tr>
            {columns.map((column) => (
              <th
                key={column.key}
                scope="col"
                className={`${cellPadding} border-b border-kp-overlay-0 text-left text-2xs font-medium text-kp-overlay-text uppercase tracking-wider whitespace-nowrap ${
                  column.align === 'right' ? 'text-right' : column.align === 'center' ? 'text-center' : ''
                }`}
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
                className={`border-b border-kp-divider last:border-b-0 ${onRowClick ? 'cursor-pointer hover:bg-kp-surface-3' : 'hover:bg-kp-surface-2/50'}`}
              >
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
              <td colSpan={columns.length} className="px-3 py-2.5 border-t border-kp-overlay-0">{footer}</td>
            </tr>
          </tfoot>
        ) : null}
      </table>
    </div>
  )
}
