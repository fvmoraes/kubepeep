import { forwardRef, type InputHTMLAttributes } from 'react'

export type InputProps = InputHTMLAttributes<HTMLInputElement>

const base = [
  'h-8 w-full px-2.5',
  'text-kp-text text-base',
  'bg-kp-crust border border-kp-overlay-0 rounded-md',
  'placeholder:text-kp-text-disabled',
  'focus:outline-none focus:border-kp-mauve focus:shadow-focus',
  'hover:not-focus:border-kp-overlay-1',
  'disabled:cursor-not-allowed disabled:opacity-50',
].join(' ')

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ className = '', ...props }, ref) {
  return <input ref={ref} className={`${base} ${className}`} {...props} />
})
