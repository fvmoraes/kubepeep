import { useEffect, useMemo, useRef, useState, isValidElement, type ReactNode } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'

import { recordFirstRowRendered, recordRenderedRowCount, recordVisibleRowRendered } from '../../observability/uxMetrics'
import { ArrowDown, ArrowUp, ArrowLeft, ArrowRight, ChevronDown, Columns3 } from 'lucide-react'
import { TableMenu } from './TableMenu'
import type { ColumnVisibilityState } from '../resource/columns'

import { Checkbox } from './Checkbox'
import { initialColumns } from '../resource/initialColumns'

export interface DataTableColumn<T> {
  initialRole?: string
  defaultHidden?: boolean
  key: string
  header: ReactNode
  width?: string
  align?: 'left' | 'right' | 'center'
  sortable?: boolean
  sortKey?: string
  value?: (row: T) => string | number | null | undefined
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
  onToggleAll?: (checked: boolean, visibleRows: T[]) => void
  columnVisibility?: ColumnVisibilityState
  rowGroup?: (row: T) => number
  stickyHeader?: boolean
  /** Opt out for a table whose rows cannot be measured in a scroll viewport. */
  virtualize?: boolean
  onScrollProgress?: (fraction: number) => void
}

export function DataTable<T>({
  columns: allColumns,
  rows: sourceRows,
  caption,
  footer,
  compact = true,
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
  columnVisibility,
  rowGroup,
}: DataTableProps<T>) {
  const [sort, setSort] = useState<{ key: string; descending: boolean } | null>(null)
  const [excluded, setExcluded] = useState<Record<string, string[]>>({})
  const [hidden, setHidden] = useState<string[]>([])
  const [localOrder, setLocalOrder] = useState<string[]>([])
  const draggedColumn = useRef<string | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const [initialColumnCount, setInitialColumnCount] = useState(9)
  useEffect(() => {
    const element = scrollRef.current
    if (!element || typeof ResizeObserver === 'undefined') return
    const measure = () => { const width = element.clientWidth; if (width > 0) setInitialColumnCount(width < 600 ? 3 : width < 920 ? 5 : 9) }
    const observer = new ResizeObserver(measure); observer.observe(element); measure()
    return () => observer.disconnect()
  }, [])
  const identifier = allColumns.find((column) => column.key === 'name')?.key ?? allColumns[0]?.key
  const initial = Boolean(columnVisibility?.useInitialVisibility)
  const defaults = initialColumns(allColumns, initial ? initialColumnCount : 9)
  const baseVisibility = columnVisibility ?? { hidden, toggle: (key: string) => setHidden((current) => current.includes(key) ? current.filter((item) => item !== key) : [...current, key]), reset: () => setHidden([]) }
  const effectiveHidden = initial ? defaults.hidden : baseVisibility.hidden
  const visibility = { ...baseVisibility, hidden: effectiveHidden, toggle: (key: string) => columnVisibility?.setHidden ? columnVisibility.setHidden(effectiveHidden.includes(key) ? effectiveHidden.filter(item => item !== key) : [...effectiveHidden, key]) : baseVisibility.toggle(key) }
  const order = columnVisibility ? columnVisibility.order?.length ? columnVisibility.order : defaults.order : localOrder
  const orderedColumns = [...allColumns].sort((left, right) => {
    const position = (key: string) => { const index = order.indexOf(key); return index < 0 ? order.length : index }
    return position(left.key) - position(right.key)
  })
  const columns = orderedColumns.filter((column) => column.key === identifier || !visibility.hidden.includes(column.key))
  const fitsInitialSet = Boolean(columnVisibility) && (initial || initialColumnCount === 9) && columns.every(column => !defaults.hidden.includes(column.key))
  const hasUsageColumns = columns.some(column => column.key === 'cpu' || column.key === 'memory')
  const initialWidth = (key: string): string | undefined => hasUsageColumns
    ? ({ namespace: '12%', name: '22%', status: '9%', ready: '5%', restarts: '6%', cpu: '14%', memory: '16%', type: '6%', age: '5%' } as Record<string, string>)[key]
    : key === identifier ? initialColumnCount === 3 ? '42%' : '25%' : key === 'namespace' ? initialColumnCount === 3 ? '20%' : '14%' : undefined
  function moveColumn(index: number, offset: number) {
    const next = orderedColumns.map((column) => column.key)
    ;[next[index], next[index + offset]] = [next[index + offset], next[index]]
    if (columnVisibility?.reorder) columnVisibility.reorder(next)
    else setLocalOrder(next)
  }
  const values = useMemo(() => sourceRows.map((row, index) => new Map(allColumns.map((column) => [column.key, columnValue(column, row, index)]))), [sourceRows, allColumns])
  const rows = useMemo(() => sourceRows.map((row, index) => ({ row, index })).filter(({ index }) =>
    allColumns.every((column) => !excluded[column.key]?.includes(String(values[index].get(column.key) ?? '—'))))
    .sort((a, b) => {
      const group = rowGroup ? rowGroup(a.row) - rowGroup(b.row) : 0
      if (group) return group
      if (!sort) return a.index - b.index
      const left = values[a.index].get(sort.key)
      const right = values[b.index].get(sort.key)
      if (left == null || right == null) return left == null ? right == null ? a.index - b.index : 1 : -1
      const order = typeof left === 'number' && typeof right === 'number' ? left - right : collator.compare(String(left), String(right))
      return (sort.descending ? -order : order) || a.index - b.index
    }).map(({ row }) => row), [sourceRows, allColumns, excluded, values, rowGroup, sort])
  // A normal resource page contains 100 rows. Give that page its own scroll
  // viewport so reaching 75% can request the next bounded page.
  const virtualized = virtualize ?? true
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
    recordFirstRowRendered(sourceRows)
    recordVisibleRowRendered(sourceRows)
  }, [sourceRows])
  useEffect(() => {
    recordRenderedRowCount(virtualized ? visibleVirtualRows.length : rows.length)
  }, [rows.length, virtualized, visibleVirtualRows.length])

  const cellPadding = compact ? 'px-2 py-1' : 'px-3 py-1.5'
  const headerBase = `${cellPadding} border-b border-kp-overlay-0 text-left text-column font-bold text-kp-overlay-text uppercase tracking-wider whitespace-nowrap ${stickyHeader || virtualized ? 'sticky top-0 z-10 bg-kp-surface-0' : ''}`
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
          <td className={`${cellPadding} align-middle pr-0`}>
            <Checkbox
              aria-label={`Select row ${key}`}
              checked={isSelected}
              onClick={(event) => event.stopPropagation()}
              onChange={(event) => onToggleRow?.(key, event.target.checked)}
            />
          </td>
        ) : null}
        {columns.map((column) => (
          <td key={column.key} className={`${cellPadding} align-middle text-kp-subtext ${column.align === 'right' ? 'text-right' : column.align === 'center' ? 'text-center' : ''}`}>
            <div className="table-cell-content" title={String(columnValue(column, row, index) ?? '')}>{column.cell(row, index)}</div>
          </td>
        ))}
        <td />
      </tr>
    )
  }

  return (
    <div
      ref={scrollRef}
      className={`data-table min-w-0 overflow-x-auto ${virtualized ? 'data-table--virtual overflow-y-auto' : ''} ${className}`}
      onScroll={onScrollProgress ? (event) => {
        const element = event.currentTarget
        const distance = element.scrollHeight - element.clientHeight
        onScrollProgress(distance <= 0 ? 0 : element.scrollTop / distance)
      } : undefined}
    >
      <table className={`w-full border-collapse text-content${fitsInitialSet ? ' data-table-initial' : ''}`} aria-rowcount={virtualized ? rows.length + 1 : undefined}>
        {fitsInitialSet ? <colgroup>{selectable ? <col style={{ width: 28 }} /> : null}{columns.map(column => <col key={column.key} style={{ width: initialWidth(column.key) }} />)}<col style={{ width: 30 }} /></colgroup> : null}
        {caption ? <caption className="sr-only">{caption}</caption> : null}
        <thead>
          <tr>
            {selectable ? (
              <th scope="col" className={`${headerBase} w-8 pr-0`}>
                <Checkbox
                  aria-label={allSelected ? 'Clear selection' : 'Select all loaded rows'}
                  title="Select loaded rows matching the column filters"
                  checked={allSelected}
                  onChange={(event) => onToggleAll?.(event.target.checked, rows)}
                />
              </th>
            ) : null}
            {columns.map((column) => (
              <th
                key={column.key}
                draggable={Boolean(columnVisibility?.reorder)}
                onDragStart={event => { draggedColumn.current = column.key; event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', column.key) }}
                onDragEnd={() => { draggedColumn.current = null }}
                onDragOver={event => { if (draggedColumn.current) event.preventDefault() }}
                onDrop={event => {
                  event.preventDefault()
                  const from = orderedColumns.findIndex(item => item.key === draggedColumn.current)
                  const to = orderedColumns.findIndex(item => item.key === column.key)
                  if (from >= 0 && to >= 0 && from !== to) { const next = orderedColumns.map(item => item.key); next.splice(to, 0, next.splice(from, 1)[0]); columnVisibility?.reorder?.(next) }
                  draggedColumn.current = null
                }}
                title={columnVisibility?.reorder ? `${textContent(column.header)} — drag to reorder, or use Choose visible columns` : undefined}
                scope="col"
                aria-label={textContent(column.header)}
                className={`${headerBase} ${column.align === 'right' ? 'text-right' : column.align === 'center' ? 'text-center' : ''}`}
                style={{ width: fitsInitialSet ? undefined : column.width }}
                aria-sort={sort?.key === column.key ? sort.descending ? 'descending' : 'ascending' : 'none'}
              >
                <div className="flex items-center gap-1">
                  <button type="button" className="table-sort" disabled={column.sortable === false} onClick={() => setSort({ key: column.key, descending: sort?.key === column.key && !sort.descending })} title="Sort loaded rows">
                    {column.header}{sort?.key === column.key ? sort.descending ? <ArrowDown size={12} /> : <ArrowUp size={12} /> : null}
                  </button>
                  <TableMenu label={`Filter ${textContent(column.header)}`} icon={<ChevronDown size={12} />} active={Boolean(excluded[column.key]?.length)}>
                    <button type="button" onClick={() => setSort({ key: column.key, descending: false })}>Sort ascending</button>
                    <button type="button" onClick={() => setSort({ key: column.key, descending: true })}>Sort descending</button>
                    <ColumnValues values={[...new Set(values.map((row) => String(row.get(column.key) ?? '—')))].sort(collator.compare)} excluded={excluded[column.key] ?? []} onChange={(next) => { onToggleAll?.(false, rows); setExcluded((current) => ({ ...current, [column.key]: next })) }} />
                    {column.key !== identifier ? <button type="button" onClick={() => visibility.toggle(column.key)}>Hide column</button> : null}
                  </TableMenu>
                </div>
              </th>
            ))}
            <th scope="col" className={`${headerBase} table-column-chooser`}>
              <TableMenu label="Choose visible columns" icon={<Columns3 size={14} />}>
                  {orderedColumns.map((column, index) => <div key={column.key} className="table-column-option">
                    <label><input type="checkbox" checked={column.key === identifier || !visibility.hidden.includes(column.key)} disabled={column.key === identifier} onChange={() => visibility.toggle(column.key)} />{column.header}</label>
                    <button type="button" disabled={index === 0} aria-label={`Move ${textContent(column.header)} left`} title="Move column left" onClick={() => moveColumn(index, -1)}><ArrowLeft size={14} aria-hidden="true" /></button>
                    <button type="button" disabled={index === orderedColumns.length - 1} aria-label={`Move ${textContent(column.header)} right`} title="Move column right" onClick={() => moveColumn(index, 1)}><ArrowRight size={14} aria-hidden="true" /></button>
                  </div>)}
                  <button type="button" onClick={() => columnVisibility?.setHidden ? columnVisibility.setHidden(defaults.hidden) : visibility.reset()}>Reset columns</button>
                  <button type="button" onClick={() => columnVisibility?.resetOrder ? columnVisibility.resetOrder() : setLocalOrder([])}>Reset column order</button>
                <button type="button" onClick={() => { setExcluded({}); setSort(null) }}>Reset column filters</button>
                {visibility.error ? <p role="alert">{visibility.error}</p> : null}
              </TableMenu>
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 && sourceRows.length > 0 ? <tr><td colSpan={columns.length + 1 + (selectable ? 1 : 0)} className="p-3">No loaded rows match the column filters. <button type="button" className="control px-2 text-kp-mauve hover:bg-kp-surface-3" onClick={() => setExcluded({})}>Clear column filters</button></td></tr> : null}
          {virtualized && before > 0 ? <tr aria-hidden="true" role="presentation"><td colSpan={columns.length + 1 + (selectable ? 1 : 0)} style={{ height: before, padding: 0, border: 0 }} /></tr> : null}
          {virtualized ? visibleVirtualRows.map((virtualRow) => renderRow(rows[virtualRow.index], virtualRow.index, true)) : rows.map((row, index) => renderRow(row, index, false))}
          {virtualized && after > 0 ? <tr aria-hidden="true" role="presentation"><td colSpan={columns.length + 1 + (selectable ? 1 : 0)} style={{ height: after, padding: 0, border: 0 }} /></tr> : null}
        </tbody>
        {footer ? (
          <tfoot>
            <tr>
              <td colSpan={columns.length + 1 + (selectable ? 1 : 0)} className="px-3 py-2.5 border-t border-kp-overlay-0">{footer}</td>
            </tr>
          </tfoot>
        ) : null}
      </table>
    </div>
  )
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

function textContent(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textContent).filter(Boolean).join(' ')
  if (isValidElement<{ children?: ReactNode; primary?: ReactNode; secondary?: ReactNode }>(node)) return [node.props.primary, node.props.children, node.props.secondary].map(textContent).filter(Boolean).join(' ')
  return ''
}

