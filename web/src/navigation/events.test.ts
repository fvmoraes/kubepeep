import { expect, it } from 'vitest'
import type { EventResource, DynamicResource } from '../api/types'
import { eventTarget } from './events'

const event = { namespace: 'events', objectNamespace: 'payments', objectKind: 'Pod', objectName: 'api', objectAPIVersion: 'v1' } as EventResource
const sql: DynamicResource = { group: 'sql.example.com', version: 'v1', kind: 'SQLInstance', resource: 'sqlinstances', namespaced: true, shortNames: [] }
it('resolves built-in events using the involved namespace and respects API groups', () => {
  expect(eventTarget(event)).toMatchObject({ collection: 'pods', namespace: 'payments', name: 'api' })
  expect(eventTarget({ ...event, objectKind: 'Node' })).toMatchObject({ collection: 'nodes', namespace: null })
  expect(eventTarget({ ...event, objectAPIVersion: 'custom.example.com/v1' })).toBeNull()
})
it('resolves custom objects without pinning a view and refuses ambiguous kinds', () => {
  const custom = { ...event, objectKind: sql.kind, objectAPIVersion: 'sql.example.com/v1' }
  expect(eventTarget(custom, [sql])).toMatchObject({ collection: 'dynamic:sql.example.com:v1:sqlinstances:n', namespace: 'payments' })
  expect(eventTarget({ ...custom, objectAPIVersion: undefined }, [sql, { ...sql, group: 'other.example.com' }])).toBeNull()
  expect(eventTarget(custom, [])).toBeNull()
})
