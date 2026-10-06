import type { ReactNode } from 'react'

import { Button } from '../ui'

export interface TableLinkProps {
  'aria-label': string
  onClick: () => void
  primary: ReactNode
  secondary?: ReactNode
  disabledReason?: string
}

/** Name cell inside resource tables — ghost button styled as a link. */
export function TableLink({ 'aria-label': label, onClick, primary, secondary, disabledReason }: TableLinkProps) {
  return (
    <Button
      type="button"
      variant="ghost"
      className="-ml-2 w-max min-w-0 max-w-[28rem] !whitespace-nowrap justify-start px-2 text-left text-kp-mauve hover:not-disabled:bg-transparent hover:not-disabled:text-kp-mauve-hover"
      aria-label={label}
      onClick={onClick}
      disabled={Boolean(disabledReason)}
      title={disabledReason}
    >
      <span className="min-w-0 text-left inline-flex items-baseline gap-2 whitespace-nowrap">
      <strong className="font-bold hover:underline">{primary}</strong>
      {secondary ? <small className="text-content text-kp-overlay-text">{secondary}</small> : null}
      </span>
    </Button>
  )
}
