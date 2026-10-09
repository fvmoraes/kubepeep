import { Select } from './ui'
import { useGlobalNamespace } from '../context/GlobalNamespace'

/**
 * Global namespace control for the top bar: kubeconfig → context → scope →
 * namespace. `All` selects every namespace allowed by the active scope; the
 * list itself is the scope universe (never beyond RBAC).
 */
export function GlobalNamespaceSelect({ disabled }: { disabled?: boolean }) {
  const { value, options, loading, ready, degraded, setValue } = useGlobalNamespace()
  if (degraded && options.length === 0) {
    return (
      <Select aria-label="Global namespace" className="!w-auto max-w-[12rem] pr-6 text-menu" value="" disabled title="The namespace list is unavailable for this scope (RBAC or cluster error)">
          <option value="">Namespaces unavailable</option>
      </Select>
    )
  }
  return (
    <Select
      aria-label="Global namespace"
      title={degraded ? 'Namespace list could not be fully verified' : 'Namespace applied to every namespaced view'}
      className="!w-auto max-w-[13rem] pr-6 text-menu"
      value={value}
      disabled={disabled || loading}
      onChange={(event) => setValue(event.target.value)}
    >
        {!ready ? <option value="">{loading ? 'Loading namespaces…' : 'Choose a namespace scope'}</option> : null}
        {options.slice(0, 1).map((namespace) => <option key={namespace} value={namespace}>{namespace}</option>)}
        {ready ? <option value="">Namespace: All</option> : null}
        {options.slice(1).map((namespace) => <option key={namespace} value={namespace}>{namespace}</option>)}
    </Select>
  )
}
