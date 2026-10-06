import { APIError } from '../../api/client'

const inventories = new Set(['pods', 'workloads', 'events', 'nodes', 'services', 'endpoints', 'ingresses', 'ingress-classes', 'endpoint-slices', 'network-policies', 'configmaps', 'secrets', 'leases', 'persistent-volumes', 'persistent-volume-claims', 'volume-attachments', 'storage-classes', 'csi-nodes', 'csi-drivers', 'service-accounts', 'resource-quotas', 'limit-ranges', 'hpas', 'pdbs', 'roles', 'role-bindings', 'cluster-roles', 'cluster-role-bindings', 'customresourcedefinitions', 'priority-classes', 'runtime-classes', 'mutating-webhook-configurations', 'validating-webhook-configurations'])

/** Refresh only bounded inventories. Detail, log and Secret data reads are explicit. */
export function inventoryRefreshInterval(query: { queryKey: readonly unknown[]; state: { error: unknown; data: unknown } }): number | false {
  if (!inventories.has(String(query.queryKey[1]))) return false
  const error = query.state.error
  if (error instanceof APIError && (error.status === 401 || error.status === 403 || error.code === 'GENERATION_CHANGED' || error.code === 'AUTHORIZATION_UNAVAILABLE')) return false
  const data = query.state.data
  return data === undefined || data && typeof data === 'object' && ('page' in data || 'pages' in data) ? 15_000 : false
}
