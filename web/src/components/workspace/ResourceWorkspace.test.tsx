import { describe, expect, it } from 'vitest'
import { tabsFor } from './ResourceWorkspace'
import { gatewayCollections } from '../../navigation/gateway'

const labels = (collection: string, kind: string | null = null, namespace: string | null = 'payments') =>
  tabsFor({ collection, kind, namespace, name: 'example', tab: 'overview' }).map(tab => tab.label)

describe('detail tab contracts beyond the actionable resource catalog', () => {
  it.each(['helm-releases', 'helm-configmap-releases'])('sorts the specialized %s tabs alphabetically without adding unsupported common tabs', collection => {
    expect(labels(collection)).toEqual(['Overview', 'History', 'Manifest', 'Values'])
  })

  it.each(['DaemonSet', 'Job', 'ReplicaSet'])('preserves the common order and capabilities for %s', kind => {
    expect(labels('workloads', kind)).toEqual(['Overview', 'Logs', 'YAML', 'Metrics', 'Events', 'Data / Env', 'Investigation', 'Pods', 'Actions'])
  })

  it.each(['configmaps', 'secrets'])('keeps YAML before Data in %s without exposing unrelated tabs', collection => {
    expect(labels(collection)).toEqual(['Overview', 'YAML', 'Data'])
  })

  it('keeps Node conditions after the common tabs even without a namespace', () => {
    expect(labels('nodes', 'Node', null)).toEqual(['Overview', 'YAML', 'Events', 'Conditions'])
  })

  it('keeps common tabs in order for all other supported namespaced details', () => {
    const collections = ['endpoints', 'endpoint-slices', 'network-policies', 'leases', 'persistent-volume-claims', 'service-accounts', 'resource-quotas', 'limit-ranges', 'hpas', 'pdbs', 'roles', 'role-bindings']
    for (const collection of collections) {
      expect(labels(collection), collection).toEqual(['Overview', 'YAML', 'Events'])
    }
  })

  it('omits unsupported namespaced details from every other cluster-scoped kind', () => {
    const collections = ['ingress-classes', 'persistent-volumes', 'storage-classes', 'csi-drivers', 'csi-nodes', 'volume-attachments', 'namespaces', 'cluster-roles', 'cluster-role-bindings', 'customresourcedefinitions', 'priority-classes', 'runtime-classes', 'mutating-webhook-configurations', 'validating-webhook-configurations']
    for (const collection of collections) {
      expect(labels(collection, null, null), collection).toEqual(['Overview', 'YAML'])
    }
  })

  it('applies the same common order to every Gateway API kind with its proper scope', () => {
    for (const [collection, kind] of Object.entries(gatewayCollections)) {
      const namespace = collection === 'gateway-classes' ? null : 'payments'
      expect(labels(collection, kind, namespace), kind).toEqual(namespace ? ['Overview', 'YAML', 'Events'] : ['Overview', 'YAML'])
    }
  })

  it.each([
    ['dynamic:example.test:v1:widgets:n', 'payments'],
    ['dynamic:example.test:v1:widgets:c', null],
  ])('keeps custom resource %s details limited to available capabilities', (collection, namespace) => {
    expect(labels(collection!, 'Widget', namespace)).toEqual(['Overview', 'YAML'])
  })
})
