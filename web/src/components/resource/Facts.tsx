import type { ReactNode } from 'react'

export interface Fact {
  label: string
  value: ReactNode
}

/** Compact two-column fact grid for resource details. */
export function Facts({ facts, className = '' }: { facts: Fact[]; className?: string }) {
  return (
    <dl className={`grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-kp-overlay-0 bg-kp-overlay-0 ${className}`}>
      {facts.map((fact) => (
        <div key={fact.label} className="min-w-0 bg-kp-surface-1 px-2.5 py-2">
          <dt className="text-2xs uppercase tracking-wider text-kp-overlay-text">{fact.label}</dt>
          <dd className="mt-0.5 break-words text-sm text-kp-subtext leading-snug">{fact.value}</dd>
        </div>
      ))}
    </dl>
  )
}
