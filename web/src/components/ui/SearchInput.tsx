import { Search, X } from 'lucide-react'
import { forwardRef, type InputHTMLAttributes } from 'react'

import { Input } from './Input'

export type SearchInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type'>

/** Search field with leading icon and clear affordance. */
export const SearchInput = forwardRef<HTMLInputElement, SearchInputProps>(function SearchInput(
  { className = '', value, onChange, ...props },
  ref,
) {
  return (
    <div className={`relative min-w-0 ${className}`}>
      <Search size={14} aria-hidden="true" className="absolute left-2.5 top-1/2 -translate-y-1/2 text-kp-text-disabled pointer-events-none" />
      <Input type="search" className="pl-8 pr-7" value={value} onChange={onChange} ref={ref} {...props} />
      {typeof value === 'string' && value !== '' ? (
        <button
          type="button"
          aria-label="Clear search"
          className="absolute right-1.5 top-1/2 -translate-y-1/2 grid place-items-center h-5 w-5 rounded text-kp-overlay-text hover:text-kp-text hover:bg-kp-surface-3"
          onClick={(event) => {
            const input = event.currentTarget.parentElement?.querySelector('input')
            if (input) {
              const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
              setter?.call(input, '')
              input.dispatchEvent(new Event('input', { bubbles: true }))
              input.focus()
            }
          }}
        >
          <X size={12} aria-hidden="true" />
        </button>
      ) : null}
    </div>
  )
})
