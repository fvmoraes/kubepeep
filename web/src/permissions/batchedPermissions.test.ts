import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Capability, SelectionSummary } from '../api/client'
import { getBatchedPermissions } from './batchedPermissions'

const selection: SelectionSummary = {
  clusterProfileId: 7,
  context: 'development',
  cluster: 'dev-cluster',
  scopeId: 12,
  scopeName: 'Applications',
  scopeMode: 'list',
  scopeSource: 'saved',
  defaultNamespace: 'alpha',
  namespaceCount: 5,
  generation: 'gen_42',
}

function response(data: unknown): Response {
  return new Response(JSON.stringify({ data }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function capability(capabilityId: string, namespace: string, decision: Capability['decision'] = 'allowed'): Capability {
  return {
    capabilityId,
    namespace,
    apiGroup: '',
    resource: namespace ? 'pods' : 'namespaces',
    subresource: '',
    verb: 'list',
    resourceName: '',
    decision,
    reasonCode: decision === 'allowed' ? 'SAR_ALLOWED' : 'SAR_DENIED',
    expiresAt: null,
  }
}

afterEach(() => vi.unstubAllGlobals())

describe('batched permission matrix', () => {
  it('partitions the full namespace scope into safe batches and removes duplicate cluster decisions', async () => {
    const permissionURLs: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/namespace-scopes/12') {
        return Promise.resolve(response({
          id: 12, clusterProfileId: 7, name: 'Applications', context: 'development', mode: 'list',
          namespaces: ['alpha', 'beta', 'gamma', 'delta', 'epsilon'], defaultNamespace: 'alpha', version: 1,
          createdAt: '2026-08-10T12:00:00Z', updatedAt: '2026-08-10T12:00:00Z',
        }))
      }
      if (path.startsWith('/api/v1/permissions?')) {
        permissionURLs.push(path)
        const namespaces = new URL(path, 'http://local').searchParams.getAll('namespace')
        return Promise.resolve(response({
          generation: 'gen_42', complete: true, truncated: false, errors: [],
          decisions: [capability('namespaces.list', ''), ...namespaces.map((namespace) => capability('pods.list', namespace))],
        }))
      }
      throw new Error(`Unexpected request: ${path}`)
    }))

    const matrix = await getBatchedPermissions(selection, false)

    expect(permissionURLs).toHaveLength(3)
    expect(permissionURLs.every((url) => new URL(url, 'http://local').searchParams.getAll('namespace').length <= 2)).toBe(true)
    expect(matrix.decisions.filter((item) => item.capabilityId === 'namespaces.list')).toHaveLength(1)
    expect(matrix.decisions.filter((item) => item.capabilityId === 'pods.list').map((item) => item.namespace)).toEqual(['alpha', 'beta', 'gamma', 'delta', 'epsilon'])
    expect(matrix.complete).toBe(true)
  })

  it('fails closed when a duplicated cluster decision changes between batches', async () => {
    let permissionCall = 0
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/v1/namespace-scopes/12') {
        return Promise.resolve(response({
          id: 12, clusterProfileId: 7, name: 'Applications', context: 'development', mode: 'list',
          namespaces: ['alpha', 'beta', 'gamma'], defaultNamespace: 'alpha', version: 1,
          createdAt: '2026-08-10T12:00:00Z', updatedAt: '2026-08-10T12:00:00Z',
        }))
      }
      const namespaces = new URL(path, 'http://local').searchParams.getAll('namespace')
      const clusterDecision = permissionCall++ === 0 ? 'allowed' : 'denied'
      return Promise.resolve(response({
        generation: 'gen_42', complete: true, truncated: false, errors: [],
        decisions: [capability('namespaces.list', '', clusterDecision), ...namespaces.map((namespace) => capability('pods.list', namespace))],
      }))
    }))

    const matrix = await getBatchedPermissions({ ...selection, namespaceCount: 3 }, false)

    expect(matrix.complete).toBe(false)
    expect(matrix.decisions.find((item) => item.capabilityId === 'namespaces.list')?.decision).toBe('unknown')
    expect(matrix.errors).toContainEqual({ code: 'AUTHORIZATION_UNAVAILABLE', message: 'A permission changed while the matrix was loading.' })
  })

  it('resolves an all scope only from catalog entries marked selected', async () => {
    const paths: string[] = []
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const path = String(input)
      paths.push(path)
      if (path === '/api/v1/namespace-scopes/12') {
        return Promise.resolve(response({
          id: 12, clusterProfileId: 7, name: 'All', context: 'development', mode: 'all', namespaces: [],
          defaultNamespace: null, version: 1, createdAt: '2026-08-10T12:00:00Z', updatedAt: '2026-08-10T12:00:00Z',
        }))
      }
      if (path === '/api/v1/namespaces') {
        return Promise.resolve(response([
          { name: 'alpha', phase: 'Active', selected: true },
          { name: 'outside', phase: 'Active', selected: false },
        ]))
      }
      const namespaces = new URL(path, 'http://local').searchParams.getAll('namespace')
      return Promise.resolve(response({
        generation: 'gen_42', complete: true, truncated: false, errors: [],
        decisions: namespaces.map((namespace) => capability('pods.list', namespace)),
      }))
    }))

    const matrix = await getBatchedPermissions({ ...selection, scopeMode: 'all', scopeName: 'All', namespaceCount: 1 }, false)

    expect(paths).toContain('/api/v1/namespaces')
    expect(paths.some((path) => path.includes('namespace=outside'))).toBe(false)
    expect(matrix.decisions.map((item) => item.namespace)).toEqual(['alpha'])
  })
})
