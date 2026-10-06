import type { ResourceBudget, HPAResourceTarget } from '../../api/types'
import { resourceUsage } from './resourceMetrics'

function formatUsage(value: number, resource: 'cpu' | 'memory') {
  return resource === 'cpu' ? `${Math.round(value * 10) / 10} m` : `${Math.round(value / 1048576 * 10) / 10} MiB`
}

export function ResourceUsage({ current, budget, resource, target }: { current?: number; budget?: ResourceBudget; resource: 'cpu' | 'memory'; target?: HPAResourceTarget }) {
  const { total, percent, threshold, tone, basis } = resourceUsage(current, budget, resource, target)
  const label = resource === 'cpu' ? 'CPU' : 'Memory'
  const text = current === undefined ? 'No sample' : `${formatUsage(current, resource)} / ${total ? formatUsage(total, resource) : 'not set'} · ${percent === null ? '—' : `${percent.toFixed(1)}%`}`
  return <div className="resource-usage" data-tone={tone} title={`${label}: ${text}${total ? ` (${basis}); warning at ${Math.min(threshold, 90)}%, red above 90%` : ''}`}>
    <span className="resource-usage-value">{text}</span>
    <div className="resource-usage-track" role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent === null ? undefined : Math.min(100, Math.round(percent))} aria-valuetext={text}>
      <span style={{ width: `${percent === null ? 0 : Math.min(100, Math.max(0, percent))}%` }} />
    </div>
    {total ? <small className="text-2xs text-kp-overlay-text">{target?.container ? `${target.container} · ` : ''}{basis}{target?.utilization ? ` ${target.utilization}%` : ''}</small> : null}
  </div>
}
