import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'

import { getStatus, type Capability, type CapabilityDecision, type CapabilityMatrix } from '../api/client'
import { getBatchedPermissions } from '../permissions/batchedPermissions'
import { StatePanel } from './StatePanel'
import { Badge, Button, Card, CardContent, CardHeader, DataTable } from './ui'

const decisionCopy: Record<CapabilityDecision, string> = {
  allowed: 'allowed',
  denied: 'denied',
  unknown: 'permission could not be verified',
}

function decisionBadgeVariant(decision: CapabilityDecision) {
  switch (decision) {
    case 'allowed':
      return 'healthy'
    case 'denied':
      return 'danger'
    case 'unknown':
      return 'warning'
  }
}

export function PermissionsMatrixView({ matrix }: { matrix: CapabilityMatrix }) {
  const columns = useMemo(() => [
    {
      key: 'capability',
      header: 'Capability',
      cell: (capability: Capability) => <code className="text-kp-sky">{capability.capabilityId}</code>,
    },
    {
      key: 'target',
      header: 'Namespace / target',
      cell: (capability: Capability) => <>{capability.namespace || 'cluster'}{capability.resourceName ? ` / ${capability.resourceName}` : ''}</>,
    },
    {
      key: 'operation',
      header: 'Operation',
      cell: (capability: Capability) => <>{capability.verb} {capability.apiGroup ? `${capability.apiGroup}/` : ''}{capability.resource}{capability.subresource ? `/${capability.subresource}` : ''}</>,
    },
    {
      key: 'decision',
      header: 'Decision',
      cell: (capability: Capability) => (
        <Badge variant={decisionBadgeVariant(capability.decision)} className={`permission-decision permission-decision--${capability.decision}`}>
          {decisionCopy[capability.decision]}
        </Badge>
      ),
    },
  ], [])

  if (matrix.decisions.length === 0) {
    return <StatePanel kind="unavailable" title="No permission decisions are available">Authorization could not produce a decision for the active generation.</StatePanel>
  }
  return (
    <>
      {!matrix.complete ? <p className="permission-notice" role="status">This matrix is partial. Unknown is not treated as a confirmed denial.</p> : null}
      {matrix.truncated ? <p className="permission-notice" role="status">Only the bounded namespace subset is shown.</p> : null}
      {matrix.errors.map((error) => <p className="field-error" role="status" key={`${error.namespace ?? 'global'}-${error.code}-${error.message}`}>{error.namespace ? `${error.namespace}: ` : ''}{error.message}</p>)}
      <DataTable
        caption={`Capabilities for generation ${matrix.generation}`}
        columns={columns}
        rows={matrix.decisions}
        getRowKey={(capability, index) => `${capability.capabilityId}-${capability.namespace}-${capability.resourceName}-${index}`}
      />
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
    <Card className="max-w-[1120px]" aria-labelledby="permissions-title">
      <CardContent>
        <CardHeader>
          <div><span className="eyebrow">RBAC</span><h1 id="permissions-title" className="text-xl text-kp-text mt-1">Permission matrix</h1></div>
          <Button variant="secondary" size="compact" onClick={() => setRefresh((value) => value + 1)} disabled={permissions.isFetching}>Refresh permissions</Button>
        </CardHeader>
        <p className="text-kp-overlay-text text-md leading-[1.65] mb-5">The backend revalidates every protected action. This display never grants authority by itself.</p>
        {permissions.isPending ? <StatePanel kind="loading" title="Evaluating capabilities">SelfSubjectAccessReview decisions are loading.</StatePanel> : null}
        {permissions.isError ? <StatePanel kind="unavailable" title="Authorization is unavailable">Permission review could not produce a matrix. No mutation is assumed to be allowed.</StatePanel> : null}
        {permissions.data ? <PermissionsMatrixView matrix={permissions.data} /> : null}
      </CardContent>
    </Card>
  )
}
