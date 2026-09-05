import type { ReactNode } from 'react'

import { Card, CardContent } from './ui'

type StateKind = 'loading' | 'empty' | 'error' | 'offline' | 'unavailable'

const dotColor: Record<StateKind, string> = {
  loading: 'bg-kp-sky animate-pulse',
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
  return (
    <section aria-live={kind === 'loading' ? 'polite' : 'assertive'} aria-busy={kind === 'loading'}>
      <Card className="max-w-[760px] p-4">
        <CardContent className="flex items-start gap-3 p-0">
          <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${dotColor[kind]}`} aria-hidden="true" />
          <div className="min-w-0">
            <h2 className="text-lg text-kp-text">{title}</h2>
            <div className="mt-1 text-sm leading-relaxed text-kp-subtext">{children}</div>
            {details ? <div className="mono mt-2 break-words text-xs text-kp-overlay-text">{details}</div> : null}
            {action ? <div className="mt-3">{action}</div> : null}
          </div>
        </CardContent>
      </Card>
    </section>
  )
}
