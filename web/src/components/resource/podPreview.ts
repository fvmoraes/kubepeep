import type { QueryClient } from '@tanstack/react-query'

import { getPods } from '../../api/client'
import type { SelectionSummary } from '../../api/types'
import { collectionGcTime, collectionStaleTime } from './useInfiniteCollection'

export function podPreviewKey(selection: SelectionSummary, globalNamespace: string, previewNamespace: string) {
  return ['pod-preview', selection.clusterProfileId, selection.context, selection.scopeId, selection.generation, globalNamespace, previewNamespace] as const
}

// A small authorized namespace read can finish before the full-scope HTTP
// page. It stays a non-selectable preview until that page confirms the rows.
export function prefetchDefaultPodPreview(queryClient: QueryClient, selection: SelectionSummary, globalNamespace: string, previewNamespace: string) {
  return queryClient.prefetchQuery({
    queryKey: podPreviewKey(selection, globalNamespace, previewNamespace),
    queryFn: ({ signal }) => getPods({ limit: 20, skipUXTiming: true, namespaces: [previewNamespace] }, signal, selection.generation),
    staleTime: collectionStaleTime,
    gcTime: collectionGcTime,
  })
}
