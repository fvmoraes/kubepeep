import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router'

import { collectionListPath, resourceDetailPath, resourceKey } from '../../navigation/paths'

export interface WorkspaceEntry {
  collection: string
  kind: string | null
  namespace: string | null
  name: string
  tab: string
}

export interface WorkspaceRefInput {
  collection: string
  kind?: string | null
  namespace?: string | null
  name: string
}

interface WorkspaceContextValue {
  open: boolean
  active: WorkspaceEntry | null
  canBack: boolean
  canForward: boolean
  historySize: number
  openResource: (ref: WorkspaceRefInput, tab?: string) => void
  /** Deep-link path: opens without duplicating history when already active. */
  openFromRoute: (ref: WorkspaceRefInput, tab?: string) => void
  close: () => void
  back: () => void
  forward: () => void
  setTab: (tab: string) => void
  reset: () => void
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null)

export const defaultWorkspaceTab = 'overview'

function toEntry(ref: WorkspaceRefInput, tab: string): WorkspaceEntry {
  return { collection: ref.collection, kind: ref.kind ?? null, namespace: ref.namespace ?? null, name: ref.name, tab }
}

export function ResourceWorkspaceProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const [entries, setEntries] = useState<WorkspaceEntry[]>([])
  const [index, setIndex] = useState(-1)
  const [open, setOpen] = useState(false)

  const active = index >= 0 && index < entries.length ? entries[index] : null

  const navigateToEntry = useCallback((entry: WorkspaceEntry) => {
    const path = resourceDetailPath(entry)
    if (path) navigate(path, { replace: true })
  }, [navigate])

  const commit = useCallback((nextEntries: WorkspaceEntry[], nextIndex: number) => {
    setEntries(nextEntries)
    setIndex(nextIndex)
    setOpen(true)
    navigateToEntry(nextEntries[nextIndex])
  }, [navigateToEntry])

  const setTab = useCallback((tab: string) => {
    setEntries((current) => {
      if (index < 0 || index >= current.length) return current
      const next = [...current]
      next[index] = { ...next[index], tab }
      return next
    })
  }, [index])

  const openResource = useCallback((ref: WorkspaceRefInput, tab?: string) => {
    const key = resourceKey(ref)
    const existing = entries.findIndex((entry) => resourceKey(entry) === key)
    if (existing >= 0) {
      const entry: WorkspaceEntry = { ...entries[existing], tab: tab ?? entries[existing].tab }
      const nextEntries = [...entries.slice(0, existing), entry]
      commit(nextEntries, existing)
      return
    }
    const nextEntries = [...entries.slice(0, index + 1), toEntry(ref, tab ?? defaultWorkspaceTab)]
    commit(nextEntries, nextEntries.length - 1)
  }, [commit, entries, index])

  const openFromRoute = useCallback((ref: WorkspaceRefInput, tab?: string) => {
    if (active && resourceKey(active) === resourceKey(ref)) {
      if (tab && tab !== active.tab) setTab(tab)
      if (!open) setOpen(true)
      return
    }
    openResource(ref, tab)
  }, [active, open, openResource, setTab])

  const close = useCallback(() => {
    setOpen(false)
    const target = active ? collectionListPath(active) : null
    navigate(target ?? '/', { replace: false })
  }, [active, navigate])

  const back = useCallback(() => {
    if (index <= 0) return
    const nextIndex = index - 1
    setIndex(nextIndex)
    setOpen(true)
    navigateToEntry(entries[nextIndex])
  }, [entries, index, navigateToEntry])

  const forward = useCallback(() => {
    if (index >= entries.length - 1) return
    const nextIndex = index + 1
    setIndex(nextIndex)
    setOpen(true)
    navigateToEntry(entries[nextIndex])
  }, [entries, index, navigateToEntry])

  const reset = useCallback(() => {
    setEntries([])
    setIndex(-1)
    setOpen(false)
  }, [])

  // Escape closes the workspace; back/forward work from the keyboard too.
  useEffect(() => {
    if (!open) return
    function handleKeyDown(event: KeyboardEvent) {
      const target = event.target as HTMLElement | null
      const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)
      if (event.key === 'Escape' && !typing) {
        event.preventDefault()
        close()
      }
      if (!typing && (event.altKey || event.metaKey) && event.key === 'ArrowLeft') {
        event.preventDefault()
        back()
      }
      if (!typing && (event.altKey || event.metaKey) && event.key === 'ArrowRight') {
        event.preventDefault()
        forward()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [back, close, forward, open])

  const value = useMemo<WorkspaceContextValue>(() => ({
    open,
    active,
    canBack: index > 0,
    canForward: index < entries.length - 1,
    historySize: entries.length,
    openResource,
    openFromRoute,
    close,
    back,
    forward,
    setTab,
    reset,
  }), [active, back, close, entries.length, forward, index, open, openFromRoute, openResource, reset, setTab])

  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>
}

export function useResourceWorkspace(): WorkspaceContextValue {
  const context = useContext(WorkspaceContext)
  if (!context) {
    throw new Error('useResourceWorkspace requires ResourceWorkspaceProvider.')
  }
  return context
}
