import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useLocation, useNavigate, type Location } from 'react-router'

import { collectionListPath, resourceDetailPath, resourceKey } from '../../navigation/paths'

export interface WorkspaceEntry {
  collection: string
  kind: string | null
  namespace: string | null
  name: string
  tab: string
  listLocation?: Location
}

export interface WorkspaceRefInput {
  collection: string
  kind?: string | null
  namespace?: string | null
  name: string
}

interface WorkspaceContextValue {
  open: boolean
  /** Keep the source list mounted while the URL identifies the inspected object. */
  backgroundLocation: Location | null
  active: WorkspaceEntry | null
  canBack: boolean
  canForward: boolean
  historySize: number
  openRelatedResource: (ref: WorkspaceRefInput, tab?: string) => void
  clearListFocus: () => void
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
  const location = useLocation()
  const [entries, setEntries] = useState<WorkspaceEntry[]>([])
  const [index, setIndex] = useState(-1)
  const [open, setOpen] = useState(false)
  const [navigationFrom, setNavigationFrom] = useState<string | null>(null)
  const [origin, setOrigin] = useState<{ location: Location; returnTo: string } | null>(null)

  const active = index >= 0 && index < entries.length ? entries[index] : null
  // A sidebar/browser navigation immediately hides the old detail. The next
  // inspection starts a fresh history, even if its page loads asynchronously.
  // Router navigation can commit after our state update. Keep the source
  // mounted during that transition, so its deep-link effect cannot reopen
  // the previous object. Unrelated navigation has a different location key.
  const visible = open && Boolean(active && (resourceDetailPath(active) === location.pathname || navigationFrom === location.key))

  const navigateToEntry = useCallback((entry: WorkspaceEntry) => {
    const path = resourceDetailPath(entry)
    if (path) {
      setNavigationFrom(location.key)
      navigate(`${path}${entry.listLocation?.search ?? ''}`, { replace: true })
    }
  }, [location.key, navigate])

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

  const inspect = useCallback((ref: WorkspaceRefInput, tab?: string, fromRoute = false, related = false) => {
    if (!resourceDetailPath(ref)) return
    const listPath = collectionListPath(ref) ?? '/'
    const focusParams = new URLSearchParams({ focus: ref.name })
    if (ref.namespace) focusParams.set('namespace', ref.namespace)
    const listLocation = related ? { ...location, pathname: listPath, search: `?${focusParams}`, hash: '', key: resourceKey(ref) }
      : fromRoute ? { ...location, pathname: listPath } : undefined
    const nextEntry = { ...toEntry(ref, tab ?? defaultWorkspaceTab), listLocation: listLocation ?? (visible ? active?.listLocation : undefined) }
    if (!visible) {
      setOrigin({
        location: listLocation ?? location,
        returnTo: fromRoute ? `${collectionListPath(ref) ?? '/'}${location.search}${location.hash}` : `${location.pathname}${location.search}${location.hash}`,
      })
      commit([nextEntry], 0)
      return
    }
    const key = resourceKey(ref)
    const existing = entries.findIndex((entry) => resourceKey(entry) === key)
    if (existing >= 0) {
      const entry: WorkspaceEntry = { ...entries[existing], tab: tab ?? entries[existing].tab, listLocation: related ? listLocation : entries[existing].listLocation }
      const nextEntries = [...entries.slice(0, existing), entry]
      commit(nextEntries, existing)
      return
    }
    const nextEntries = [...entries.slice(0, index + 1), nextEntry]
    commit(nextEntries, nextEntries.length - 1)
  }, [active, commit, entries, index, location, visible])

  const openResource = useCallback((ref: WorkspaceRefInput, tab?: string) => inspect(ref, tab), [inspect])

  const openRelatedResource = useCallback((ref: WorkspaceRefInput, tab?: string) => inspect(ref, tab, false, true), [inspect])
  const clearListFocus = useCallback(() => {
    if (!active?.listLocation) return
    const params = new URLSearchParams(active.listLocation.search)
    params.delete('focus')
    const next = { ...active, listLocation: { ...active.listLocation, search: params.size ? `?${params}` : '' } }
    commit(entries.map((entry, at) => at === index ? next : entry), index)
  }, [active, commit, entries, index])

  const openFromRoute = useCallback((ref: WorkspaceRefInput, tab?: string) => {
    if (visible && active && resourceKey(active) === resourceKey(ref)) {
      if (tab && tab !== active.tab) setTab(tab)
      if (!open) setOpen(true)
      return
    }
    inspect(ref, tab, true)
  }, [active, visible, open, inspect, setTab])

  const close = useCallback(() => {
    setNavigationFrom(location.key)
    setOpen(false)
    navigate(active?.listLocation ? `${active.listLocation.pathname}${active.listLocation.search}` : origin?.returnTo ?? (active ? collectionListPath(active) : null) ?? '/', { replace: true })
  }, [active, location.key, navigate, origin])

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
    if (visible && origin) {
      setNavigationFrom(location.key)
      navigate(origin.returnTo, { replace: true })
    } else {
      setNavigationFrom(null)
      setOrigin(null)
    }
    setEntries([])
    setIndex(-1)
    setOpen(false)
  }, [location.key, navigate, origin, visible])

  // Escape closes the workspace; back/forward work from the keyboard too.
  useEffect(() => {
    if (!visible) return
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
  }, [back, close, forward, visible])

  const value = useMemo<WorkspaceContextValue>(() => ({
    open: visible,
    backgroundLocation: visible || navigationFrom === location.key ? active?.listLocation ?? origin?.location ?? null : null,
    active: visible ? active : null,
    canBack: visible && index > 0,
    canForward: visible && index < entries.length - 1,
    historySize: entries.length,
    openResource,
    openRelatedResource,
    clearListFocus,
    openFromRoute,
    close,
    back,
    forward,
    setTab,
    reset,
  }), [active, back, close, entries.length, forward, index, visible, origin, navigationFrom, location.key, openFromRoute, openResource, openRelatedResource, clearListFocus, reset, setTab])

  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>
}

export function useResourceWorkspace(): WorkspaceContextValue {
  const context = useContext(WorkspaceContext)
  if (!context) {
    throw new Error('useResourceWorkspace requires ResourceWorkspaceProvider.')
  }
  return context
}
