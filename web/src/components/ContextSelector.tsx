import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import {
  APIError,
  getClusterProfiles,
  getContexts,
  getSession,
  selectContext,
  type SelectionData,
  type SelectionSummary,
} from '../api/client'

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
    mutationFn: async ({ intent, controller }: { intent: number; controller: AbortController }) => {
      if (effectiveProfileId === null || effectiveContextName === '' || !session.data) {
        throw new Error('Context selection is not ready.')
      }
      const selected = await selectContext({
        clusterProfileId: effectiveProfileId,
        context: effectiveContextName,
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

  const submitSelection = () => {
    selectionController.current?.abort()
    const controller = new AbortController()
    selectionController.current = controller
    selectionIntent.current += 1
    setSelectionError(null)
    contextSelection.mutate({ intent: selectionIntent.current, controller })
  }

  if (profiles.isPending) {
    return <div className="context-selector context-selector--message" role="status">Loading kubeconfig profiles…</div>
  }
  if (profiles.isError) {
    return <div className="context-selector context-selector--message context-selector--error" role="status">{queryError(profiles.error)}</div>
  }
  if (profileList.length === 0) {
    return <div className="context-selector context-selector--message" role="status">No kubeconfig profile found</div>
  }

  return (
    <div className="context-selector" aria-label="Kubernetes context selector">
      <label>
        <span>Profile</span>
        <select
          aria-label="Cluster profile"
          value={effectiveProfileId ?? ''}
          onChange={(event) => {
            selectionController.current?.abort()
            setProfileId(Number(event.target.value))
            setContextName('')
            setSelectionError(null)
          }}
        >
          {profileList.map((profile) => <option key={profile.id} value={profile.id}>{profile.name}</option>)}
        </select>
      </label>
      <label>
        <span>Context</span>
        <select
          aria-label="Kubernetes context"
          value={effectiveContextName}
          disabled={contexts.isPending || contexts.isError || contextList.length === 0}
          onChange={(event) => {
            selectionController.current?.abort()
            setContextName(event.target.value)
            setSelectionError(null)
          }}
        >
          <option value="">{contexts.isPending ? 'Loading contexts…' : contexts.isError ? 'Contexts unavailable' : 'Choose a context'}</option>
          {contextList.map((context) => <option key={context.name} value={context.name}>{context.name} · {context.cluster}</option>)}
        </select>
      </label>
      <button
        type="button"
        className="button button--compact"
        onClick={submitSelection}
        disabled={effectiveContextName === '' || session.isPending || session.isError}
      >
        {contextSelection.isPending ? 'Switching…' : 'Select'}
      </button>
      {contexts.isError ? <small className="field-error" role="status">{queryError(contexts.error)}</small> : null}
      {contexts.data && contextList.length === 0 ? <small className="field-error" role="status">No contexts exist in this profile.</small> : null}
      {session.isError ? <small className="field-error" role="status">Session bootstrap is unavailable.</small> : null}
      {selectionError ? <small className="field-error" role="alert">{selectionError}</small> : null}
    </div>
  )
}
