/** Shared first-use order. Explicit user preferences always take precedence. */
export interface InitialColumn { key: string; initialRole?: string; defaultHidden?: boolean }
const slots = [
  ['namespace'], ['name', 'object'], ['status', 'phase', 'attached'],
  ['ready', 'healthy', 'replicas', 'available'], ['restarts'], ['cpu'], ['memory'],
  ['type', 'kind', 'types', 'class'], ['age', 'created', 'created-at', 'time'],
]
const secondary = ['capacity', 'usage', 'summary', 'target', 'allowed', 'minmax', 'count', 'message', 'rules', 'subjects', 'holder', 'duration', 'renew', 'provisioner', 'binding', 'drivers', 'attach', 'group', 'scope', 'versions', 'value', 'preemption', 'handler', 'chart', 'revision', 'version', 'hosts', 'listeners', 'local', 'remote-port', 'actions', 'items']
export function initialColumns(columns: readonly InitialColumn[], maximumVisible = 9): { order: string[]; hidden: string[] } {
  const role = (column: InitialColumn) => column.initialRole ?? column.key
  const chosen = new Set<string>()
  if (columns.some(column => column.key === 'actions')) chosen.add('actions')
  for (const slot of slots) {
    const column = slot.flatMap(key => columns.filter(column => role(column) === key && !column.defaultHidden))[0]
    if (column) chosen.add(column.key)
  }
  const hasMetrics = columns.some(column => role(column) === 'cpu' || role(column) === 'memory')
  // Inventory types without pod metrics use a few useful, bounded equivalents.
  if (!hasMetrics) for (const key of secondary) {
    if (chosen.size >= (chosen.has('actions') ? 7 : 6)) break
    const column = columns.find(column => role(column) === key && !column.defaultHidden)
    if (column) chosen.add(column.key)
  }
  // CRD printer columns may have arbitrary names; show a small priority-zero set.
  if (chosen.size < 5) for (const column of columns) {
    if (chosen.size >= 5) break
    if (column.key.startsWith('printer-') && !column.defaultHidden) chosen.add(column.key)
  }
  const rank = (column: InitialColumn) => {
    const index = slots.findIndex(slot => slot.includes(role(column)))
    return index < 0 ? 7.5 : index
  }
  const order = [...columns].sort((a, b) => rank(a) - rank(b)).map(column => column.key)
  const visible = order.filter(key => chosen.has(key)).slice(0, maximumVisible)
  return { order, hidden: columns.filter(column => !visible.includes(column.key)).map(column => column.key) }
}
