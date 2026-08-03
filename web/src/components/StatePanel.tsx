import type { ReactNode } from 'react'

type StateKind = 'loading' | 'empty' | 'error' | 'offline' | 'unavailable'

interface StatePanelProps {
  kind: StateKind
  title: string
  children: ReactNode
  action?: ReactNode
}

export function StatePanel({ kind, title, children, action }: StatePanelProps) {
  return (
    <section className="state-panel" aria-live={kind === 'loading' ? 'polite' : 'assertive'} aria-busy={kind === 'loading'}>
      <span className={`state-dot state-dot--${kind}`} aria-hidden="true" />
      <div>
        <h2>{title}</h2>
        <div className="state-copy">{children}</div>
        {action ? <div className="state-action">{action}</div> : null}
      </div>
    </section>
  )
}
