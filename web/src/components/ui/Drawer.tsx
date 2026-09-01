import type { MouseEvent, ReactNode } from 'react'
import { useEffect } from 'react'

export interface DrawerProps {
  open: boolean
  onClose: () => void
  title: ReactNode
  children: ReactNode
  className?: string
}

export function Drawer({ open, onClose, title, children, className = '' }: DrawerProps) {
  useEffect(() => {
    if (!open) return
    const originalOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = originalOverflow
    }
  }, [open])

  function handleBackdropClick(event: MouseEvent<HTMLDivElement>) {
    if (event.target === event.currentTarget) {
      onClose()
    }
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      onClose()
    }
  }

  if (!open) return null

  return (
    <div
      role="presentation"
      className="fixed inset-0 z-50"
      onClick={handleBackdropClick}
      onKeyDown={handleKeyDown}
    >
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" aria-hidden="true" />
      <div
        role="dialog"
        aria-modal="true"
        className={`
          absolute top-0 right-0 bottom-0 w-full max-w-xl
          bg-kp-surface-0 border-l border-kp-overlay-0 shadow-dialog
          flex flex-col
          ${className}
        `}
      >
        <header className="flex items-start justify-between gap-4 px-5 py-4 border-b border-kp-overlay-0">
          <div className="min-w-0">{title}</div>
          <button
            type="button"
            onClick={onClose}
            className="flex-0-0-auto w-8 h-8 grid place-items-center rounded-md text-kp-overlay-text hover:text-kp-text hover:bg-kp-surface-2"
            aria-label="Close details"
          >
            ×
          </button>
        </header>
        <div className="flex-1 overflow-auto p-5">{children}</div>
      </div>
    </div>
  )
}
