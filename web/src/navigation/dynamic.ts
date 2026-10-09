import type { DynamicColumn, DynamicResource } from '../api/types'

export function dynamicCollection(resource: DynamicResource): string {
  return `dynamic:${resource.group || '_'}:${resource.version}:${resource.resource}:${resource.namespaced ? 'n' : 'c'}`
}
export function parseDynamicCollection(collection: string): DynamicResource | null {
  const parts = collection.split(':')
  if (parts.length !== 5 || parts[0] !== 'dynamic' || !['n', 'c'].includes(parts[4])) return null
  const [, group, version, resource, scope] = parts
  if (!/^[a-z][a-z0-9-]{0,62}$/.test(version) || !/^[a-z][a-z0-9-]{0,62}$/.test(resource) || (group !== '_' && !/^[a-z0-9][a-z0-9.-]*$/.test(group))) return null
  return { group: group === '_' ? '' : group, version, resource, kind: resource, namespaced: scope === 'n' }
}
export function dynamicResourceSegments(resource: DynamicResource): string {
  return [resource.group || '_', resource.version, resource.resource, resource.namespaced ? 'n' : 'c'].map(encodeURIComponent).join('/')
}
export function dynamicListPath(resource: DynamicResource): string { return `/workloads/custom/${dynamicResourceSegments(resource)}` }

/** Printer order may change when a CRD is updated; keep saved columns tied to their definition. */
export function dynamicColumnKey(column: DynamicColumn): string {
  const identity = `${column.name}\0${column.type}\0${column.format || ''}`
  let hash = 2166136261
  for (let index = 0; index < identity.length; index++) hash = Math.imul(hash ^ identity.charCodeAt(index), 16777619)
  return `printer-${column.name.toLowerCase().replace(/[^a-z0-9-]/g, '-').slice(0, 14)}-${(hash >>> 0).toString(36)}`
}
