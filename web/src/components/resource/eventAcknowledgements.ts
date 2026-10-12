import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { EventResource, SelectionSummary } from '../../api/types'

export function eventOccurrence(event: EventResource): string {
  return JSON.stringify([event.uid ?? event.name, event.namespace, event.objectAPIVersion, event.objectKind, event.objectName, event.reason, event.type, event.source, event.timestamp, event.count])
}

/** Session-only and bounded: a new occurrence remains visible, even for the same Event. */
export function useEventAcknowledgements(selection: SelectionSummary | null) {
  const client = useQueryClient()
  const key = ['event-acknowledgements', selection?.clusterProfileId, selection?.context, selection?.cluster]
  const { data = [] } = useQuery<string[]>({ queryKey: key, queryFn: () => [], initialData: [], enabled: false, staleTime: Infinity, gcTime: Infinity })
  const acknowledged = new Set(data)
  return {
    isAcknowledged: (event: EventResource) => acknowledged.has(eventOccurrence(event)),
    acknowledge: (events: EventResource[]) => client.setQueryData<string[]>(key, current => [...new Set([...(current ?? []), ...events.map(eventOccurrence)])].slice(-2000)),
    restore: () => client.setQueryData(key, []),
  }
}
