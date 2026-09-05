import type { ReactNode } from 'react'

import { PageHeader } from '../ui'

export interface ResourcePageProps {
  title: string
  description?: ReactNode
  actions?: ReactNode
  children: ReactNode
  className?: string
}

/** Standard resource page scaffold: header + vertical stack of sections. */
export function ResourcePage({ title, description, actions, children, className = '' }: ResourcePageProps) {
  return (
    <div className={`flex w-full min-w-0 flex-col gap-4 ${className}`}>
      <PageHeader title={title} description={description} actions={actions} />
      {children}
    </div>
  )
}
