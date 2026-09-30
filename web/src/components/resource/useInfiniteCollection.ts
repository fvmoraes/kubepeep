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
  fetchPage: (cursor: string, signal: AbortSignal, prefetch: boolean) => Promise<CollectionResult<T>>
}) {
  const queryKey = [...options.identity, options.filters]
  const prefetchCursorRef = useRef<string | null>(null)
  const prefetchAfterRef = useRef(0)
  const query = useInfiniteQuery({
    queryKey,
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) => options.fetchPage(pageParam, signal, pageParam !== '' && prefetchCursorRef.current === pageParam),
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
    && (query.error.status === 403 || query.error.code === 'GENERATION_CHANGED' || query.error.code === 'AUTHORIZATION_UNAVAILABLE')
  const identityFingerprint = JSON.stringify(options.identity)
  const idleHandle = useRef<ReturnType<typeof setTimeout> | number | null>(null)
  const deferredNearEnd = useRef(false)
  const fetchNextPage = query.fetchNextPage
  const hasNextPage = query.hasNextPage
  const isFetching = query.isFetching
  const isPlaceholderData = query.isPlaceholderData
  const cancelIdle = useCallback(() => {
    if (idleHandle.current !== null) {
      if (typeof window.cancelIdleCallback === 'function') window.cancelIdleCallback(Number(idleHandle.current))
      else globalThis.clearTimeout(idleHandle.current)
      idleHandle.current = null
    }
    prefetchCursorRef.current = null
  }, [])
  useEffect(() => () => {
    cancelIdle()
    deferredNearEnd.current = false
  }, [identityFingerprint, options.filters, cancelIdle])
  useEffect(() => {
    if (isFetching && idleHandle.current !== null) {
      cancelIdle()
      deferredNearEnd.current = true
    }
    if (!hasNextPage || isPlaceholderData) {
      cancelIdle()
      deferredNearEnd.current = false
    }
  }, [isFetching, hasNextPage, isPlaceholderData, cancelIdle])
  const loadNextPage = useCallback(() => {
    cancelIdle()
    deferredNearEnd.current = false
    return fetchNextPage()
  }, [fetchNextPage, cancelIdle])
  const onScrollProgress = useCallback((fraction: number) => {
    const nextCursor = pages?.at(-1)?.page.next
    if (fraction < 0.75) {
      deferredNearEnd.current = false
      return
    }
    if (isFetching) {
      deferredNearEnd.current = true
      return
    }
    if (!nextCursor || idleHandle.current !== null || !options.enabled || !hasNextPage || isPlaceholderData || document.visibilityState === 'hidden' || Date.now() < prefetchAfterRef.current) return
    deferredNearEnd.current = false
    const run = () => {
      idleHandle.current = null
      prefetchCursorRef.current = nextCursor
      void fetchNextPage().then((result) => {
        if (result.error instanceof APIError && result.error.code === 'PREFETCH_DEFERRED') prefetchAfterRef.current = Date.now() + 5_000
      }).finally(() => {
        if (prefetchCursorRef.current === nextCursor) prefetchCursorRef.current = null
      })
    }
    idleHandle.current = typeof window.requestIdleCallback === 'function'
      ? window.requestIdleCallback(run, { timeout: 1_000 })
      : globalThis.setTimeout(run, 100)
  }, [options.enabled, hasNextPage, isFetching, isPlaceholderData, fetchNextPage, pages])
  useEffect(() => {
    if (!isFetching && deferredNearEnd.current && hasNextPage && !isPlaceholderData) {
      deferredNearEnd.current = false
      onScrollProgress(1)
    }
  }, [isFetching, hasNextPage, isPlaceholderData, onScrollProgress])

  return {
    query,
    queryKey,
    items: authorizationFailed ? [] as T[] : items,
    lastPage: pages?.at(-1),
    snapshotRenewed: pages?.some((page) => page.snapshotRenewed) ?? false,
    authorizationFailed,
    nextPageError: query.isFetchNextPageError && !(query.error instanceof APIError && query.error.code === 'PREFETCH_DEFERRED'),
    loadNextPage,
    onScrollProgress,
  }
}
