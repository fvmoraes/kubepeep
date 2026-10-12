import { useQuery } from '@tanstack/react-query'
import { getResourceDiscovery } from '../../api/client'
import type { EventResource } from '../../api/types'
import { eventTarget } from '../../navigation/events'
import { useResourceWorkspace } from '../workspace/ResourceWorkspaceProvider'
import { TableLink } from './TableLink'

export function EventObjectLink({ event, generation }: { event: EventResource; generation?: string }) {
  const workspace = useResourceWorkspace()
  const builtin = eventTarget(event)
  const discovery = useQuery({ queryKey: ['resource-discovery', generation], queryFn: ({ signal }) => getResourceDiscovery(false, signal, generation), enabled: Boolean(generation) && !builtin, staleTime: 300_000, retry: false })
  const target = builtin ?? eventTarget(event, discovery.data?.resources)
  const label = `${event.objectKind}/${event.objectName}`
  return target ? <TableLink primary={label} aria-label={`Open ${event.objectKind} ${event.objectName}`} onClick={() => workspace.openResource(target)} />
    : <span title={discovery.isFetching ? 'Finding this resource…' : 'This object is not available in the resource catalog for this context.'}>{label}</span>
}
