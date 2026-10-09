import { expect, it } from 'vitest'
import { relatedResources } from './relatedResources'
import { resourceRefForKind } from '../../navigation/paths'

it.each(['HorizontalPodAutoscaler', 'PodDisruptionBudget', 'IngressClass', 'NetworkPolicy', 'ResourceQuota', 'LimitRange', 'Lease', 'CSIDriver', 'CSINode', 'VolumeAttachment', 'Namespace', 'CustomResourceDefinition', 'RuntimeClass'])('resolves %s through the canonical routing catalog', (kind) => {
  expect(resourceRefForKind({ kind, name: 'sample', namespace: 'team' })).not.toBeNull()
})
it('strips namespaces from cluster objects and does not turn users into resource links', () => {
  expect(relatedResources('role-bindings', { roleRefKind: 'ClusterRole', roleRefName: 'reader', subjects: [{ kind: 'ServiceAccount', name: 'account', namespace: 'other' }, { kind: 'User', name: 'person' }] }, 'team')).toEqual([
    { kind: 'ClusterRole', name: 'reader', namespace: undefined }, { kind: 'ServiceAccount', name: 'account', namespace: 'other' },
  ])
  expect(relatedResources('hpas', { targetKind: 'Deployment', targetName: 'api' }, 'team')).toEqual([{ kind: 'Deployment', name: 'api', namespace: 'team' }])
})

it('does not link custom resources sharing a built-in kind name to the wrong object', () => {
  expect(relatedResources('http-routes', { related: [
    { kind: 'Service', apiGroup: 'custom.example', name: 'api', namespace: 'web' },
    { kind: 'Gateway', apiGroup: 'gateway.networking.k8s.io', name: 'edge', namespace: 'infra' },
  ] }, 'web')).toEqual([{ kind: 'Gateway', apiGroup: 'gateway.networking.k8s.io', name: 'edge', namespace: 'infra' }])
})
