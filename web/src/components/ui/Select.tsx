import { forwardRef, type CSSProperties, type SelectHTMLAttributes } from 'react'

export type SelectProps = SelectHTMLAttributes<HTMLSelectElement>

const base = [
  'h-8 w-full px-2 pr-7',
  'text-kp-text text-base',
  'bg-kp-crust border border-kp-overlay-0 rounded-md',
  'appearance-none bg-no-repeat',
  'focus:outline-none focus:border-kp-mauve focus:shadow-focus',
  'hover:not-focus:border-kp-overlay-1',
  'disabled:cursor-not-allowed disabled:opacity-50',
].join(' ')

const caret: CSSProperties = {
  backgroundImage:
    "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23918a9e' stroke-width='2.5' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")",
  backgroundRepeat: 'no-repeat',
  backgroundPosition: 'right 7px center',
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select({ className = '', style, children, ...props }, ref) {
  return (
    <select ref={ref} className={`${base} ${className}`} style={{ ...caret, ...style }} {...props}>
      {children}
    </select>
  )
})
