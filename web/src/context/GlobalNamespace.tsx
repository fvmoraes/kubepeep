import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'

import { getNamespaces, getNamespaceScope } from '../api/client'

interface GlobalNamespaceContextValue {
  /** '' means All — every namespace allowed by the active scope. */
  value: string
  options: string[]
  loading: boolean
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
export function GlobalNamespaceProvider({ generation, scopeId, scopeMode, children }: {
  generation: string | undefined
  scopeId: number | null
  scopeMode: string | null
  children: ReactNode
}) {
  const [rawValue, setRawValue] = useState('')
  const [prevGeneration, setPrevGeneration] = useState(generation)

  // Render-time adjustment (React "you might not need an effect"): a context
  // switch resets the filter to All so a stale namespace can never leak into
  // the new selection.
  if (generation !== prevGeneration) {
    setPrevGeneration(generation)
    setRawValue('')
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

  const { options, degraded, loading } = useMemo(() => {
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

  // A namespace picked for a previous scope must never surface as selected
  // when the option universe no longer contains it.
  const value = rawValue !== '' && !loading && options.length > 0 && !options.includes(rawValue) ? '' : rawValue

  const contextValue = useMemo<GlobalNamespaceContextValue>(() => ({ value, options, loading, degraded, setValue: setRawValue }), [degraded, loading, options, value])
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
