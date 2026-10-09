import { describe, expect, it } from 'vitest'
import { dynamicCollection, dynamicColumnKey, parseDynamicCollection } from './dynamic'

describe('dynamic resource identities', () => {
  it('keeps the core API distinct from a group named core', () => {
    const core = { group: '', version: 'v1', resource: 'pods', kind: 'Pod', namespaced: true }
    expect(dynamicCollection(core)).not.toBe(dynamicCollection({ ...core, group: 'core' }))
    expect(parseDynamicCollection(dynamicCollection(core))).toMatchObject({ group: '', resource: 'pods', namespaced: true })
  })
  it('retains a printer column identity across schema reorderings', () => {
    const columns = [{ name: 'Ready', type: 'string' }, { name: 'Replicas', type: 'integer' }]
    const saved = columns.map(dynamicColumnKey)
    expect([...columns].reverse().map(dynamicColumnKey)).toEqual([...saved].reverse())
    expect(dynamicColumnKey({ name: 'Ready', type: 'boolean' })).not.toBe(saved[0])
    expect(dynamicColumnKey({ name: 'Extremely long printer column name', type: 'string' })).toMatch(/^printer-[a-z0-9-]{1,14}-[a-z0-9]+$/)
  })
})
