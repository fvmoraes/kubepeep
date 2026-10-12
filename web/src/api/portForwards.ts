import { APIError } from './client'
import { mutatePreferences } from './preferences'
import type { PortForward, SavedPortForward } from './types'

export function sameForwardTarget(a: SavedPortForward, b: SavedPortForward) {
  return a.clusterProfileId === b.clusterProfileId && a.context === b.context && a.kind === b.kind && a.namespace === b.namespace && a.name === b.name
}

export function forwardRecipe(session: PortForward): SavedPortForward {
  return { clusterProfileId: session.clusterProfileId, context: session.context, namespace: session.namespace, kind: session.sourceKind ?? 'Pod', name: session.sourceName ?? session.pod, remotePort: session.sourcePort ?? session.remotePort, localPort: session.localPort }
}

export function saveForwardRecipe(recipe: SavedPortForward, generation: string, signal?: AbortSignal, remove = false) {
  return mutatePreferences(current => ({ ...current, portForwards: [
    ...(current.portForwards ?? []).filter(item => !sameForwardTarget(item, recipe)),
    ...(remove ? [] : [recipe]),
  ] }), { signal, validateSession: session => {
    if (session.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
  } })
}

export function portForwardURL(session: PortForward) {
  if (session.status !== 'active' || session.localAddress !== '127.0.0.1' || !Number.isInteger(session.localPort) || session.localPort < 1024 || session.localPort > 65535) return null
  return `${session.sourcePort === 443 || session.remotePort === 443 || session.remotePort === 8443 ? 'https' : 'http'}://127.0.0.1:${session.localPort}`
}
