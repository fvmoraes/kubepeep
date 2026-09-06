import { Layers } from 'lucide-react'

import { Select } from './ui'
import { useGlobalNamespace } from '../context/GlobalNamespace'

/**
 * Global namespace control for the top bar: kubeconfig → context → scope →
 * namespace. `All` selects every namespace allowed by the active scope; the
 * list itself is the scope universe (never beyond RBAC).
 */
export function GlobalNamespaceSelect({ disabled }: { disabled?: boolean }) {
  const { value, options, loading, degraded, setValue } = useGlobalNamespace()
  if (degraded && options.length === 0) {
    return (
      <Select aria-label="Global namespace" className="!w-auto max-w-[12rem] pr-6 text-sm" value="" disabled data-tip="The namespace list is unavailable for this scope (RBAC or cluster error)">
        <option value="">Namespace: All</option>
      </Select>
    )
  }
  return (
    <Select
      aria-label="Global namespace"
      data-tip={degraded ? 'Namespace list could not be fully verified' : 'Namespace applied to every namespaced view'}
      className="!w-auto max-w-[13rem] pr-6 text-sm"
      value={value}
      disabled={disabled || loading}
      onChange={(event) => setValue(event.target.value)}
    >
      <option value="">Namespace: All</option>
      {options.map((namespace) => <option key={namespace} value={namespace}>{namespace}</option>)}
    </Select>
  )
}

export function NamespaceIndicator() {
  const { value } = useGlobalNamespace()
  return (
    <span className="inline-flex items-center gap-1 text-xs text-kp-overlay-text" data-tip="Active global namespace filter">
      <Layers size={12} aria-hidden="true" className="text-kp-mauve" />
      {value === '' ? 'All namespaces' : value}
    </span>
  )
}
