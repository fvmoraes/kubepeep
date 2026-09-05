import type { ReactNode } from 'react'

export interface FieldProps {
  label: ReactNode
  children: ReactNode
  help?: ReactNode
  error?: ReactNode
  className?: string
  compact?: boolean
}

/** Labeled form control with optional help and error text. */
export function Field({ label, children, help, error, className = '', compact = false }: FieldProps) {
  return (
    <label className={`grid gap-1 min-w-0 ${className}`}>
      <span className={`${compact ? 'text-2xs' : 'text-xs'} text-kp-overlay-text uppercase tracking-wide`}>{label}</span>
      {children}
      {help ? <span className="text-xs text-kp-overlay-text leading-snug">{help}</span> : null}
      {error ? <span className="text-xs text-kp-red leading-snug" role="alert">{error}</span> : null}
    </label>
  )
}
