import type { ReactNode } from 'react'

export interface EmptyStateProps {
  title: string
  description?: ReactNode
  icon?: ReactNode
  action?: ReactNode
  className?: string
}

/** Compact empty state — no oversized box, the message is the focus. */
export function EmptyState({ title, description, icon, action, className = '' }: EmptyStateProps) {
  return (
    <div className={`flex flex-col items-center justify-center gap-1.5 py-10 px-4 text-center ${className}`}>
      {icon ? <div className="mb-1 text-kp-text-disabled" aria-hidden="true">{icon}</div> : null}
      <strong className="text-lg text-kp-text">{title}</strong>
      {description ? <span className="max-w-md text-sm text-kp-overlay-text leading-relaxed">{description}</span> : null}
      {action ? <div className="mt-3">{action}</div> : null}
    </div>
  )
}
