import { useInfiniteQuery } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef } from 'react'

import { APIError } from '../../api/client'
import type { CollectionResult } from '../../api/types'

export const collectionStaleTime = 30_000
export const collectionGcTime = 120_000

export function useInfiniteCollection<T>(options: {
  identity: readonly unknown[]
  filters: unknown
  enabled: boolean
  fetchPage: (cursor: string, signal: AbortSignal) => Promise<CollectionResult<T>>
}) {
  const queryKey = [...options.identity, options.filters]
  const query = useInfiniteQuery({
    queryKey,
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) => options.fetchPage(pageParam, signal),
    getNextPageParam: (lastPage) => lastPage.page.next || undefined,
    maxPages: 5,
    staleTime: collectionStaleTime,
    gcTime: collectionGcTime,
    placeholderData: (previousData, previousQuery) => {
      const previousKey = previousQuery?.queryKey
      return previousKey?.length === queryKey.length
        && options.identity.every((value, index) => previousKey[index] === value)
        ? previousData : undefined
    },
    enabled: options.enabled,
  })
  const pages = query.data?.pages
  const items = useMemo(() => {
    if (!pages) return [] as T[]
    let firstCurrentPage = 0
    for (let index = 0; index < pages.length; index += 1) {
      if (pages[index].snapshotRenewed) firstCurrentPage = index
    }
    const currentPages = pages.slice(firstCurrentPage)
    // Preserve the transport's exact first-page array: UX timing associates
    // that array with the request and observes its first committed table row.
    return currentPages.length === 1 ? currentPages[0].items : currentPages.flatMap((page) => page.items)
  }, [pages])
  const authorizationFailed = query.error instanceof APIError
    && (query.error.status === 403 || query.error.code === 'GENERATION_CHANGED')
  const identityFingerprint = JSON.stringify(options.identity)
  const idleHandle = useRef<ReturnType<typeof setTimeout> | number | null>(null)
  const fetchNextPage = query.fetchNextPage
  const hasNextPage = query.hasNextPage
  const isFetching = query.isFetching
  const isPlaceholderData = query.isPlaceholderData
  useEffect(() => () => {
    if (idleHandle.current !== null) {
      if (typeof window.cancelIdleCallback === 'function') window.cancelIdleCallback(Number(idleHandle.current))
      else globalThis.clearTimeout(idleHandle.current)
      idleHandle.current = null
    }
  }, [identityFingerprint, options.filters, isFetching, hasNextPage, isPlaceholderData])
  const onScrollProgress = useCallback((fraction: number) => {
    if (fraction < 0.75 || idleHandle.current !== null || !options.enabled || !hasNextPage || isFetching || isPlaceholderData || document.visibilityState === 'hidden') return
    const run = () => {
      idleHandle.current = null
      void fetchNextPage()
    }
    idleHandle.current = typeof window.requestIdleCallback === 'function'
      ? window.requestIdleCallback(run, { timeout: 1_000 })
      : globalThis.setTimeout(run, 100)
  }, [options.enabled, hasNextPage, isFetching, isPlaceholderData, fetchNextPage])

  return {
    query,
    queryKey,
    items: authorizationFailed ? [] as T[] : items,
    lastPage: pages?.at(-1),
    snapshotRenewed: pages?.some((page) => page.snapshotRenewed) ?? false,
    authorizationFailed,
    onScrollProgress,
  }
}
