import { useEffect, useState, type ReactNode } from 'react'
import { TriangleAlert } from 'lucide-react'

import { Button, Checkbox } from './index'

export interface ConfirmAction {
  id: string
  label: string
}

interface ConfirmDialogProps {
  open: boolean
  severity?: 'danger' | 'warning'
  title: string
  description?: ReactNode
  resources?: Array<{ namespace?: string; kind?: string; name: string }>
  consequenceNote?: string
  confirmLabel?: string
  cancelLabel?: string
  pendingLabel?: string
  pending?: boolean
  /** Require typing the single resource name for especially dangerous operations. */
  requireTypingName?: string
  onConfirm: () => void
  onCancel: () => void
}

/**
 * Destructive-action confirmation. The user must tick the explicit
 * acknowledgement (and optionally type the resource name) before the confirm
 * button enables — mirrors the backend `confirmed: true` contract.
 */
export function ConfirmDialog(props: ConfirmDialogProps) {
  if (!props.open) return null
  return <ConfirmDialogBody {...props} />
}

function ConfirmDialogBody({
  severity = 'danger',
  title,
  description,
  resources = [],
  consequenceNote,
  confirmLabel,
  cancelLabel = 'Cancel',
  pendingLabel = 'Working…',
  pending = false,
  requireTypingName,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const [acknowledged, setAcknowledged] = useState(false)
  const [typed, setTyped] = useState('')

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') onCancel()
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [onCancel])

  const typingSatisfied = !requireTypingName || typed.trim() === requireTypingName
  const canConfirm = acknowledged && typingSatisfied && !pending
  const accent = severity === 'danger'
    ? { border: 'border-kp-red-border', bg: 'bg-kp-red-bg/40', text: 'text-kp-red', button: 'danger' as const }
    : { border: 'border-kp-yellow-border', bg: 'bg-kp-yellow-bg/40', text: 'text-kp-yellow', button: 'warning' as const }
  const defaultConfirm = severity === 'danger' ? 'Confirm' : 'Continue'

  return (
    <div className="fixed inset-0 z-[var(--z-confirm)] grid place-items-center p-4" role="presentation">
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" aria-hidden="true" onMouseDown={(event) => { if (event.target === event.currentTarget) onCancel() }} />
      <div role="alertdialog" aria-modal="true" aria-labelledby="confirm-dialog-title" className={`relative w-full max-w-lg rounded-xl border ${accent.border} bg-kp-surface-0 p-4 shadow-dialog`}>
        <div className="flex items-start gap-2.5">
          <TriangleAlert size={18} strokeWidth={1.8} className={`mt-0.5 shrink-0 ${accent.text}`} aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <h2 id="confirm-dialog-title" className="m-0 text-base text-kp-text">{title}</h2>
            {description ? <div className="mt-1 text-sm leading-relaxed text-kp-subtext">{description}</div> : null}
          </div>
        </div>
        {resources.length > 0 ? (
          <ul className="m-0 mt-3 grid max-h-40 list-none gap-1 overflow-auto rounded-lg border border-kp-overlay-0 bg-kp-crust p-2.5">
            {resources.map((resource) => (
              <li key={`${resource.namespace ?? ''}/${resource.kind ?? ''}/${resource.name}`} className="mono text-xs text-kp-subtext">
                {resource.kind ? `${resource.kind} ` : ''}{resource.name}
                {resource.namespace ? <span className="text-kp-overlay-text"> · ns {resource.namespace}</span> : null}
              </li>
            ))}
          </ul>
        ) : null}
        {consequenceNote ? (
          <p className={`mt-3 rounded-r-md border-l-2 ${severity === 'danger' ? 'border-kp-red-border bg-kp-red-bg/40' : 'border-kp-yellow-border bg-kp-yellow-bg/40'} px-3 py-2 text-xs text-kp-subtext`} role="note">
            {consequenceNote}
          </p>
        ) : null}
        {requireTypingName ? (
          <label className="mt-3 grid gap-1">
            <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Type <strong className="mono text-kp-subtext">{requireTypingName}</strong> to confirm</span>
            <input
              type="text"
              className="h-8 rounded-md border border-kp-overlay-0 bg-kp-crust px-2.5 text-sm text-kp-text focus:border-kp-mauve focus:outline-none"
              value={typed}
              onChange={(event) => setTyped(event.target.value)}
              aria-label={`Type ${requireTypingName} to confirm`}
            />
          </label>
        ) : null}
        <Checkbox checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)}>
          I understand this action cannot be undone.
        </Checkbox>
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="md" onClick={onCancel} disabled={pending}>{cancelLabel}</Button>
          <Button variant={accent.button} size="md" disabled={!canConfirm} onClick={onConfirm}>
            {pending ? pendingLabel : confirmLabel ?? defaultConfirm}
          </Button>
        </div>
      </div>
    </div>
  )
}
