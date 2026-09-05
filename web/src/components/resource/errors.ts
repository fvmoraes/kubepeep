import { APIError } from '../../api/client'

export function errorMessage(error: unknown): string {
  if (error instanceof APIError) return error.message
  return error instanceof Error ? error.message : 'The local API could not load this resource.'
}

export function errorCode(error: unknown): string | undefined {
  return error instanceof APIError ? error.code : undefined
}
