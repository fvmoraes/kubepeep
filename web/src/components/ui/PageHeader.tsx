import type { ReactNode } from 'react'

export interface PageHeaderProps {
  title: string
  description?: ReactNode
  actions?: ReactNode
  className?: string
}

/** Standard page heading — title, optional description and right-side actions. */
export function PageHeader({ title, description, actions, className = '' }: PageHeaderProps) {
  return (
    <header className={`flex items-end justify-between gap-4 flex-wrap ${className}`}>
      <div className="min-w-0">
        <h1 className="text-2xl text-kp-text">{title}</h1>
        {description ? <p className="mt-0.5 max-w-2xl text-sm text-kp-overlay-text leading-relaxed">{description}</p> : null}
      </div>
      {actions ? <div className="flex items-center justify-end flex-wrap gap-2">{actions}</div> : null}
    </header>
  )
}
