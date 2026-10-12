import { describe, expect, it } from 'vitest'
import { initialColumns } from './initialColumns'

describe('first-use inventory columns', () => {
  it('uses the requested pod order and keeps optional details in the chooser', () => {
    const columns = ['namespace', 'name', 'type', 'status', 'ready', 'containers', 'restarts', 'cpu', 'memory', 'node', 'owner', 'ip', 'age'].map(key => ({ key }))
    const initial = initialColumns(columns)
    expect(initial.order.filter(key => !initial.hidden.includes(key))).toEqual(['namespace', 'name', 'status', 'ready', 'restarts', 'cpu', 'memory', 'type', 'age'])
    expect(initial.hidden).toEqual(['containers', 'node', 'owner', 'ip'])
    expect(initial.order.slice(9)).toEqual(['containers', 'ip', 'node', 'owner'])
  })
  it.each([
    ['namespace', 'name', 'kind', 'available', 'ready', 'updated', 'status', 'age'],
    ['namespace', 'name', 'target', 'minmax', 'replicas', 'capacity', 'age'],
    ['namespace', 'name', 'uid', 'created'],
    ['name', 'status', 'capacity', 'class', 'claim', 'age'],
    ['namespace', 'name', 'chart', 'revision', 'status', 'age'],
  ])('keeps common columns first and a bounded default for %j', (...keys) => {
    const initial = initialColumns(keys.map(key => ({ key })))
    const visible = initial.order.filter(key => !initial.hidden.includes(key))
    expect(visible.slice(0, keys.includes('namespace') ? 2 : 1)).toEqual(keys.includes('namespace') ? ['namespace', 'name'] : ['name'])
    const extra = visible.filter(key => ['target', 'minmax', 'capacity', 'chart', 'revision'].includes(key))
    const ageIndex = visible.findIndex(key => key === 'age' || key === 'created')
    expect(extra.every(key => visible.indexOf(key) > ageIndex)).toBe(true)
    expect(extra).toEqual([...extra].sort())
    expect(visible.length).toBeLessThanOrEqual(7)
  })
  it('uses CRD printer equivalents and keeps wide columns opt-in', () => {
    const initial = initialColumns([{ key: 'name' }, { key: 'printer-0', initialRole: 'status' }, { key: 'printer-1', initialRole: 'ready' }, { key: 'printer-2', initialRole: 'detail', defaultHidden: true }, { key: 'age' }])
    expect(initial.order.filter(key => !initial.hidden.includes(key))).toEqual(['name', 'printer-0', 'printer-1', 'age'])
    expect(initial.hidden).toEqual(['printer-2'])
  })
  it('keeps identity and status available in a narrow first-use viewport', () => {
    const columns = ['namespace', 'name', 'type', 'status', 'ready', 'containers', 'restarts', 'cpu', 'memory', 'age'].map(key => ({ key }))
    const initial = initialColumns(columns, 3)
    expect(initial.order.filter(key => !initial.hidden.includes(key))).toEqual(['namespace', 'name', 'status'])
    expect(initial.order).toHaveLength(columns.length)
  })
  it('sorts arbitrary printer columns by their displayed labels after common equivalents', () => {
    const columns = [
      { key: 'printer-0', header: 'Zone' },
      { key: 'name', header: 'Name' },
      { key: 'printer-1', header: 'address' },
      { key: 'age', header: 'Age' },
      { key: 'printer-2', header: 'Status', initialRole: 'status' },
      { key: 'printer-3', header: 'Image' },
    ]
    expect(initialColumns(columns, 9, column => column.header).order)
      .toEqual(['name', 'printer-2', 'age', 'printer-1', 'printer-3', 'printer-0'])
  })
})
