import { forwardRef, type InputHTMLAttributes } from 'react'

export type InputProps = InputHTMLAttributes<HTMLInputElement>

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ className = '', ...props }, ref) {
  return (
    <input
      ref={ref}
      className={`
        min-h-9 w-full px-2.5 py-1.5
        text-kp-text text-base
        bg-kp-crust border border-kp-overlay-2 rounded-md
        placeholder:text-kp-overlay-text
        focus:outline-none focus:border-kp-mauve focus:shadow-focus
        disabled:opacity-60
        ${className}
      `}
      {...props}
    />
  )
})
