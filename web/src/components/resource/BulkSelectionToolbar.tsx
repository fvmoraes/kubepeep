import { useState, type ReactNode } from 'react'
import { Copy } from 'lucide-react'
import { Button } from '../ui/Button'

export function BulkSelectionToolbar({ names, onClear, children }: { names: string[]; onClear: () => void; children?: ReactNode }) {
  const [copied, setCopied] = useState<string | null>(null)
  const [error, setError] = useState(false)
  const text = names.join('\n')
  return <div className="mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-kp-accent-border bg-kp-accent-bg/50 px-3 py-2" role="toolbar" aria-label="Bulk actions">
    <strong className="text-content text-kp-text">{names.length} selected</strong>
    {children}
    <Button variant="secondary" onClick={async () => {
      try { await navigator.clipboard.writeText(text); setCopied(text); setError(false) }
      catch { setError(true) }
    }}><Copy size={12} aria-hidden="true" />{copied === text ? 'Names copied' : 'Copy selected names'}</Button>
    <Button variant="ghost" onClick={onClear}>Clear selection</Button>
    <span className="text-content text-kp-subtext">Loaded rows only</span>
    {error ? <span role="alert" className="text-content text-kp-red">Clipboard unavailable. Allow clipboard access and try again.</span> : null}
  </div>
}
