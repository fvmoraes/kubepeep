import { APIError, getSession } from '../api/client'

/**
 * Fresh CSRF token bound to the active selection generation. Actions must call
 * this immediately before mutating so a stale token can never cross a context
 * switch (the server rotates the nonce with every generation).
 */
export async function csrfForGeneration(generation: string, signal?: AbortSignal): Promise<string> {
  const session = await getSession(signal)
  if (session.generation !== generation) {
    throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed before the action.' })
  }
  return session.csrfToken
}
