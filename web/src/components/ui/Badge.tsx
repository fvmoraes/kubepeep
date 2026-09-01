import type { ReactNode } from 'react'

export type BadgeVariant = 'default' | 'healthy' | 'warning' | 'danger' | 'info' | 'unknown'

export interface BadgeProps {
  variant?: BadgeVariant
  children: ReactNode
  className?: string
}

export function Badge({ variant = 'default', children, className = '' }: BadgeProps) {
  const base =
    'inline-flex items-center justify-center px-2 py-1.5 border rounded-full text-xs whitespace-nowrap'

  const variants: Record<BadgeVariant, string> = {
    default: 'text-kp-subtext border-kp-overlay-0',
    healthy: 'text-kp-green border-kp-green-border',
    warning: 'text-kp-yellow border-kp-yellow-border',
    danger: 'text-kp-red border-kp-red-border',
    info: 'text-kp-sky border-kp-sky-border',
    unknown: 'text-kp-overlay-text border-kp-overlay-0',
  }

  return <span className={`${base} ${variants[variant]} ${className}`}>{children}</span>
}
