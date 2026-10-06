import type { ReactNode } from 'react'

import { Card, CardContent } from './ui'
import { LoadingState } from './ui/LoadingState'

type StateKind = 'loading' | 'empty' | 'error' | 'offline' | 'unavailable'

const dotColor: Record<Exclude<StateKind, 'loading'>, string> = {
  empty: 'bg-kp-overlay-text',
  error: 'bg-kp-red',
  offline: 'bg-kp-red',
  unavailable: 'bg-kp-yellow',
}

interface StatePanelProps {
  kind: StateKind
  title: string
  children: ReactNode
  action?: ReactNode
  details?: ReactNode
}

export function StatePanel({ kind, title, children, action, details }: StatePanelProps) {
  if (kind === 'loading') return <LoadingState label={title} />
  return (
    <section aria-live={kind === 'empty' ? 'polite' : 'assertive'}>
      <Card className="max-w-[760px] p-4">
        <CardContent className="flex items-start gap-3 p-0">
          <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${dotColor[kind]}`} aria-hidden="true" />
          <div className="min-w-0">
            <h2 className="text-heading text-kp-text">{title}</h2>
            <div className="mt-1 text-content leading-relaxed text-kp-subtext">{children}</div>
            {details ? <div className="mono mt-2 break-words text-content text-kp-overlay-text">{details}</div> : null}
            {action ? <div className="mt-3">{action}</div> : null}
          </div>
        </CardContent>
      </Card>
    </section>
  )
}
