import {
  APIError,
  getNamespaceScope,
  getNamespaces,
  getPermissions,
  type Capability,
  type CapabilityError,
  type CapabilityMatrix,
  type SelectionSummary,
} from '../api/client'

// The backend allowlist currently expands to one cluster capability plus 42
// namespace capabilities. Two namespaces therefore produce 85 decisions,
// safely below the public maximum of 100 without duplicating or omitting an ID.
const namespacesPerBatch = 2
const maximumConcurrentBatches = 4

function unique(values: string[]): string[] {
  return [...new Set(values.filter(Boolean))]
}

async function activeNamespaces(selection: SelectionSummary, signal?: AbortSignal): Promise<string[]> {
  if (selection.scopeSource === 'cli') {
    return selection.defaultNamespace ? [selection.defaultNamespace] : []
  }
  if (selection.scopeSource === 'saved' && selection.scopeId !== null) {
    const scope = await getNamespaceScope(selection.scopeId, signal)
    if (scope.mode !== 'all') {
      return unique(scope.namespaces)
    }
  } else if (selection.scopeMode !== 'all') {
    return selection.defaultNamespace ? [selection.defaultNamespace] : []
  }
  const namespaces = await getNamespaces({}, signal)
  return unique(namespaces.filter((namespace) => namespace.selected).map((namespace) => namespace.name))
}

function chunks(values: string[]): string[][] {
  const result: string[][] = []
  for (let index = 0; index < values.length; index += namespacesPerBatch) {
    result.push(values.slice(index, index + namespacesPerBatch))
  }
  return result
}

function capabilityKey(capability: Capability): string {
  return [capability.capabilityId, capability.namespace, capability.resourceName].join('\u0000')
}

function errorKey(error: CapabilityError): string {
  return [error.namespace ?? '', error.code, error.message].join('\u0000')
}

function combine(selection: SelectionSummary, matrices: CapabilityMatrix[]): CapabilityMatrix {
  const decisions = new Map<string, Capability>()
  const errors = new Map<string, CapabilityError>()
  let inconsistent = false

  for (const matrix of matrices) {
    if (matrix.generation !== selection.generation) {
      throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed while permissions were loading.' })
    }
    for (const decision of matrix.decisions) {
      const key = capabilityKey(decision)
      const existing = decisions.get(key)
      if (!existing) {
        decisions.set(key, decision)
        continue
      }
      if (existing.decision !== decision.decision || existing.reasonCode !== decision.reasonCode) {
        inconsistent = true
        decisions.set(key, { ...existing, decision: 'unknown', reasonCode: 'BATCH_INCONSISTENT', expiresAt: null })
      }
    }
    for (const error of matrix.errors) {
      errors.set(errorKey(error), error)
    }
  }
  if (inconsistent) {
    const error = { code: 'AUTHORIZATION_UNAVAILABLE', message: 'A permission changed while the matrix was loading.' }
    errors.set(errorKey(error), error)
  }
  return {
    generation: selection.generation,
    decisions: [...decisions.values()],
    complete: matrices.every((matrix) => matrix.complete) && errors.size === 0,
    truncated: matrices.some((matrix) => matrix.truncated),
    errors: [...errors.values()],
  }
}

export async function getBatchedPermissions(selection: SelectionSummary, refresh: boolean, signal?: AbortSignal): Promise<CapabilityMatrix> {
  const namespaces = await activeNamespaces(selection, signal)
  const batches = chunks(namespaces)
  if (batches.length === 0) {
    return { generation: selection.generation, decisions: [], complete: true, truncated: false, errors: [] }
  }

  const matrices = new Array<CapabilityMatrix>(batches.length)
  const batchController = new AbortController()
  const abortBatches = () => batchController.abort()
  if (signal?.aborted) {
    batchController.abort()
  } else {
    signal?.addEventListener('abort', abortBatches, { once: true })
  }
  let nextBatch = 0
  const worker = async () => {
    while (nextBatch < batches.length) {
      const index = nextBatch++
      matrices[index] = await getPermissions({ namespaces: batches[index], refresh }, batchController.signal)
    }
  }
  try {
    await Promise.all(Array.from({ length: Math.min(maximumConcurrentBatches, batches.length) }, () => worker()))
    return combine(selection, matrices)
  } catch (error) {
    batchController.abort()
    throw error
  } finally {
    signal?.removeEventListener('abort', abortBatches)
  }
}
