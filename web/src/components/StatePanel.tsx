import type { ReactNode } from 'react'

import { Card, CardContent } from './ui'

type StateKind = 'loading' | 'empty' | 'error' | 'offline' | 'unavailable'

interface StatePanelProps {
  kind: StateKind
  title: string
  children: ReactNode
  action?: ReactNode
}

export function StatePanel({ kind, title, children, action }: StatePanelProps) {
  return (
    <section aria-live={kind === 'loading' ? 'polite' : 'assertive'} aria-busy={kind === 'loading'}>
      <Card className="max-w-[760px] min-h-[180px] p-6 grid grid-cols-[12px_1fr] gap-4">
        <span className={`state-dot state-dot--${kind}`} aria-hidden="true" />
        <CardContent className="p-0">
          <h2 className="text-xl text-kp-text mb-2.5">{title}</h2>
          <div className="text-kp-subtext leading-[1.65] text-md">{children}</div>
          {action ? <div className="mt-5">{action}</div> : null}
        </CardContent>
      </Card>
    </section>
  )
}
