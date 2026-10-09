import type { ResourceRef } from '../../api/types'
import { resourceRefForKind } from '../../navigation/paths'

/** Only explicit object references are navigable; selectors and user names are not. */
export function relatedResources(collection: string, value: unknown, namespace?: string | null): ResourceRef[] {
  if (!value || typeof value !== 'object') return []
  const data = value as Record<string, unknown>
  const summary = (data.summary ?? data) as Record<string, unknown>
  const refs: ResourceRef[] = []
  const add = (kind: unknown, name: unknown, ns: unknown = namespace, apiGroup?: unknown) => {
    if (typeof kind !== 'string' || typeof name !== 'string' || !name) return
    const ref = { kind, name, namespace: typeof ns === 'string' ? ns : undefined, ...(typeof apiGroup === 'string' ? { apiGroup } : {}) }
    const target = resourceRefForKind(ref)
    if (target) refs.push({ ...ref, namespace: target.namespace ?? undefined })
  }
  const addRef = (value: unknown) => {
    if (!value || typeof value !== 'object') return
    const ref = value as Record<string, unknown>
    add(ref.kind, ref.name, ref.namespace ?? namespace, ref.apiGroup)
  }
  if (collection === 'hpas') add(data.targetKind, data.targetName)
  if (collection === 'role-bindings' || collection === 'cluster-role-bindings') {
    add(data.roleRefKind, data.roleRefName)
    if (Array.isArray(data.subjects)) data.subjects.forEach(addRef)
  }
  if (collection === 'persistent-volumes' || collection === 'persistent-volume-claims') {
    add('StorageClass', data.storageClass)
    add('PersistentVolume', data.volumeName)
    if (data.claim && typeof data.claim === 'object') {
      const claim = data.claim as Record<string, unknown>
      add('PersistentVolumeClaim', claim.name, claim.namespace)
    }
  }
  if (collection === 'volume-attachments') {
    add('PersistentVolume', data.persistentVolumeName ?? data.volumeName)
    add('Node', data.nodeName)
    add('CSIDriver', data.attacher)
  }
  if (collection === 'csi-nodes') {
    const metadata = data.metadata as Record<string, unknown> | undefined
    add('Node', metadata?.name)
    if (Array.isArray(data.drivers)) data.drivers.forEach((driver) => add('CSIDriver', driver.name))
  }
  if (collection === 'ingresses') {
    add('IngressClass', summary.className)
    if (Array.isArray(summary.paths)) summary.paths.forEach((path) => add('Service', path.backend?.serviceName))
    if (data.defaultBackend && typeof data.defaultBackend === 'object') add('Service', (data.defaultBackend as Record<string, unknown>).serviceName)
  }
  if (collection === 'endpoint-slices' && Array.isArray(summary.endpoints)) {
    summary.endpoints.forEach((endpoint) => { addRef(endpoint.targetRef); add('Node', endpoint.nodeName) })
    const metadata = data.metadata as { labels?: Record<string, string> } | undefined
    add('Service', metadata?.labels?.['kubernetes.io/service-name'])
  }
  if (collection === 'endpoints') {
    add('Service', data.name)
    if (Array.isArray(data.subsets)) data.subsets.forEach((subset) => {
      for (const address of [...(subset.addresses ?? []), ...(subset.notReadyAddresses ?? [])]) { addRef(address.targetRef); add('Node', address.nodeName) }
    })
  }
  if (Array.isArray(data.related)) data.related.forEach(addRef)
  return refs.filter((ref, index) => refs.findIndex((other) => other.kind === ref.kind && other.name === ref.name && other.namespace === ref.namespace) === index)
}
