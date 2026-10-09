import { useAutoRefreshQueryOptions } from './resource/AutoRefreshProvider'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'

import { getStatus, type Capability, type CapabilityDecision, type CapabilityMatrix } from '../api/client'
import { getBatchedPermissions } from '../permissions/batchedPermissions'
import { StatePanel } from './StatePanel'
import { Badge, Card, CardContent, DataTable, PageHeader } from './ui'
import { ResourceListControls } from './ResourceListControls'
import { usePreferenceColumnVisibility, type ColumnVisibilityState } from './resource/columns'
import { WarningBanner } from './ui/Banner'
import { errorMessage } from './resource/errors'

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

export function PermissionsMatrixView({ matrix, columnVisibility }: { matrix: CapabilityMatrix; columnVisibility?: ColumnVisibilityState }) {
  const [search, setSearch] = useState('')
  const columns = useMemo(() => [
    {
      key: 'capability',
      header: 'Capability',
      cell: (capability: Capability) => <span>{capability.capabilityId}</span>,
    },
    {
      key: 'namespace', header: 'Namespace', cell: (capability: Capability) => capability.namespace || 'cluster',
    },
    {
      key: 'resource-name', sortKey: 'resourceName', header: 'Target', cell: (capability: Capability) => capability.resourceName || '—',
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
      <ResourceListControls search={search} appliedSearch={search} onSearchChange={setSearch} onApply={() => {}} />
      {!matrix.complete ? <WarningBanner className="mb-2.5">This matrix is partial. Unknown is not treated as a confirmed denial.</WarningBanner> : null}
      {matrix.truncated ? <WarningBanner className="mb-2.5">Only the bounded namespace subset is shown.</WarningBanner> : null}
      {matrix.errors.map((error) => <p className="text-content text-kp-red" role="status" key={`${error.namespace ?? 'global'}-${error.code}-${error.message}`}>{error.namespace ? `${error.namespace}: ` : ''}{error.message}</p>)}
      <DataTable
        caption={`Capabilities for generation ${matrix.generation}`}
        columns={columns} columnVisibility={columnVisibility}
        rows={matrix.decisions.filter((item) => [item.capabilityId, item.namespace, item.resourceName, item.verb, item.resource, item.apiGroup, item.subresource, decisionCopy[item.decision]].join(' ').toLocaleLowerCase().includes(search.toLocaleLowerCase()))}
        getRowKey={(capability, index) => `${capability.capabilityId}-${capability.namespace}-${capability.resourceName}-${index}`}
      />
    </>
  )
}

export function PermissionsMatrixPage() {
  const autoRefresh = useAutoRefreshQueryOptions()
  const columnVisibility = usePreferenceColumnVisibility('permissions')
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000, retry: false })
  const permissions = useQuery({
    queryKey: ['permissions', status.data?.selection?.generation],
    queryFn: ({ signal }) => getBatchedPermissions(status.data!.selection!, false, signal),
    enabled: Boolean(status.data?.selection?.scopeMode && status.data.selection.namespaceCount > 0),
    staleTime: 45_000,
    ...autoRefresh,
    refetchIntervalInBackground: false,
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
    <div className="resource-page flex w-full min-w-0 flex-col gap-4">
      <PageHeader
        title="Permission matrix"
        description="Effective capabilities evaluated by SelfSubjectAccessReview for the active scope."
      />
      <Card className="resource-page-content p-4">
        <CardContent className="resource-page-content flex-1 min-h-0 p-0">
          <p className="m-0 text-content text-kp-overlay-text">The backend revalidates every protected action. This display never grants authority by itself.</p>
          {permissions.isPending ? <StatePanel kind="loading" title="Evaluating capabilities">SelfSubjectAccessReview decisions are loading.</StatePanel> : null}
          {permissions.isError ? <StatePanel kind="unavailable" title="Permission review could not complete">{errorMessage(permissions.error)} No mutation is assumed to be allowed.</StatePanel> : null}
          {permissions.data ? <PermissionsMatrixView matrix={permissions.data} columnVisibility={columnVisibility} /> : null}
        </CardContent>
      </Card>
    </div>
  )
}
