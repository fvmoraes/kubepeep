import { forwardRef, type InputHTMLAttributes } from 'react'

export type InputProps = InputHTMLAttributes<HTMLInputElement>

const base = [
  'control min-w-0 w-full px-2',
  'text-kp-text text-content',
  'bg-kp-crust border border-kp-overlay-0',
  'placeholder:text-kp-overlay-text',
  'focus:outline-none focus:border-kp-mauve focus:shadow-focus',
  'hover:not-focus:border-kp-overlay-1',
  'disabled:cursor-not-allowed disabled:opacity-50',
].join(' ')

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ className = '', ...props }, ref) {
  return <input ref={ref} className={`${base} ${className}`} {...props} />
})
