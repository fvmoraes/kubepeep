import { describe, expect, it } from 'vitest'

import { sectionChunkFor } from './sectionChunks'

// Each path below was transcribed from App's route table and paired with the
// component that route renders. This is a hand-maintained mirror, not a
// derivation: re-pointing a route at a different lazy module keeps these
// green while the warm targets the wrong chunk. It still earns its place,
// because warming the wrong chunk is otherwise completely silent — the lazy
// boundary imports the right module anyway, so nothing breaks and nothing is
// gained.
describe('sectionChunkFor mirrors the route table', () => {
  it.each([
    ['/', 'Dashboard'],
    ['/events', 'ResourcePages'],
    ['/nodes', 'ResourcePages'],
    ['/nodes/worker-1', 'ResourcePages'],
    ['/pods', 'ResourcePages'],
    ['/pods/web/api-0', 'ResourcePages'],
    ['/workloads', 'ResourcePages'],
    ['/workloads/kind/Deployment', 'ResourcePages'],
    ['/workloads/Deployment/web/api', 'ResourcePages'],
    ['/network', 'ResourcePages'],
    ['/network/services', 'ResourcePages'],
    ['/network/services/web/api', 'ResourcePages'],
    ['/config', 'ResourcePages'],
    ['/config/configmaps/web/settings', 'ResourcePages'],
    ['/leases', 'FamilyPages'],
    ['/leases/kube-system/lock', 'FamilyPages'],
    ['/storage', 'FamilyPages'],
    ['/storage/persistent-volume-claims', 'FamilyPages'],
    ['/storage/persistent-volume-claims/web/data', 'FamilyPages'],
    ['/namespaces/web', 'FamilyPages'],
    ['/namespaces', 'NamespaceScopeEditor'],
    ['/configuration', 'ConfigurationPages'],
    ['/configuration/resource-quotas', 'ConfigurationPages'],
    ['/service-accounts', 'ConfigurationPages'],
    ['/service-accounts/web/deployer', 'ConfigurationPages'],
    ['/access/roles', 'AccessPages'],
    ['/access/roles/web/editor', 'AccessPages'],
    ['/administration', 'AccessPages'],
    ['/administration/priority-classes', 'AccessPages'],
    ['/permissions', 'PermissionsMatrix'],
    ['/logs', 'LogsPage'],
    ['/settings', 'SettingsPage'],
    ['/helm/releases', 'HelmPage'],
    ['/helm/releases/secret/web/app', 'HelmPage'],
    ['/network/gateway-api', 'GatewayPage'],
    ['/network/gateway-api/gateways', 'GatewayPage'],
    ['/custom-resources', 'DynamicResourcePage'],
    ['/custom-resources/example.com/v1/widgets/n', 'DynamicResourcePage'],
    ['/workloads/custom', 'DynamicResourcePage'],
    ['/workloads/custom/example.com/v1/widgets/n', 'DynamicResourcePage'],
  ])('%s warms %s', (pathname, chunk) => {
    expect(sectionChunkFor(pathname)).toBe(chunk)
  })

  // These two live under a generic prefix that would otherwise capture them.
  it('prefers the dynamic resource view over the workload list', () => {
    expect(sectionChunkFor('/workloads/custom')).not.toBe('ResourcePages')
  })

  it('prefers the Gateway page over the network list', () => {
    expect(sectionChunkFor('/network/gateway-api')).not.toBe('ResourcePages')
  })

  it('warms nothing for a path outside the route table', () => {
    expect(sectionChunkFor('/not-a-route')).toBeNull()
  })
})
