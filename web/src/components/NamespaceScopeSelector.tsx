import { useEffect, useRef } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Waypoints } from 'lucide-react'
import { getNamespaceScopeOptions, selectNamespaceScope, type SelectionSummary } from '../api/client'
import { csrfForGeneration } from '../actions/csrf'
import { publishSelection } from '../api/selection'
import { errorMessage } from './resource/errors'
import { Select } from './ui'

export function NamespaceScopeSelector({ selection }: { selection: SelectionSummary | null }) {
  const client = useQueryClient()
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])
  const scopes = useQuery({
    queryKey: ['namespace-scopes', 'options', selection?.generation],
    queryFn: ({ signal }) => getNamespaceScopeOptions(signal),
    enabled: Boolean(selection),
    staleTime: 60_000,
    retry: false,
  })
  const options = (scopes.data ?? []).filter(scope => scope.clusterProfileId === selection?.clusterProfileId && scope.context === selection?.context)
  const change = useMutation({
    mutationFn: async (id: number) => {
      if (!selection || !options.some(scope => scope.id === id)) throw new Error('Choose a scope from the active context.')
      const request = new AbortController()
      controller.current = request
      const csrf = await csrfForGeneration(selection.generation, request.signal)
      return selectNamespaceScope(id, { expectedGeneration: selection.generation }, csrf, request.signal)
    },
    onSuccess: async selected => {
      await publishSelection(client, selected, selection?.generation, () => !controller.current?.signal.aborted)
      client.removeQueries({ queryKey: ['session'] })
    },
  })
  const currentLabel = selection
    ? `${selection.context} / ${selection.scopeName ?? (selection.scopeMode === 'all' ? 'All namespaces' : 'Choose namespace scope')}`
    : 'No Kubernetes context selected'
  return <div className="flex min-w-0 flex-wrap items-center gap-1.5">
    <Waypoints size={14} strokeWidth={1.8} className="shrink-0 text-kp-mauve" aria-hidden="true" />
    <Select aria-label="Namespace scope" className="!w-auto max-w-[15rem] pr-6 text-menu" title={selection ? `${selection.cluster} · ${selection.namespaceCount} namespaces in scope` : currentLabel} value={selection?.scopeId ?? ''} disabled={!selection || scopes.isPending || change.isPending} onChange={event => {
      if (event.target.value && Number(event.target.value) !== selection?.scopeId) change.mutate(Number(event.target.value))
    }}>
      {!options.some(scope => scope.id === selection?.scopeId) ? <option value={selection?.scopeId ?? ''}>{currentLabel}</option> : null}
      {options.map(scope => <option key={scope.id} value={scope.id}>{selection?.context} / {scope.name}</option>)}
    </Select>
    {change.isPending ? <span role="status" className="text-content text-kp-overlay-text">Switching scope…</span> : null}
    {change.isError ? <span role="alert" className="text-content text-kp-red">{errorMessage(change.error)}</span> : null}
    {scopes.isError ? <button type="button" className="text-content text-kp-red" onClick={() => void scopes.refetch()}>Retry scopes</button> : null}
  </div>
}
