import type { ReactNode } from 'react'

export interface FieldProps {
  label: ReactNode
  children: ReactNode
  help?: ReactNode
  error?: ReactNode
  className?: string
}

/** Labeled form control with optional help and error text. */
export function Field({ label, children, help, error, className = '' }: FieldProps) {
  return (
    <label className={`grid gap-1 min-w-0 ${className}`}>
      <span className="text-column text-kp-overlay-text uppercase tracking-wide">{label}</span>
      {children}
      {help ? <span className="text-content text-kp-overlay-text leading-snug">{help}</span> : null}
      {error ? <span className="text-content text-kp-red leading-snug" role="alert">{error}</span> : null}
    </label>
  )
}
