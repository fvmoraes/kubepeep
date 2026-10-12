import { LoadingState } from './ui/LoadingState'
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
import type { StatusData } from '../api/types'
import { Select } from './ui'
import { KubeconfigImport } from './KubeconfigImport'
import { ContextColorPicker } from './ContextColorPicker'
import { publishSelection } from '../api/selection'

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
  const effectiveContextName = contextName === '' && effectiveProfileId !== selection?.clusterProfileId ? '' : preferredContext?.name ?? contextName

  const contextSelection = useMutation({
    mutationFn: async ({ intent, controller, context }: { intent: number; controller: AbortController; context: string }) => {
      if (effectiveProfileId === null || context === '') {
        throw new Error('Context selection is not ready.')
      }
      // The nonce rotates on every switch. Fetch it for this intent instead of
      // holding the selector disabled while unrelated lists refresh.
      const previousGeneration = queryClient.getQueryData<StatusData>(['local-status'])?.selection?.generation
      const session = await getSession(controller.signal)
      const selected = await selectContext({
        clusterProfileId: effectiveProfileId,
        context,
        setDefault: true,
        expectedGeneration: session.generation,
      }, session.csrfToken, controller.signal)
      return { selected, intent, previousGeneration }
    },
    onSuccess: async ({ selected, intent, previousGeneration }) => {
      if (intent !== selectionIntent.current) {
        return
      }
      setSelectionError(null)
      const published = await publishSelection(queryClient, selected, previousGeneration, () => intent === selectionIntent.current)
      if (!published || intent !== selectionIntent.current) return
      onSelected?.(selected)
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ['cluster-profiles'] }),
        queryClient.invalidateQueries({ queryKey: ['contexts'] }),
      ])
      queryClient.removeQueries({ queryKey: ['session'] })
    },
    onError: (error, { intent }) => {
      if (intent !== selectionIntent.current) return
      if (error instanceof DOMException && error.name === 'AbortError') {
        return
      }
      setSelectionError(queryError(error as Error))
    },
  })

  useEffect(() => () => { selectionIntent.current += 1; selectionController.current?.abort() }, [])

  const submitSelection = (context: string) => {
    selectionController.current?.abort()
    const controller = new AbortController()
    selectionController.current = controller
    selectionIntent.current += 1
    setSelectionError(null)
    contextSelection.mutate({ intent: selectionIntent.current, controller, context })
  }

  const importControl = <KubeconfigImport onImported={(profile) => {
    selectionController.current?.abort()
    selectionIntent.current += 1
    setProfileId(profile.id)
    setContextName('')
    setSelectionError(null)
  }} />

  if (profiles.isPending) {
    return <>{importControl}<LoadingState label="Loading kubeconfigs…" layout="inline" /></>
  }
  if (profiles.isError) {
    return <>{importControl}<div className="control flex items-center text-content text-kp-red" role="status">{queryError(profiles.error)}</div></>
  }
  if (profileList.length === 0) {
    return <>{importControl}<div className="control flex items-center text-content text-kp-overlay-text" role="status">No kubeconfig source found</div></>
  }

  return (
    <div className="flex items-center gap-1.5 min-w-0" aria-label="Kubernetes context selector">
      {importControl}
      <div className="relative min-w-0">
        <Select
          aria-label="Kubeconfig source"
          data-tip={kubeconfigLabel(preferredProfile)}
          className="!w-auto max-w-[10rem] pr-6 text-menu"
          value={effectiveProfileId ?? ''}
          onChange={(event) => {
            selectionController.current?.abort()
            selectionIntent.current += 1
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
        className="!w-auto max-w-[9rem] pr-6 text-menu text-kp-sky"
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
        className="!w-auto max-w-[11rem] pr-6 text-menu"
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
      <ContextColorPicker selection={selection} />
      {contextSelection.isPending ? <span className="text-content text-kp-overlay-text" role="status">Switching…</span> : null}
      {contexts.isError ? <span className="text-content text-kp-red" role="status">{queryError(contexts.error)}</span> : null}
      {contexts.data && contextList.length === 0 ? <span className="text-content text-kp-red" role="status">No contexts exist in this kubeconfig.</span> : null}
      {selectionError ? <span className="text-content text-kp-red" role="alert">{selectionError}</span> : null}
    </div>
  )
}
