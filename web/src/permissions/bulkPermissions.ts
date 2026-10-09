import { APIError, getPermissions, type CapabilityMatrix } from '../api/client'

export interface BulkPermissionTarget {
  capabilityId: string
  namespace: string
  name: string
}

/** Named permission checks stay below the API's 20-name/100-decision limits. */
export async function getBulkPermissions(targets: BulkPermissionTarget[], generation: string, signal?: AbortSignal): Promise<CapabilityMatrix> {
  const groups = new Map<string, { namespace: string; capabilityId: string; names: Set<string> }>()
  for (const target of targets) {
    const key = JSON.stringify([target.namespace, target.capabilityId])
    let group = groups.get(key)
    if (!group) {
      group = { namespace: target.namespace, capabilityId: target.capabilityId, names: new Set() }
      groups.set(key, group)
    }
    group.names.add(target.name)
  }
  const batches = Array.from(groups.values()).flatMap((group) => {
    const names = Array.from(group.names)
    return Array.from({ length: Math.ceil(names.length / 20) }, (_, index) => ({
      namespaces: [group.namespace], capabilityIds: [group.capabilityId], resourceNames: names.slice(index * 20, (index + 1) * 20),
    }))
  })
  const matrices = new Array<CapabilityMatrix>(batches.length)
  const controller = new AbortController()
  const abort = () => controller.abort()
  if (signal?.aborted) abort()
  else signal?.addEventListener('abort', abort, { once: true })
  let next = 0
  try {
    await Promise.all(Array.from({ length: Math.min(4, batches.length) }, async () => {
      while (next < batches.length) {
        controller.signal.throwIfAborted()
        const index = next++
        const matrix = await getPermissions(batches[index], controller.signal, generation)
        if (matrix.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active context changed while permissions were loading.' })
        matrices[index] = matrix
      }
    }))
    return { generation, decisions: matrices.flatMap((matrix) => matrix.decisions), complete: matrices.every((matrix) => matrix.complete), truncated: matrices.some((matrix) => matrix.truncated), errors: matrices.flatMap((matrix) => matrix.errors) }
  } catch (error) {
    controller.abort()
    throw error
  } finally {
    signal?.removeEventListener('abort', abort)
  }
}
