import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'

import { getNamespaces, getNamespaceScope } from '../api/client'

interface GlobalNamespaceContextValue {
  /** '' means All — every namespace allowed by the active scope. */
  value: string
  options: string[]
  loading: boolean
  /** Resource views wait until a concrete initial namespace is resolved. */
  ready: boolean
  /** True when the option list could not be fully verified (RBAC denial). */
  degraded: boolean
  setValue: (value: string) => void
}

const GlobalNamespaceContext = createContext<GlobalNamespaceContextValue | null>(null)

/**
 * Global namespace filter: one selection drives every namespaced collection
 * in the app. The option universe is the active Scope — `All` means every
 * namespace the Scope (and RBAC) allows, never more.
 */
export function GlobalNamespaceProvider({ generation, scopeId, scopeMode, defaultNamespace, requestedNamespace, children }: {
  generation: string | undefined
  scopeId: number | null
  scopeMode: string | null
  requestedNamespace?: string | null
  defaultNamespace?: string | null
  children: ReactNode
}) {
  const [rawValue, setRawValue] = useState<string | undefined>(requestedNamespace ?? undefined)
  const binding = `${generation ?? ''}/${scopeId ?? ''}/${scopeMode ?? ''}/${defaultNamespace ?? ''}`
  const [previousRequested, setPreviousRequested] = useState(requestedNamespace)
  if (requestedNamespace !== previousRequested) { setPreviousRequested(requestedNamespace); if (requestedNamespace) setRawValue(requestedNamespace) }
  const [previousBinding, setPreviousBinding] = useState(binding)

  // Render-time adjustment (React "you might not need an effect"): a context
  // switch restores the new scope's default before resource views render.
  // Only an explicit user selection can set the filter to All ('').
  if (binding !== previousBinding) {
    setPreviousBinding(binding)
    setRawValue(undefined)
  }

  const scopeDetail = useQuery({
    queryKey: ['global-namespace-scope', generation, scopeId],
    queryFn: ({ signal }) => getNamespaceScope(scopeId!, signal),
    enabled: scopeMode !== 'all' && scopeId != null,
    staleTime: 15_000,
  })
  const clusterNamespaces = useQuery({
    queryKey: ['global-namespace-cluster', generation],
    queryFn: ({ signal }) => getNamespaces({ limit: 500 }, signal),
    enabled: scopeMode === 'all',
    staleTime: 60_000,
  })

  const { options: available, degraded, loading } = useMemo(() => {
    if (scopeMode === 'all') {
      if (clusterNamespaces.isPending) return { options: [] as string[], degraded: false, loading: true }
      if (clusterNamespaces.isError) return { options: [] as string[], degraded: true, loading: false }
      const names = (clusterNamespaces.data ?? []).map((namespace) => namespace.name).filter(Boolean).sort((left, right) => left.localeCompare(right))
      return { options: names, degraded: false, loading: false }
    }
    if (scopeDetail.isPending && scopeId != null) return { options: [] as string[], degraded: false, loading: true }
    if (scopeDetail.isError) return { options: [] as string[], degraded: true, loading: false }
    const names = [...(scopeDetail.data?.namespaces ?? [])].sort((left, right) => left.localeCompare(right))
    return { options: names, degraded: false, loading: false }
  }, [clusterNamespaces.isError, clusterNamespaces.isPending, clusterNamespaces.data, scopeDetail.isError, scopeDetail.isPending, scopeDetail.data, scopeId, scopeMode])

  const initialNamespace = defaultNamespace ?? scopeDetail.data?.defaultNamespace
    ?? (available.includes('default') ? 'default' : available[0]) ?? ''
  const options = useMemo(() => initialNamespace
    ? [initialNamespace, ...available.filter((name) => name !== initialNamespace)]
    : available, [available, initialNamespace])
  // An unavailable explicit choice falls back to the default, never to All.
  const value = rawValue === undefined || rawValue !== '' && !loading && !degraded && !options.includes(rawValue)
    ? initialNamespace : rawValue
  const ready = value !== '' || rawValue === ''

  const contextValue = useMemo<GlobalNamespaceContextValue>(() => ({ value, options, loading, ready, degraded, setValue: setRawValue }), [degraded, loading, ready, options, value])
  return <GlobalNamespaceContext.Provider value={contextValue}>{children}</GlobalNamespaceContext.Provider>
}

export function useGlobalNamespace(): GlobalNamespaceContextValue {
  const context = useContext(GlobalNamespaceContext)
  if (!context) {
    throw new Error('useGlobalNamespace requires GlobalNamespaceProvider.')
  }
  return context
}

/** Effective namespace list for a namespaced collection query. */
export function effectiveNamespaces(globalNamespace: string, filterNamespaces: string[]): string[] {
  if (globalNamespace) return [globalNamespace]
  return filterNamespaces
}
