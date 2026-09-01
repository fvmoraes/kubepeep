import { forwardRef, type SelectHTMLAttributes } from 'react'

export type SelectProps = SelectHTMLAttributes<HTMLSelectElement>

export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select({ className = '', children, ...props }, ref) {
  return (
    <select
      ref={ref}
      className={`
        min-h-9 w-full px-2.5 py-1.5
        text-kp-text text-base
        bg-kp-crust border border-kp-overlay-2 rounded-md
        focus:outline-none focus:border-kp-mauve focus:shadow-focus
        disabled:opacity-60
        ${className}
      `}
      {...props}
    >
      {children}
    </select>
  )
})