function columnValue<T>(column: DataTableColumn<T>, row: T, index: number): string | number | null | undefined {
  if (column.value) return column.value(row)
  const record = row as Record<string, unknown>
  if (column.key === 'age' && typeof record.ageSeconds === 'number') return record.ageSeconds
  const value = record[column.sortKey ?? column.key]
  if (typeof value === 'string' || typeof value === 'number') return value
  return textContent(column.cell(row, index)) || null
}

function ColumnValues({ values, excluded, onChange }: { values: string[]; excluded: string[]; onChange: (values: string[]) => void }) {
  const [search, setSearch] = useState('')
  const matches = values.filter((value) => value.toLocaleLowerCase().includes(search.toLocaleLowerCase()))
  return <>
    <span className="text-content text-kp-overlay-text">Values in loaded rows</span>
    {values.length > 12 ? <input type="search" aria-label="Find column values" placeholder="Find a value" value={search} onChange={(event) => setSearch(event.target.value)} /> : null}
    <div className="flex gap-2"><button type="button" onClick={() => onChange([])}>Show all</button><button type="button" onClick={() => onChange(values)}>Hide all</button></div>
    <div className="table-menu-values">{matches.map((value) => <label key={value} title={value}><input type="checkbox" checked={!excluded.includes(value)} onChange={() => onChange(excluded.includes(value) ? excluded.filter((item) => item !== value) : [...excluded, value])} /><span className="truncate">{value}</span></label>)}</div>
  </>
}
