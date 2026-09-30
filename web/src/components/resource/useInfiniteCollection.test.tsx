import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'

import { APIError } from '../../api/client'
import type { CollectionResult } from '../../api/types'
import { useInfiniteCollection } from './useInfiniteCollection'

interface Row { name: string }

function page(name: string, next = ''): CollectionResult<Row> {
  return {
    items: [{ name }],
    page: { limit: 100, next, complete: !next, truncated: Boolean(next), filterScope: 'page' },
    coverage: null,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return function Provider({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
}

describe('infinite collection authorization identity', () => {
  it('keeps a same-scope placeholder but drops it before another scope or generation loads', async () => {
    const sameScope = deferred<CollectionResult<Row>>()
    const otherScope = deferred<CollectionResult<Row>>()
    const initialIdentity = ['resources', 'pods', 1, 'ctx', 7, 'gen-1', '']
    const { result, rerender } = renderHook(({ identity, filter }: { identity: readonly unknown[]; filter: string }) => useInfiniteCollection<Row>({
      identity, filters: filter, enabled: true,
      fetchPage: () => filter === 'initial' ? Promise.resolve(page('private')) : filter === 'sort' ? sameScope.promise : otherScope.promise,
    }), { initialProps: { identity: initialIdentity, filter: 'initial' }, wrapper: wrapper() })
    await waitFor(() => expect(result.current.items.map((row) => row.name)).toEqual(['private']))

    rerender({ identity: initialIdentity, filter: 'sort' })
    expect(result.current.items.map((row) => row.name)).toEqual(['private'])
    expect(result.current.query.isPlaceholderData).toBe(true)

    rerender({ identity: ['resources', 'pods', 1, 'ctx', 8, 'gen-2', ''], filter: 'new' })
    expect(result.current.items).toEqual([])
    await act(async () => sameScope.resolve(page('stale-result')))
    expect(result.current.items).toEqual([])
    await act(async () => otherScope.resolve(page('authorized-result')))
    await waitFor(() => expect(result.current.items.map((row) => row.name)).toEqual(['authorized-result']))
  })

  it('hides loaded rows when a later page cannot confirm authorization', async () => {
    const { result } = renderHook(() => useInfiniteCollection<Row>({
      identity: ['resources', 'pods', 1, 'ctx', 7, 'gen-1', ''], filters: 'initial', enabled: true,
      fetchPage: (cursor) => cursor
        ? Promise.reject(new APIError(503, { code: 'AUTHORIZATION_UNAVAILABLE', message: 'Authorization cannot be confirmed.' }))
        : Promise.resolve(page('private', 'next-token')),
    }), { wrapper: wrapper() })
    await waitFor(() => expect(result.current.items.map((row) => row.name)).toEqual(['private']))
    await act(async () => { await result.current.loadNextPage() })
    await waitFor(() => expect(result.current.authorizationFailed).toBe(true))
    expect(result.current.items).toEqual([])
  })
})
