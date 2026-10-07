import { useState } from 'react'

import type { SelectionSummary, WorkloadDetail } from '../../api/types'
import { Checkbox } from '../ui'
import { PodLogsPanel } from './PodLogsPanel'

/** Reuse the authorized, UID-checked Pod references already in the detail. */
export function WorkloadLogsPanel({ detail, selection }: { detail: WorkloadDetail; selection: SelectionSummary }) {
  const namespace = detail.metadata.namespace!
  const [picked, setPicked] = useState<string[] | null>(null)
  const available = (detail.related ?? []).filter((ref) => ref.kind === 'Pod' && ref.namespace === namespace).slice(0, 100)
  const names = new Set(available.map((pod) => pod.name))
  const selected = (picked ?? available.slice(0, 5).map((pod) => pod.name)).filter((name) => names.has(name))
  if (!available.length) return <p className="text-content text-kp-overlay-text">No authorized Pod references are available in this workload detail. Relationships may be incomplete in the current scope.</p>
  return <div className="workload-logs-panel grid min-w-0 gap-2">
    <details className="workload-log-targets">
      <summary className="cursor-pointer text-content text-kp-subtext">Pods · {selected.length} selected / {available.length} loaded</summary>
      <div className="mt-2 flex max-h-40 flex-wrap gap-3 overflow-auto">
        {available.map((pod) => <Checkbox key={pod.name} checked={selected.includes(pod.name)} disabled={!selected.includes(pod.name) && selected.length >= 5} onChange={(event) => setPicked(event.target.checked ? [...selected, pod.name] : selected.filter((name) => name !== pod.name))}>{pod.name}</Checkbox>)}
      </div>
      <p role="note" className="text-content text-kp-overlay-text">Pod targets come from the loaded workload relationships; this list may be incomplete.</p>
      {available.length > 5 ? <p role="note" className="text-content text-kp-overlay-text">Up to 5 Pods at once. Change the selection above to inspect other logs.</p> : null}
    </details>
    {selected.length ? <PodLogsPanel key={JSON.stringify(selected)} pods={selected.map((name) => ({ namespace, name }))} selection={selection} /> : <p role="note">Select a Pod to view its logs.</p>}
  </div>
}
