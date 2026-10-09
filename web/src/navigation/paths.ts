// Canonical routing for every resource detail and list. Single source of
// truth shared by tables, the command palette, favorites, recents and the
// Resource Workspace so no navigation target can 404 again.

import { gatewayCollections } from './gateway'

export const clusterScopedCollections = new Set([
  'gateway-classes',
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
  const dynamic = parseDynamicCollection(collection)
  if (dynamic) return !dynamic.namespaced
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
 'helm-releases': '/helm/releases/secrets',
 'helm-configmap-releases': '/helm/releases/configmaps',
 ...Object.fromEntries(Object.keys(gatewayCollections).map((key) => [key, `/network/gateway-api/${key}`])),
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
  const dynamic = parseDynamicCollection(reference.collection)
  if (dynamic) return `${dynamicListPath(dynamic)}/${encodeURIComponent(reference.namespace || '_')}/${encodeURIComponent(reference.name)}`
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
  const dynamic = parseDynamicCollection(reference.collection)
  if (dynamic) return dynamicListPath(dynamic)
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
    'helm-releases': 'HelmRelease',
    'helm-configmap-releases': 'HelmRelease',
    ...gatewayCollections,
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

/** Resolve references with the same catalog used by resource routes. */
export function resourceRefForKind(ref: { kind: string; name: string; namespace?: string | null; apiGroup?: string | null }): ResourceRefInput | null {
  const workload = workloadKindPath(ref.kind)
  const collection = workload ? 'workloads' : Object.keys(collectionDetailRoots).find((collection) => resourceKindLabel({ collection, name: ref.name }) === ref.kind)
  if (!collection || !ref.name) return null
  // Custom resources can reuse built-in kind names. Never send a reference
  // from another group to an unrelated built-in object's detail or editor.
  if (ref.apiGroup != null) {
    const groups: Record<string, string> = {
      ...Object.fromEntries(Object.keys(gatewayCollections).map((key) => [key, 'gateway.networking.k8s.io'])),
      workloads: ref.kind === 'Job' || ref.kind === 'CronJob' ? 'batch' : 'apps',
      ingresses: 'networking.k8s.io', 'ingress-classes': 'networking.k8s.io', 'network-policies': 'networking.k8s.io',
      'endpoint-slices': 'discovery.k8s.io', leases: 'coordination.k8s.io',
      'storage-classes': 'storage.k8s.io', 'csi-drivers': 'storage.k8s.io', 'csi-nodes': 'storage.k8s.io', 'volume-attachments': 'storage.k8s.io',
      roles: 'rbac.authorization.k8s.io', 'role-bindings': 'rbac.authorization.k8s.io', 'cluster-roles': 'rbac.authorization.k8s.io', 'cluster-role-bindings': 'rbac.authorization.k8s.io',
      hpas: 'autoscaling', pdbs: 'policy', customresourcedefinitions: 'apiextensions.k8s.io',
      'priority-classes': 'scheduling.k8s.io', 'runtime-classes': 'node.k8s.io',
      'mutating-webhook-configurations': 'admissionregistration.k8s.io', 'validating-webhook-configurations': 'admissionregistration.k8s.io',
    }
    if (ref.apiGroup !== (groups[collection] ?? '')) return null
  }
  const target = { collection, kind: workload ? ref.kind : null, namespace: isClusterScopedCollection(collection) ? null : ref.namespace ?? null, name: ref.name }
  return resourceDetailPath(target) ? target : null
}
import { dynamicListPath, parseDynamicCollection } from './dynamic'
