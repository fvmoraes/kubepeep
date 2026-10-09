import type { Pod } from '../../api/types'

const colors = { running: 'bg-kp-green', problem: 'bg-kp-red', starting: 'bg-kp-yellow', inactive: 'bg-kp-overlay-text' }

export function PodContainerStatus({ pod }: { pod: Pod }) {
  const containers = pod.containers ?? []
  const count = pod.containerCount ?? containers.length
  if (pod.containerCount === undefined && containers.length === 0) return <span className="text-kp-overlay-text">—</span>
  const description = (container: typeof containers[number]) => `${container.name} (${container.type}): ${container.reason || container.state} · ${container.status}`
  return <span className="inline-flex items-center gap-2" aria-label={`${count} containers: ${containers.map(description).join('; ')}`}>
    <span>{count}</span>
    <span className="inline-flex items-center gap-1" aria-hidden="true">{containers.map((container) => <span key={`${container.type}/${container.name}`} className={`h-2 w-2 shrink-0 rounded-full ${colors[container.status]}`} title={description(container)} />)}</span>
    {count > containers.length ? <span className="text-kp-overlay-text" title="Additional container states were omitted by the bounded inventory">+{count - containers.length}</span> : null}
  </span>
}
