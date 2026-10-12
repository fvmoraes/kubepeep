import { useEffect, useRef } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ExternalLink, RotateCcw, Square, Trash2 } from 'lucide-react'
import { closePortForward, createIdempotencyKey, createPortForward, createServicePortForward, getPortForwards, getPreferences } from '../api/client'
import { forwardRecipe, portForwardURL, sameForwardTarget, saveForwardRecipe } from '../api/portForwards'
import { csrfForGeneration } from '../actions/csrf'
import type { PortForward, SavedPortForward, SelectionSummary } from '../api/types'
import { useAutoRefreshQueryOptions } from './resource/AutoRefreshProvider'
import { Badge, Button } from './ui'
import { useToast } from './ui/Toast'

export function PortForwardLink({ session }: { session: PortForward }) {
  const url = portForwardURL(session)
  if (!url) return null
  return <a className="control inline-flex items-center gap-1.5 px-2 text-kp-sky" href={url} target="_blank" rel="noopener noreferrer" aria-label={`Open ${url} in browser`} onClick={event => {
    const runtime = (window as Window & { runtime?: { BrowserOpenURL?: (url: string) => void } }).runtime
    if (runtime?.BrowserOpenURL) { event.preventDefault(); runtime.BrowserOpenURL(url) }
  }}><ExternalLink size={12} aria-hidden="true" />Open in browser</a>
}

export function PortForwardList({ selection, target, inactiveOnly = false }: { selection: SelectionSummary; target?: Pick<SavedPortForward, 'kind' | 'namespace' | 'name'>; inactiveOnly?: boolean }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const refresh = useAutoRefreshQueryOptions()
  const lifecycle = useRef(new AbortController())
  useEffect(() => {
    const controller = new AbortController()
    lifecycle.current = controller
    return () => controller.abort()
  }, [selection.generation, target?.kind, target?.namespace, target?.name])
  const sessions = useQuery({ queryKey: ['port-forwards', selection.generation], queryFn: ({ signal }) => getPortForwards(signal, selection.generation), ...refresh })
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal), staleTime: 60_000 })
  const inScope = (item: SavedPortForward) => item.clusterProfileId === selection.clusterProfileId && item.context === selection.context && (!target || item.kind === target.kind && item.namespace === target.namespace && item.name === target.name)
  const active = (sessions.data ?? []).filter(session => session.status === 'active' && inScope(forwardRecipe(session)))
  const rows: Array<{ recipe: SavedPortForward; session?: PortForward }> = inactiveOnly ? [] : active.map(session => ({ recipe: forwardRecipe(session), session }))
  for (const recipe of preferences.data?.portForwards ?? []) {
    if (inScope(recipe) && !active.some(session => sameForwardTarget(forwardRecipe(session), recipe))) rows.push({ recipe })
  }
  const operation = useMutation({
    mutationFn: async ({ action, recipe, session }: { action: 'stop' | 'delete' | 'enable'; recipe: SavedPortForward; session?: PortForward }) => {
      const signal = lifecycle.current.signal
      const csrf = await csrfForGeneration(selection.generation, signal)
      if (action === 'enable') {
        const create = recipe.kind === 'Service' ? createServicePortForward : createPortForward
        const result = await create(recipe.namespace, recipe.name, { confirmed: true, action: 'portForward', consequenceCode: recipe.kind === 'Service' ? 'EXPOSE_SERVICE_PORT_LOCALLY' : 'EXPOSE_POD_PORT_LOCALLY', target: { clusterProfileId: recipe.clusterProfileId, context: recipe.context, namespace: recipe.namespace, kind: recipe.kind, name: recipe.name }, expectedGeneration: selection.generation, remotePort: recipe.remotePort, localPort: recipe.localPort }, csrf, createIdempotencyKey(), signal)
        // Successful reactivation uses the already-persisted exact ports.
        return result
      }
      if (session) await closePortForward(session.id, selection.generation, csrf, signal)
      if (action === 'delete') {
        const saved = await saveForwardRecipe(recipe, selection.generation, signal, true)
        queryClient.setQueryData(['preferences'], saved)
      }
    },
    onSuccess: (_, { action }) => toast.info(action === 'enable' ? 'Port-forward active' : action === 'delete' ? 'Port-forward deleted' : 'Port-forward stopped'),
    onError: error => { if (!(error instanceof Error && error.name === 'AbortError') && !lifecycle.current.signal.aborted) toast.error('Port-forward action failed', error instanceof Error ? error.message : 'Please retry.') },
    onSettled: () => { void queryClient.invalidateQueries({ queryKey: ['port-forwards'] }) },
  })
  if (inactiveOnly && rows.length === 0) return null
  return <section className="grid gap-2" aria-label={inactiveOnly ? 'Saved inactive port forwards' : 'Object port forwards'}>
    <h3 className="m-0 text-content font-semibold">{inactiveOnly ? 'Saved port forwards' : 'Port forwards'}</h3>
    <p className="m-0 text-content text-kp-overlay-text">One active connection per object. Ports are saved locally; after restarting KubePeep, use Re-enable to reconnect.</p>
    {sessions.isError || preferences.isError ? <p className="m-0 text-content text-kp-yellow" role="alert">Some port-forward information could not be loaded. <Button variant="ghost" onClick={() => { void sessions.refetch(); void preferences.refetch() }}>Retry</Button></p> : null}
    {sessions.isPending || preferences.isPending ? <p className="m-0 text-content" role="status">Loading port forwards…</p> : rows.length === 0 ? <p className="m-0 text-content text-kp-overlay-text">No port forwards for this object.</p> : null}
    <ul className="m-0 grid list-none gap-2 p-0">
      {rows.map(({ recipe, session }) => <li key={`${recipe.kind}/${recipe.namespace}/${recipe.name}`} className="flex flex-wrap items-center gap-2 rounded border border-kp-overlay-0 bg-kp-surface-0 p-2">
        <div className="min-w-0 flex-1 text-content"><span className="break-all">Loopback listener: <strong>127.0.0.1:{recipe.localPort}</strong> → {recipe.kind}/{recipe.namespace}/{recipe.name}:{recipe.remotePort}</span></div>
        <Badge variant={session ? 'healthy' : 'unknown'}>{session ? 'Active' : 'Inactive'}</Badge>
        {session ? <PortForwardLink session={session} /> : null}
        <Button variant="secondary" disabled={operation.isPending || sessions.isPending || sessions.isError} onClick={() => operation.mutate({ action: session ? 'stop' : 'enable', recipe, session })}>{session ? <Square size={12} aria-hidden="true" /> : <RotateCcw size={12} aria-hidden="true" />}{session ? 'Stop' : 'Re-enable'}</Button>
        <Button variant="danger" disabled={operation.isPending || sessions.isPending || sessions.isError || preferences.isError} aria-label={`Delete port-forward for ${recipe.kind} ${recipe.name}`} onClick={() => operation.mutate({ action: 'delete', recipe, session })}><Trash2 size={12} aria-hidden="true" />Delete</Button>
      </li>)}
    </ul>
  </section>
}
