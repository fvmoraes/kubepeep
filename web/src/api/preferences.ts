import { getPreferences, getSession, putPreferences } from './client'
import type { Preferences, SessionData } from './types'

export interface PreferenceMutationOptions {
  signal?: AbortSignal
  validateSession?: (session: SessionData) => void
}

export type PreferenceMutator = (current: Preferences) => Preferences

// Preferences are persisted as one allowlisted document. Every frontend writer
// shares this queue so each operation reads the latest committed document and
// mutates only the section it owns instead of PUTting a stale render snapshot.
let mutationQueue: Promise<void> = Promise.resolve()

function throwIfAborted(signal?: AbortSignal): void {
  if (!signal?.aborted) return
  if (signal.reason instanceof Error) throw signal.reason
  throw new DOMException('The operation was aborted.', 'AbortError')
}

export function mutatePreferences(mutator: PreferenceMutator, options: PreferenceMutationOptions = {}): Promise<Preferences> {
  const operation = mutationQueue.then(async () => {
    throwIfAborted(options.signal)
    const session = await getSession(options.signal)
    options.validateSession?.(session)
    const current = await getPreferences(options.signal)
    throwIfAborted(options.signal)
    const next = mutator(structuredClone(current))
    return putPreferences(next, session.csrfToken, options.signal)
  })

  // A rejected mutation must not poison later, unrelated writers.
  mutationQueue = operation.then(() => undefined, () => undefined)
  return operation
}
