import type { EventResource, DynamicResource } from '../api/types'
import { dynamicCollection } from './dynamic'
import { resourceRefForKind, type ResourceRefInput } from './paths'

export function eventTarget(event: EventResource, catalog: DynamicResource[] = []): ResourceRefInput | null {
  const apiGroup = event.objectAPIVersion === undefined ? undefined : event.objectAPIVersion.includes('/') ? event.objectAPIVersion.split('/')[0] : ''
  const namespace = event.objectNamespace ?? event.namespace
  const builtin = resourceRefForKind({ kind: event.objectKind, name: event.objectName, namespace, apiGroup })
  if (builtin) return builtin
  const candidates = catalog.filter(item => item.kind === event.objectKind && (apiGroup === undefined || item.group === apiGroup))
  if (new Set(candidates.map(item => `${item.group}/${item.resource}`)).size !== 1) return null
  const resource = candidates.find(item => `${item.group ? `${item.group}/` : ''}${item.version}` === event.objectAPIVersion) ?? candidates[0]
  if (!resource || !event.objectName || resource.namespaced && !namespace) return null
  return { collection: dynamicCollection(resource), kind: resource.kind, namespace: resource.namespaced ? namespace : null, name: event.objectName }
}
