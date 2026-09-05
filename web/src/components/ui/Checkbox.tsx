import type { InputHTMLAttributes, ReactNode } from 'react'

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  children?: ReactNode
}

export function Checkbox({ children, className = '', ...props }: CheckboxProps) {
  return (
    <label className={`inline-flex items-start gap-2 text-sm text-kp-subtext cursor-pointer ${className}`}>
      <input
        type="checkbox"
        className="mt-0.5 h-3.5 w-3.5 shrink-0 accent-kp-mauve cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
        {...props}
      />
      {children}
    </label>
  )
}
