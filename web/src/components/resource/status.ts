import type { BadgeVariant } from '../ui'

/**
 * Kubernetes status → semantic color.
 * healthy/running/succeeded → green · pending/progressing/suspended/degraded → amber
 * failed/error → red · unknown → gray · informational → blue.
 */
export function statusBadgeVariant(status: string): BadgeVariant {
  switch (status.toLowerCase()) {
    case 'healthy':
    case 'completed':
    case 'running':
    case 'succeeded':
    case 'active':
    case 'bound':
    case 'available':
      return 'healthy'
    case 'progressing':
    case 'suspended':
    case 'pending':
    case 'degraded':
    case 'terminating':
    case 'containercreating':
      return 'warning'
    case 'failed':
    case 'error':
    case 'crashloopbackoff':
    case 'evicted':
    case 'imagepullbackoff':
      return 'danger'
    case 'info':
      return 'info'
    default:
      return 'unknown'
  }
}

export function eventBadgeVariant(type: string): BadgeVariant {
  switch (type.toLowerCase()) {
    case 'normal':
      return 'default'
    case 'warning':
      return 'warning'
    default:
      return 'unknown'
  }
}
