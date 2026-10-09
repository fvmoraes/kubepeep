import { DataTable, type DataTableProps } from '../ui/DataTable'
import { useSelectionBoundKeys } from './useSelectionBoundKeys'
import { BulkSelectionToolbar } from './BulkSelectionToolbar'

type Props<T> = Omit<DataTableProps<T>, 'selectedKeys' | 'onToggleRow' | 'onToggleAll'> & {
  selectionIdentity: readonly unknown[]
  getRowKey: (row: T, index: number) => string
}

/** Shared inventory selection has no destructive actions. Pods own deletion. */
export function SelectableResourceTable<T>({ selectionIdentity, selectable = true, ...props }: Props<T>) {
  const [keys, setKeys] = useSelectionBoundKeys([...selectionIdentity, selectable])
  const selected = props.rows.flatMap((row, index) => keys.has(props.getRowKey(row, index)) ? [props.getRowKey(row, index)] : [])
  return <>
    {selected.length > 0 && selectable ? <BulkSelectionToolbar names={selected} onClear={() => setKeys(new Set())} /> : null}
    <DataTable {...props} selectable={selectable} selectedKeys={keys}
      onToggleRow={(key, checked) => setKeys((current) => { const next = new Set(current); if (checked) next.add(key); else next.delete(key); return next })}
      onToggleAll={(checked, rows) => setKeys(checked ? new Set(rows.map(props.getRowKey)) : new Set())}
    />
  </>
}
