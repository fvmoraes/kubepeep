import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'

import { getStatus, type CapabilityDecision, type CapabilityMatrix } from '../api/client'
import { getBatchedPermissions } from '../permissions/batchedPermissions'
import { StatePanel } from './StatePanel'

const decisionCopy: Record<CapabilityDecision, string> = {
  allowed: 'allowed',
  denied: 'denied',
  unknown: 'permission could not be verified',
}

export function PermissionsMatrixView({ matrix }: { matrix: CapabilityMatrix }) {
  if (matrix.decisions.length === 0) {
    return <StatePanel kind="unavailable" title="No permission decisions are available">Authorization could not produce a decision for the active generation.</StatePanel>
  }
  return (
    <>
      {!matrix.complete ? <p className="permission-notice" role="status">This matrix is partial. Unknown is not treated as a confirmed denial.</p> : null}
      {matrix.truncated ? <p className="permission-notice" role="status">Only the bounded namespace subset is shown.</p> : null}
      {matrix.errors.map((error) => <p className="field-error" role="status" key={`${error.namespace ?? 'global'}-${error.code}-${error.message}`}>{error.namespace ? `${error.namespace}: ` : ''}{error.message}</p>)}
      <div className="table-scroll">
        <table className="permissions-table">
          <caption>Capabilities for generation {matrix.generation}</caption>
          <thead><tr><th>Capability</th><th>Namespace / target</th><th>Operation</th><th>Decision</th></tr></thead>
          <tbody>
            {matrix.decisions.map((capability, index) => (
              <tr key={`${capability.capabilityId}-${capability.namespace}-${capability.resourceName}-${index}`}>
                <td><code>{capability.capabilityId}</code></td>
                <td>{capability.namespace || 'cluster'}{capability.resourceName ? ` / ${capability.resourceName}` : ''}</td>
                <td>{capability.verb} {capability.apiGroup ? `${capability.apiGroup}/` : ''}{capability.resource}{capability.subresource ? `/${capability.subresource}` : ''}</td>
                <td><span className={`permission-decision permission-decision--${capability.decision}`}>{decisionCopy[capability.decision]}</span></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}

export function PermissionsMatrixPage() {
  const [refresh, setRefresh] = useState(0)
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000, retry: false })
  const permissions = useQuery({
    queryKey: ['permissions', status.data?.selection?.generation, refresh],
    queryFn: ({ signal }) => getBatchedPermissions(status.data!.selection!, refresh > 0, signal),
    enabled: Boolean(status.data?.selection?.scopeMode && status.data.selection.namespaceCount > 0),
    retry: false,
  })

  if (status.isPending) {
    return <StatePanel kind="loading" title="Loading permissions">Checking the active selection.</StatePanel>
  }
  if (status.isError) {
    return <StatePanel kind="offline" title="Permissions are offline">The local API could not be reached.</StatePanel>
  }
  if (!status.data.selection) {
    return <StatePanel kind="empty" title="Select a context first">Permissions are evaluated only for the active generation and scope.</StatePanel>
  }
  if (!status.data.selection.scopeMode || status.data.selection.namespaceCount === 0) {
    return <StatePanel kind="empty" title="Select a namespace scope first">The permission matrix evaluates every allowlisted capability only inside the active scope.</StatePanel>
  }

  return (
    <section className="feature-card" aria-labelledby="permissions-title">
      <div className="feature-heading">
        <div><span className="eyebrow">RBAC</span><h1 id="permissions-title">Permission matrix</h1></div>
        <button type="button" className="button button--secondary" onClick={() => setRefresh((value) => value + 1)} disabled={permissions.isFetching}>Refresh permissions</button>
      </div>
      <p className="muted">The backend revalidates every protected action. This display never grants authority by itself.</p>
      {permissions.isPending ? <StatePanel kind="loading" title="Evaluating capabilities">SelfSubjectAccessReview decisions are loading.</StatePanel> : null}
      {permissions.isError ? <StatePanel kind="unavailable" title="Authorization is unavailable">Permission review could not produce a matrix. No mutation is assumed to be allowed.</StatePanel> : null}
      {permissions.data ? <PermissionsMatrixView matrix={permissions.data} /> : null}
    </section>
  )
}
