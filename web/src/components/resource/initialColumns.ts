/** Shared first-use order. Explicit user preferences always take precedence. */
export interface InitialColumn { key: string; initialRole?: string; defaultHidden?: boolean }
const slots = [
  ['namespace'], ['name', 'object'], ['status', 'phase', 'attached'],
  ['ready', 'healthy', 'replicas', 'available'], ['restarts'], ['cpu'], ['memory'],
  ['type', 'kind', 'types', 'class'], ['age', 'created', 'created-at', 'time'],
]
const secondary = ['capacity', 'usage', 'summary', 'target', 'allowed', 'minmax', 'count', 'message', 'rules', 'subjects', 'holder', 'duration', 'renew', 'provisioner', 'binding', 'drivers', 'attach', 'group', 'scope', 'versions', 'value', 'preemption', 'handler', 'chart', 'revision', 'version', 'hosts', 'listeners', 'local', 'remote-port', 'actions', 'items']
const labelCollator = new Intl.Collator('en', { numeric: true, sensitivity: 'base' })
export function initialColumns<T extends InitialColumn>(columns: readonly T[], maximumVisible = 9, label: (column: T) => string = column => column.key): { order: string[]; hidden: string[] } {
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
    return index < 0 ? slots.length : index
  }
  const order = [...columns].sort((a, b) => rank(a) - rank(b)
    || (rank(a) === slots.length ? labelCollator.compare(label(a), label(b)) : 0)).map(column => column.key)
  // Keep the existing compact selection of useful fields. Alphabetical display
  // order must not hide actions or change which fields fit the initial viewport.
  const visibilityRank = (column: InitialColumn) => rank(column) === slots.length ? slots.length - 1.5 : rank(column)
  const visible = columns.filter(column => chosen.has(column.key))
    .sort((a, b) => visibilityRank(a) - visibilityRank(b)).slice(0, maximumVisible).map(column => column.key)
  return { order, hidden: columns.filter(column => !visible.includes(column.key)).map(column => column.key) }
}
