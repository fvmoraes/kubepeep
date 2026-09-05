import type { ReactNode } from 'react'

import { Button } from '../ui'

export interface TableLinkProps {
  'aria-label': string
  onClick: () => void
  primary: ReactNode
  secondary?: ReactNode
}

/** Name cell inside resource tables — ghost button styled as a link. */
export function TableLink({ 'aria-label': label, onClick, primary, secondary }: TableLinkProps) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="-ml-2.5 h-auto justify-start px-2 py-0.5 text-left text-kp-mauve hover:not-disabled:bg-transparent hover:not-disabled:text-kp-mauve-hover"
      aria-label={label}
      onClick={onClick}
    >
      <strong className="block font-medium hover:underline">{primary}</strong>
      {secondary ? <small className="block text-xs text-kp-overlay-text">{secondary}</small> : null}
    </Button>
  )
}
