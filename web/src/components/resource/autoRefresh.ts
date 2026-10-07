import { APIError } from '../../api/client'

const inventories = new Set(['pods', 'workloads', 'events', 'nodes', 'services', 'endpoints', 'ingresses', 'ingress-classes', 'endpoint-slices', 'network-policies', 'configmaps', 'secrets', 'leases', 'persistent-volumes', 'persistent-volume-claims', 'volume-attachments', 'storage-classes', 'csi-nodes', 'csi-drivers', 'service-accounts', 'resource-quotas', 'limit-ranges', 'hpas', 'pdbs', 'roles', 'role-bindings', 'cluster-roles', 'cluster-role-bindings', 'customresourcedefinitions', 'priority-classes', 'runtime-classes', 'mutating-webhook-configurations', 'validating-webhook-configurations'])

/** A failed authorization review is retryable; only explicit denial or stale identity stops polling. */
export function resourceRefreshInterval(error: unknown): number | false {
  if (error instanceof APIError && (error.status === 401 || error.status === 403 || error.code === 'GENERATION_CHANGED')) return false
  // AUTHORIZATION_UNAVAILABLE is an unknown decision, not a grant. The next
  // bounded read must pass backend authorization before rows can reappear.
  return 15_000
}

/** Refresh only bounded inventories. Detail, log and Secret data reads are explicit. */
export function inventoryRefreshInterval(query: { queryKey: readonly unknown[]; state: { error: unknown; data: unknown } }): number | false {
  if (!inventories.has(String(query.queryKey[1]))) return false
  const data = query.state.data
  return data === undefined || data && typeof data === 'object' && ('page' in data || 'pages' in data) ? resourceRefreshInterval(query.state.error) : false
}
