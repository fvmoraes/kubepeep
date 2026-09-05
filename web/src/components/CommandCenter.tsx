import { ArrowDown, ArrowUp, CornerDownLeft, Keyboard, Search, X } from 'lucide-react'
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router'

import { Button, Input } from './ui'

export interface CommandRoute {
  path: string
  label: string
  description: string
  keywords?: readonly string[]
}

interface CommandCenterProps {
  routes: readonly CommandRoute[]
  // Visible-resource entries (F7-04): identifiers only, gathered from
  // bounded pages already loaded in this session. Resolved when the palette
  // opens so the index always reflects the freshest cache without reactive
  // subscriptions.
  getResources?: () => readonly CommandRoute[]
  // Saved favorite targets (F7-01): resolved at open time, rendered first.
  getFavorites?: () => readonly CommandRoute[]
  getRecent?: () => readonly CommandRoute[]
  onClearRecent?: () => void
  onRefresh?: () => void | Promise<unknown>
}

type CommandCenterView = 'commands' | 'help' | null

function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || target.closest('input, textarea, select, [contenteditable="true"], [contenteditable=""], [role="textbox"]') !== null
}

function usesPrimaryModifier(event: KeyboardEvent) {
  return event.ctrlKey !== event.metaKey && !event.altKey && !event.shiftKey
}

function isComposingShortcutEvent(event: KeyboardEvent) {
  return event.isComposing || event.key === 'Process' || event.keyCode === 229 || event.which === 229
}

function activateShortcutTarget(name: 'context-selector' | 'search') {
  const targets = Array.from(document.querySelectorAll<HTMLElement>(`[data-app-shortcut="${name}"]`))
  const target = targets.find((candidate) => (
    !candidate.matches(':disabled, [aria-disabled="true"]')
    && candidate.closest('[hidden], [aria-hidden="true"]') === null
  ))
  if (!target) return false
  target.focus({ preventScroll: true })
  if (document.activeElement !== target) return false
  if (target instanceof HTMLInputElement) target.select()
  if (name === 'context-selector' && target instanceof HTMLSelectElement && typeof target.showPicker === 'function') {
    try {
      target.showPicker()
    } catch {
      // Focus remains a keyboard-accessible fallback when the browser refuses a programmatic native picker.
    }
  }
  return true
}

function hasLocalHistoryEntry() {
  const state = window.history.state as unknown
  if (typeof state !== 'object' || state === null || !('idx' in state)) return false
  const index = (state as { idx?: unknown }).idx
  return typeof index === 'number' && Number.isInteger(index) && index > 0
}

function matchesQuery(route: CommandRoute, query: string) {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return true
  const searchable = [route.label, route.description, route.path, ...(route.keywords ?? [])].join(' ').toLowerCase()
  return terms.every((term) => searchable.includes(term))
}

