import { describe, expect, it } from 'vitest'

import { resourceRefForKind } from './paths'

describe('resource reference identity', () => {
  it.each([
    ['Service', '', 'services'], ['Deployment', 'apps', 'workloads'],
    ['Job', 'batch', 'workloads'], ['HTTPRoute', 'gateway.networking.k8s.io', 'http-routes'],
    ['GatewayClass', 'gateway.networking.k8s.io', 'gateway-classes'], ['ClusterRole', 'rbac.authorization.k8s.io', 'cluster-roles'],
  ])('resolves %s only in its canonical API group', (kind, apiGroup, collection) => {
    expect(resourceRefForKind({ kind, apiGroup, name: 'edge', namespace: 'web' })?.collection).toBe(collection)
    expect(resourceRefForKind({ kind, apiGroup: 'custom.example', name: 'edge', namespace: 'web' })).toBeNull()
  })

  it('preserves legacy references whose API group is omitted', () => {
    expect(resourceRefForKind({ kind: 'Deployment', name: 'api', namespace: 'web' })?.collection).toBe('workloads')
  })
})
