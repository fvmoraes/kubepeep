// Canonical routing for every resource detail and list. Single source of
// truth shared by tables, the command palette, favorites, recents and the
// Resource Workspace so no navigation target can 404 again.

export const clusterScopedCollections = new Set([
  'nodes',
  'persistent-volumes',
  'storage-classes',
  'csi-drivers',
  'csi-nodes',
  'volume-attachments',
  'namespaces',
  'cluster-roles',
  'cluster-role-bindings',
  'customresourcedefinitions',
  'priority-classes',
  'runtime-classes',
  'mutating-webhook-configurations',
  'validating-webhook-configurations',
  'ingress-classes',
])

export function isClusterScopedCollection(collection: string): boolean {
  return clusterScopedCollections.has(collection)
}

const workloadKindPaths: Record<string, string> = {
  Deployment: 'deployments',
  StatefulSet: 'statefulsets',
  DaemonSet: 'daemonsets',
  Job: 'jobs',
  CronJob: 'cronjobs',
  ReplicaSet: 'replicasets',
}

export function workloadKindPath(kind: string): string | null {
  return workloadKindPaths[kind] ?? null
}

const collectionDetailRoots: Record<string, string> = {
  pods: '/pods',
  services: '/network/services',
  ingresses: '/network/ingresses',
  'endpoint-slices': '/network/endpoint-slices',
  endpoints: '/network/endpoints',
  'network-policies': '/network/network-policies',
  'ingress-classes': '/network/ingress-classes',
  configmaps: '/config/configmaps',
  secrets: '/config/secrets',
  leases: '/leases',
  'persistent-volume-claims': '/storage/persistent-volume-claims',
  'persistent-volumes': '/storage/persistent-volumes',
  'storage-classes': '/storage/storage-classes',
  'csi-drivers': '/storage/csi-drivers',
  'csi-nodes': '/storage/csi-nodes',
  'volume-attachments': '/storage/volume-attachments',
  'service-accounts': '/service-accounts',
  'resource-quotas': '/configuration/resource-quotas',
  'limit-ranges': '/configuration/limit-ranges',
  hpas: '/configuration/hpas',
  pdbs: '/configuration/pdbs',
  roles: '/access/roles',
  'role-bindings': '/access/role-bindings',
  'cluster-roles': '/access/cluster-roles',
  'cluster-role-bindings': '/access/cluster-role-bindings',
  customresourcedefinitions: '/administration/customresourcedefinitions',
  'priority-classes': '/administration/priority-classes',
  'runtime-classes': '/administration/runtime-classes',
  'mutating-webhook-configurations': '/administration/mutating-webhook-configurations',
  'validating-webhook-configurations': '/administration/validating-webhook-configurations',
  nodes: '/nodes',
  namespaces: '/namespaces',
}

export interface ResourceRefInput {
  collection: string
  kind?: string | null
  namespace?: string | null
  name: string
}

/** Full detail path for a resource, or null when the collection is unknown. */
export function resourceDetailPath(reference: ResourceRefInput): string | null {
  const name = encodeURIComponent(reference.name)
  const namespace = reference.namespace ? encodeURIComponent(reference.namespace) : null
  if (reference.collection === 'workloads') {
    const kindPath = reference.kind ? workloadKindPath(reference.kind) : null
    if (!kindPath || !namespace) return null
    return `/workloads/${kindPath}/${namespace}/${name}`
  }
  const root = collectionDetailRoots[reference.collection]
  if (!root) return null
  if (isClusterScopedCollection(reference.collection)) return `${root}/${name}`
  if (!namespace) return null
  return `${root}/${namespace}/${name}`
}

/** List path that hosts the collection (used when closing the workspace). */
export function collectionListPath(reference: ResourceRefInput): string | null {
  if (reference.collection === 'workloads') {
    // Prefer the kind tab when known; the general workloads page otherwise.
    const kindPath = reference.kind ? workloadKindPath(reference.kind) : null
    return kindPath ? `/workloads/kind/${kindPath}` : '/workloads'
  }
  const root = collectionDetailRoots[reference.collection]
  return root ?? null
}

/** Stable key for history entries and React lists. */
export function resourceKey(reference: ResourceRefInput): string {
  return `${reference.collection}|${reference.kind ?? ''}|${reference.namespace ?? ''}|${reference.name}`
}

/** Human kind label for a collection/kind pair (e.g. "Deployment", "Pod"). */
export function resourceKindLabel(reference: ResourceRefInput): string {
  if (reference.kind) return reference.kind
  const labels: Record<string, string> = {
    pods: 'Pod',
    services: 'Service',
    ingresses: 'Ingress',
    'endpoint-slices': 'EndpointSlice',
    endpoints: 'Endpoints',
    'network-policies': 'NetworkPolicy',
    'ingress-classes': 'IngressClass',
    configmaps: 'ConfigMap',
    secrets: 'Secret',
    leases: 'Lease',
    'persistent-volume-claims': 'PersistentVolumeClaim',
    'persistent-volumes': 'PersistentVolume',
    'storage-classes': 'StorageClass',
    'csi-drivers': 'CSIDriver',
    'csi-nodes': 'CSINode',
    'volume-attachments': 'VolumeAttachment',
    'service-accounts': 'ServiceAccount',
    'resource-quotas': 'ResourceQuota',
    'limit-ranges': 'LimitRange',
    hpas: 'HorizontalPodAutoscaler',
    pdbs: 'PodDisruptionBudget',
    roles: 'Role',
    'role-bindings': 'RoleBinding',
    'cluster-roles': 'ClusterRole',
    'cluster-role-bindings': 'ClusterRoleBinding',
    customresourcedefinitions: 'CustomResourceDefinition',
    'priority-classes': 'PriorityClass',
    'runtime-classes': 'RuntimeClass',
    'mutating-webhook-configurations': 'MutatingWebhookConfiguration',
    'validating-webhook-configurations': 'ValidatingWebhookConfiguration',
    nodes: 'Node',
    namespaces: 'Namespace',
  }
  return labels[reference.collection] ?? reference.collection
}