export function CommandCenter({ routes, getFavorites, getRecent, getResources, onRefresh, onClearRecent }: CommandCenterProps) {
  const [sessionResources, setSessionResources] = useState<readonly CommandRoute[]>([])
  const [sessionFavorites, setSessionFavorites] = useState<readonly CommandRoute[]>([])
  const [sessionRecent, setSessionRecent] = useState<readonly CommandRoute[]>([])
  const navigate = useNavigate()
  const [view, setView] = useState<CommandCenterView>(null)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const dialogRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const helpCloseRef = useRef<HTMLButtonElement>(null)
  const returnFocusRef = useRef<HTMLElement | null>(null)
  const titleId = useId()
  const descriptionId = useId()
  const listboxId = useId()
  const filteredRoutes = useMemo(() => routes.filter((route) => matchesQuery(route, query)), [query, routes])
  const filteredResources = useMemo(
    () => (sessionResources.length > 0 ? sessionResources.filter((entry) => matchesQuery(entry, query)) : []),
    [query, sessionResources],
  )
  const filteredFavorites = useMemo(
    () => (sessionFavorites.length > 0 ? sessionFavorites.filter((entry) => matchesQuery(entry, query)) : []),
    [query, sessionFavorites],
  )
  const filteredRecent = useMemo(
    () => (sessionRecent.length > 0 ? sessionRecent.filter((entry) => matchesQuery(entry, query)) : []),
    [query, sessionRecent],
  )
  const combinedResults = useMemo(
    () => [...filteredFavorites, ...filteredRecent, ...filteredRoutes, ...(filteredResources.length > 0 ? filteredResources : [])],
    [filteredFavorites, filteredRecent, filteredResources, filteredRoutes],
  )

  const open = useCallback((nextView: Exclude<CommandCenterView, null>) => {
    if (view === null && document.activeElement instanceof HTMLElement) {
      returnFocusRef.current = document.activeElement
    }
    if (nextView === 'commands') {
      setSessionFavorites(getFavorites?.() ?? [])
      setSessionRecent(getRecent?.() ?? [])
      setSessionResources(getResources?.() ?? [])
    }
    setView(nextView)
  }, [getFavorites, getRecent, getResources, view])

  const close = useCallback(() => {
    setView(null)
    const returnTarget = returnFocusRef.current
    returnFocusRef.current = null
    if (returnTarget?.isConnected) returnTarget.focus({ preventScroll: true })
  }, [])

  useEffect(() => {
    const onGlobalKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat || isComposingShortcutEvent(event) || isEditableTarget(event.target)) return
      if (view !== null) return
      const key = event.key.toLowerCase()
      if (usesPrimaryModifier(event)) {
        if (key === 'k') {
          event.preventDefault()
          open('commands')
          return
        }
        if (key === 'r' && onRefresh) {
          event.preventDefault()
          void onRefresh()
          return
        }
        if (key === 'f' && activateShortcutTarget('search')) {
          event.preventDefault()
          return
        }
        if (key === 'o' && activateShortcutTarget('context-selector')) {
          event.preventDefault()
          return
        }
        if (key === 'b' && hasLocalHistoryEntry()) {
          event.preventDefault()
          navigate(-1)
          return
        }
      }
      const opensHelp = event.key === '?' || (event.key === '/' && event.shiftKey)
      if (opensHelp && !event.metaKey && !event.ctrlKey && !event.altKey) {
        event.preventDefault()
        open('help')
      }
    }
    document.addEventListener('keydown', onGlobalKeyDown)
    return () => document.removeEventListener('keydown', onGlobalKeyDown)
  }, [navigate, onRefresh, open, view])

  useEffect(() => {
    if (view === null) return
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    if (view === 'commands') inputRef.current?.focus({ preventScroll: true })
    else helpCloseRef.current?.focus({ preventScroll: true })
    return () => {
      document.body.style.overflow = previousOverflow
    }
  }, [view])

  const chooseRoute = useCallback((route: CommandRoute) => {
    navigate(route.path)
    setQuery('')
    setActiveIndex(0)
    close()
  }, [close, navigate])

  const onDialogKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      close()
      return
    }

    if (event.key === 'Tab') {
      const dialog = dialogRef.current
      if (!dialog) return
      const focusable = Array.from(dialog.querySelectorAll<HTMLElement>('button:not([disabled]):not([tabindex="-1"]), input:not([disabled]), [href], [tabindex]:not([tabindex="-1"])'))
      const first = focusable[0]
      const last = focusable.at(-1)
      if (!first || !last) return
      if (event.shiftKey && (document.activeElement === first || !dialog.contains(document.activeElement))) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && (document.activeElement === last || !dialog.contains(document.activeElement))) {
        event.preventDefault()
        first.focus()
      }
      return
    }

    if (view !== 'commands' || combinedResults.length === 0) return
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIndex((current) => (current + 1) % combinedResults.length)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIndex((current) => (current - 1 + combinedResults.length) % combinedResults.length)
    } else if (event.key === 'Enter' && event.target === inputRef.current) {
      event.preventDefault()
      chooseRoute(combinedResults[Math.min(activeIndex, combinedResults.length - 1)])
    }
  }

  const resultClass = (index: number) => `w-full min-h-[46px] flex items-center justify-between gap-4 px-2.5 py-1.5 rounded-md text-left cursor-pointer border ${
    index === activeIndex ? 'bg-kp-surface-3 border-kp-overlay-1' : 'bg-transparent border-transparent'
  }`

  return (
    <>
      <Button
        variant="secondary"
        size="md"
        className="text-kp-overlay-text font-normal"
        aria-label="Open command center"
        aria-keyshortcuts="Control+K Meta+K"
        onClick={() => open('commands')}
      >
        <Search size={14} aria-hidden="true" />
        <span>Search…</span>
        <kbd>⌘K</kbd>
      </Button>

      {view ? createPortal(
        <div
          className="fixed z-[var(--z-command-backdrop)] inset-0 grid place-items-start justify-center overflow-y-auto py-[min(12vh,96px)] px-4 bg-black/70 backdrop-blur-sm"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) close()
          }}
        >
          <div
            ref={dialogRef}
            role="dialog"
            aria-modal="true"
            data-view={view}
            aria-labelledby={titleId}
            aria-describedby={descriptionId}
            onKeyDown={onDialogKeyDown}
            className={`w-[min(680px,100%)] max-h-[min(720px,78vh)] grid overflow-hidden rounded-xl border border-kp-overlay-1 bg-kp-surface-0 shadow-dialog text-kp-text ${
              view === 'commands' ? 'grid-rows-[auto_auto_auto_minmax(0,1fr)_auto]' : 'grid-rows-[auto_auto_minmax(0,1fr)_auto]'
            }`}
          >
            <header className="flex items-center justify-between gap-5 px-4 pt-4 pb-2.5">
              <div>
                <h2 id={titleId} className="text-xl">{view === 'commands' ? 'Command center' : 'Keyboard shortcuts'}</h2>
              </div>
              <button
                ref={view === 'help' ? helpCloseRef : undefined}
                type="button"
                aria-label="Close command center"
                onClick={close}
                className="h-8 w-8 grid place-items-center rounded-md text-kp-overlay-text hover:text-kp-text hover:bg-kp-surface-3"
              >
                <X size={16} aria-hidden="true" />
              </button>
            </header>

            {view === 'commands' ? (
              <>
                <p id={descriptionId} className="px-4 pb-2.5 text-xs text-kp-overlay-text leading-relaxed">{sessionResources.length > 0 ? 'Search pages and resources already loaded in this session. Only names and namespaces are searched; no resource content is read.' : 'Search the pages built into this local application. No cluster data is queried.'}</p>
                <div className="mx-4 mb-3 grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2.5 px-3 rounded-md border border-kp-overlay-1 bg-kp-crust focus-within:border-kp-mauve focus-within:shadow-focus">
                  <Search size={16} aria-hidden="true" className="text-kp-mauve" />
                  <Input
                    ref={inputRef}
                    type="search"
                    role="combobox"
                    aria-label="Search application pages"
                    aria-autocomplete="list"
                    aria-controls={listboxId}
                    aria-expanded="true"
                    aria-activedescendant={combinedResults.length > 0 ? `${listboxId}-option-${activeIndex}` : undefined}
                    autoComplete="off"
                    spellCheck="false"
                    value={query}
                    placeholder="Search Overview, Pods, Logs…"
                    className="border-0 bg-transparent px-0 shadow-none focus:border-0 focus:shadow-none"
                    onChange={(event) => {
                      setQuery(event.target.value)
                      setActiveIndex(0)
                    }}
                  />
                </div>

                {combinedResults.length > 0 ? (
                  <div id={listboxId} role="listbox" aria-label={sessionResources.length > 0 || sessionFavorites.length > 0 || sessionRecent.length > 0 ? 'Favorites, recent targets, application pages and visible resources' : 'Application pages'} className="min-h-0 grid gap-0.5 content-start overflow-y-auto px-2.5 pb-2.5">
                    {[...filteredFavorites, ...filteredRecent, ...filteredRoutes, ...filteredResources].map((entry, index) => (
                      <button
                        key={`${entry.path}-${index}`}
                        id={`${listboxId}-option-${index}`}
                        type="button"
                        role="option"
                        tabIndex={-1}
                        aria-selected={index === activeIndex}
                        className={resultClass(index)}
                        onMouseEnter={() => setActiveIndex(index)}
                        onFocus={() => setActiveIndex(index)}
                        onClick={() => chooseRoute(entry)}
                      >
                        <span className="block min-w-0"><strong className="block text-sm text-kp-text">{entry.label}</strong><small className="block mt-0.5 text-xs text-kp-overlay-text">{entry.description}</small></span>
                        <code className="shrink-0 text-xs">{entry.path}</code>
                      </button>
                    ))}
                  </div>
                ) : (
                  <p role="status" className="min-h-[110px] grid place-items-center px-6 text-sm text-kp-overlay-text text-center">{sessionResources.length > 0 ? 'No page or loaded resource matches this search.' : 'No application page matches this search.'}</p>
                )}

                <footer className="flex items-center gap-3.5 border-t border-kp-overlay-0 bg-kp-surface-1 px-4 py-2 text-xs text-kp-overlay-text">
                  <span className="inline-flex items-center gap-1"><ArrowUp size={12} aria-hidden="true" /><ArrowDown size={12} aria-hidden="true" /> move</span>
                  <span className="inline-flex items-center gap-1"><CornerDownLeft size={12} aria-hidden="true" /> open</span>
                  <span className="inline-flex items-center gap-1"><kbd>Esc</kbd> close</span>
                  {sessionRecent.length > 0 && onClearRecent ? <button type="button" className="inline-flex items-center gap-1.5 text-kp-sky hover:text-kp-text" onClick={() => { onClearRecent(); setSessionRecent([]) }}>clear recent</button> : null}
                  <button type="button" className="ml-auto inline-flex items-center gap-1.5 text-kp-sky hover:text-kp-text" onClick={() => setView('help')}><Keyboard size={13} aria-hidden="true" /> ? shortcuts</button>
                </footer>
              </>
            ) : (
              <>
                <p id={descriptionId} className="px-4 pb-2.5 text-xs text-kp-overlay-text leading-relaxed">Navigation and focus stay local. Refresh repeats only active read queries; no shortcut mutates Kubernetes resources.</p>
                <dl className="min-h-0 grid gap-px overflow-y-auto mx-4 mb-4 content-start">
                  {[
                    ['⌘/Ctrl K', 'Open page search from anywhere.'],
                    ['⌘/Ctrl R', 'Refresh active read-only views instead of reloading the browser.'],
                    ['⌘/Ctrl F', 'Focus and select the current page search; browser Find remains available when no page search exists.'],
                    ['⌘/Ctrl O', 'Open and focus the Kubernetes context selector when it is available.'],
                    ['⌘/Ctrl B', 'Go back when local application history has an earlier entry.'],
                    ['?', 'Open this help outside editable fields.'],
                    ['↑ ↓', 'Move through matching pages.'],
                    ['Enter', 'Open the selected page.'],
                    ['Esc', 'Close and return focus.'],
                    ['Tab', 'Move between controls inside the dialog.'],
                  ].map(([keys, help]) => (
                    <div key={keys} className="grid grid-cols-[minmax(110px,0.4fr)_1fr] items-center gap-4 rounded-md bg-kp-surface-1 px-2.5 py-2">
                      <dt><kbd>{keys}</kbd></dt>
                      <dd className="m-0 text-xs text-kp-subtext leading-relaxed">{help}</dd>
                    </div>
                  ))}
                </dl>
                <footer className="flex items-center justify-end border-t border-kp-overlay-0 bg-kp-surface-1 px-4 py-2">
                  <button type="button" className="inline-flex items-center gap-1.5 text-kp-sky hover:text-kp-text" onClick={() => setView('commands')}><Search size={13} aria-hidden="true" /> Search pages</button>
                </footer>
              </>
            )}
          </div>
        </div>,
        document.body,
      ) : null}
    </>
  )
}
