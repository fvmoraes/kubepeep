import { describe, expect, it } from 'vitest'
import { initialColumns } from './initialColumns'

describe('first-use inventory columns', () => {
  it('uses the requested pod order and keeps optional details in the chooser', () => {
    const columns = ['namespace', 'name', 'type', 'status', 'ready', 'containers', 'restarts', 'cpu', 'memory', 'node', 'owner', 'ip', 'age'].map(key => ({ key }))
    const initial = initialColumns(columns)
    expect(initial.order.filter(key => !initial.hidden.includes(key))).toEqual(['namespace', 'name', 'status', 'ready', 'restarts', 'cpu', 'memory', 'type', 'age'])
    expect(initial.hidden).toEqual(['containers', 'node', 'owner', 'ip'])
  })
  it.each([
    ['namespace', 'name', 'kind', 'available', 'ready', 'updated', 'status', 'age'],
    ['namespace', 'name', 'target', 'minmax', 'replicas', 'capacity', 'age'],
    ['namespace', 'name', 'uid', 'created'],
    ['name', 'status', 'capacity', 'class', 'claim', 'age'],
    ['namespace', 'name', 'chart', 'revision', 'status', 'age'],
  ])('keeps identity first, age last and a bounded default for %j', (...keys) => {
    const initial = initialColumns(keys.map(key => ({ key })))
    const visible = initial.order.filter(key => !initial.hidden.includes(key))
    expect(visible.slice(0, keys.includes('namespace') ? 2 : 1)).toEqual(keys.includes('namespace') ? ['namespace', 'name'] : ['name'])
    expect(['age', 'created']).toContain(visible.at(-1))
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
})
