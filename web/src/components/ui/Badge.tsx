import type { ReactNode } from 'react'

export type BadgeVariant = 'default' | 'healthy' | 'warning' | 'danger' | 'info' | 'unknown'

export interface BadgeProps {
  variant?: BadgeVariant
  children: ReactNode
  className?: string
}

const variants: Record<BadgeVariant, string> = {
  default: 'text-kp-subtext bg-kp-surface-3 border-kp-overlay-1',
  healthy: 'text-kp-green bg-kp-green-bg border-kp-green-border',
  warning: 'text-kp-yellow bg-kp-yellow-bg border-kp-yellow-border',
  danger: 'text-kp-red bg-kp-red-bg border-kp-red-border',
  info: 'text-kp-sky bg-kp-blue-bg border-kp-blue-border',
  unknown: 'text-kp-overlay-text bg-kp-surface-2 border-kp-overlay-0',
}

export function Badge({ variant = 'default', children, className = '' }: BadgeProps) {
  return (
    <span className={`inline-flex items-center justify-center gap-1 px-1.5 py-0.5 border rounded-full text-xs whitespace-nowrap ${variants[variant]} ${className}`}>
      {children}
    </span>
  )
}

const statusDot: Record<BadgeVariant, string> = {
  default: 'bg-kp-overlay-text',
  healthy: 'bg-kp-green',
  warning: 'bg-kp-yellow',
  danger: 'bg-kp-red',
  info: 'bg-kp-sky',
  unknown: 'bg-kp-overlay-text',
}

/** Badge with a leading status dot — used for Kubernetes resource states. */
export function StatusBadge({ variant, children, className = '' }: BadgeProps & { variant: BadgeVariant }) {
  return (
    <Badge variant={variant} className={className}>
      <span aria-hidden="true" className={`h-1.5 w-1.5 rounded-full ${statusDot[variant]}`} />
      {children}
    </Badge>
  )
}
