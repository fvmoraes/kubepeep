import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import {
  APIError,
  getClusterProfiles,
  getContexts,
  getSession,
  selectContext,
  type ClusterProfile,
  type SelectionData,
  type SelectionSummary,
} from '../api/client'
import { Select } from './ui'

interface ContextSelectorProps {
  selection: SelectionSummary | null
  onSelected?: (selection: SelectionData) => void
}

function queryError(error: Error): string {
  if (error instanceof APIError) {
    if (error.code === 'CONTEXT_NOT_FOUND') {
      return 'The selected context no longer exists.'
    }
    if (error.code === 'KUBECONFIG_INVALID') {
      return 'The selected kubeconfig is invalid.'
    }
    return error.message
  }
  return 'The local API is offline.'
}

function kubeconfigLabel(profile: ClusterProfile): string {
  const paths = profile.kubeconfigFiles
    .slice()
    .sort((left, right) => left.position - right.position)
    .map((file) => file.displayPath.trim())
    .filter(Boolean)
  return paths.length > 0 ? paths.join(' + ') : 'Kubeconfig source unavailable'
}

/**
 * Compact header control for kubeconfig, cluster and context.
 * Selecting a context applies it immediately — no extra confirmation step.
 */
export function ContextSelector({ selection, onSelected }: ContextSelectorProps) {
  const queryClient = useQueryClient()
  const selectionController = useRef<AbortController | null>(null)
  const selectionIntent = useRef(0)
  const [profileId, setProfileId] = useState<number | null>(selection?.clusterProfileId ?? null)
  const [contextName, setContextName] = useState(selection?.context ?? '')
  const [selectionError, setSelectionError] = useState<string | null>(null)

  const profiles = useQuery({
    queryKey: ['cluster-profiles'],
    queryFn: ({ signal }) => getClusterProfiles(signal),
    staleTime: 15_000,
    retry: false,
  })
  const profileList = Array.isArray(profiles.data) ? profiles.data : []

  const preferredProfile = profileList.find((profile) => profile.id === profileId)
    ?? profileList.find((profile) => profile.id === selection?.clusterProfileId)
    ?? profileList.find((profile) => profile.isDefault)
    ?? profileList[0]
  const effectiveProfileId = preferredProfile?.id ?? null

  const contexts = useQuery({
    queryKey: ['contexts', effectiveProfileId],
    queryFn: ({ signal }) => getContexts(effectiveProfileId!, signal),
    enabled: effectiveProfileId !== null,
    staleTime: 15_000,
    retry: false,
  })
  const contextList = Array.isArray(contexts.data) ? contexts.data : []

  const preferredContext = contextList.find((context) => context.name === contextName)
    ?? contextList.find((context) => context.selected)
    ?? contextList.find((context) => context.name === selection?.context)
    ?? contextList[0]
  const effectiveContextName = preferredContext?.name ?? contextName

  const session = useQuery({
    queryKey: ['session'],
    queryFn: ({ signal }) => getSession(signal),
    enabled: effectiveProfileId !== null && effectiveContextName !== '',
    staleTime: 5 * 60_000,
    retry: false,
  })

  const contextSelection = useMutation({
    mutationFn: async ({ intent, controller, context }: { intent: number; controller: AbortController; context: string }) => {
      if (effectiveProfileId === null || context === '' || !session.data) {
        throw new Error('Context selection is not ready.')
      }
      const selected = await selectContext({
        clusterProfileId: effectiveProfileId,
        context,
        setDefault: true,
        expectedGeneration: session.data.generation,
      }, session.data.csrfToken, controller.signal)
      return { selected, intent }
    },
    onSuccess: async ({ selected, intent }) => {
      if (intent !== selectionIntent.current) {
        return
      }
      setSelectionError(null)
      onSelected?.(selected)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['local-status'] }),
        queryClient.invalidateQueries({ queryKey: ['cluster-profiles'] }),
        queryClient.invalidateQueries({ queryKey: ['contexts'] }),
      ])
      queryClient.removeQueries({ queryKey: ['session'] })
      await queryClient.prefetchQuery({ queryKey: ['session'], queryFn: ({ signal }) => getSession(signal), staleTime: 5 * 60_000 })
    },
    onError: (error) => {
      if (error instanceof DOMException && error.name === 'AbortError') {
        return
      }
      setSelectionError(queryError(error as Error))
    },
  })

  useEffect(() => () => selectionController.current?.abort(), [])

  const submitSelection = (context: string) => {
    selectionController.current?.abort()
    const controller = new AbortController()
    selectionController.current = controller
    selectionIntent.current += 1
    setSelectionError(null)
    contextSelection.mutate({ intent: selectionIntent.current, controller, context })
  }

  if (profiles.isPending) {
    return <div className="flex h-8 items-center text-xs text-kp-overlay-text" role="status">Loading kubeconfigs…</div>
  }
  if (profiles.isError) {
    return <div className="flex h-8 items-center text-xs text-kp-red" role="status">{queryError(profiles.error)}</div>
  }
  if (profileList.length === 0) {
    return <div className="flex h-8 items-center text-xs text-kp-overlay-text" role="status">No kubeconfig source found</div>
  }

  return (
    <div className="flex items-center gap-1.5 min-w-0" aria-label="Kubernetes context selector">
      <div className="relative min-w-0">
        <Select
          aria-label="Kubeconfig source"
          data-tip={kubeconfigLabel(preferredProfile)}
          className="!w-auto max-w-[10rem] pr-6 text-sm"
          value={effectiveProfileId ?? ''}
          onChange={(event) => {
            selectionController.current?.abort()
            setProfileId(Number(event.target.value))
            setContextName('')
            setSelectionError(null)
          }}
        >
          {profileList.map((profile) => <option key={profile.id} value={profile.id}>{kubeconfigLabel(profile)}</option>)}
        </Select>
      </div>
      <Select
        aria-label="Selected cluster"
        data-tip={preferredContext?.cluster ? `Cluster ${preferredContext.cluster}` : 'Choose a context to resolve the cluster'}
        className="!w-auto max-w-[9rem] pr-6 text-sm text-kp-sky"
        disabled
        value={preferredContext?.cluster ?? ''}
        onChange={() => {}}
      >
        <option value="">{preferredContext?.cluster ?? 'Cluster'}</option>
      </Select>
      <Select
        aria-label="Kubernetes context"
        aria-keyshortcuts="Control+O Meta+O"
        data-app-shortcut="context-selector"
        className="!w-auto max-w-[11rem] pr-6 text-sm"
        value={effectiveContextName}
        disabled={contexts.isPending || contexts.isError || contextList.length === 0 || contextSelection.isPending}
        onChange={(event) => {
          selectionController.current?.abort()
          setContextName(event.target.value)
          setSelectionError(null)
          if (event.target.value !== '') {
            submitSelection(event.target.value)
          }
        }}
      >
        <option value="">{contexts.isPending
          ? 'Loading contexts…'
          : contexts.isError
            ? 'Contexts unavailable'
            : contextList.length > 0
              ? `Choose a context (${contextList.length} available)`
              : 'Choose a context'}</option>
        {contextList.map((context) => <option key={context.name} value={context.name}>{context.name} · {context.cluster}</option>)}
      </Select>
      {contextSelection.isPending ? <span className="text-xs text-kp-overlay-text" role="status">Switching…</span> : null}
      {contexts.isError ? <span className="text-xs text-kp-red" role="status">{queryError(contexts.error)}</span> : null}
      {contexts.data && contextList.length === 0 ? <span className="text-xs text-kp-red" role="status">No contexts exist in this kubeconfig.</span> : null}
      {session.isError ? <span className="text-xs text-kp-red" role="status">Session bootstrap is unavailable.</span> : null}
      {selectionError ? <span className="text-xs text-kp-red" role="alert">{selectionError}</span> : null}
    </div>
  )
}
