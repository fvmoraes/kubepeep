import type { QueryClient } from '@tanstack/react-query'
import type { SelectionData, StatusData } from './types'

/** Publish the server-confirmed selection without waiting for health/list reads. */
export async function publishSelection(client: QueryClient, selected: SelectionData, expectedGeneration: string | undefined, isCurrent: () => boolean = () => true): Promise<boolean> {
  const canPublish = (current: StatusData | undefined) => !current || (
    current.selection?.generation === expectedGeneration || current.selection?.generation === selected.generation
  )
  // A newer status or a different selection control can finish while this
  // POST response is in transit. Never roll its generation back in the UI.
  if (!isCurrent() || !canPublish(client.getQueryData<StatusData>(['local-status']))) return false
  await client.cancelQueries({ queryKey: ['local-status'], exact: true })
  let published = false
  client.setQueryData<StatusData>(['local-status'], (current) => {
    if (!isCurrent() || !canPublish(current)) return current
    published = true
    if (!current) return undefined
    return {
      ...current,
      selection: selected,
      components: { ...current.components, ...selected.components },
    }
  })
  void client.invalidateQueries({ queryKey: ['local-status'], exact: true })
  return published
}
