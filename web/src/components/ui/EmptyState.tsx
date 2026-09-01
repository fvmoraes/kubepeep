import type { ReactNode } from 'react'

export interface EmptyStateProps {
  title: string
  description?: ReactNode
  icon?: ReactNode
  action?: ReactNode
  className?: string
}

export function EmptyState({ title, description, icon, action, className = '' }: EmptyStateProps) {
  return (
    <div
      className={`
        min-h-22 flex flex-col justify-center gap-1.5
        p-4 border border-dashed border-kp-overlay-2 rounded-md
        text-kp-subtext bg-kp-surface-1
        ${className}
      `}
    >
      {icon ? <div className="text-kp-text">{icon}</div> : null}
      <strong className="text-base text-kp-text">{title}</strong>
      {description ? <span className="text-xs leading-relaxed">{description}</span> : null}
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  )
}
