import { ArrowDown, ArrowUp, CornerDownLeft, Keyboard, Search, X } from 'lucide-react'
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'

export interface CommandRoute {
  path: string
  label: string
  description: string
  keywords?: readonly string[]
}

interface CommandCenterProps {
  routes: readonly CommandRoute[]
}

type CommandCenterView = 'commands' | 'help' | null

function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || target.closest('input, textarea, select, [contenteditable="true"], [contenteditable=""], [role="textbox"]') !== null
}

function matchesQuery(route: CommandRoute, query: string) {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return true
  const searchable = [route.label, route.description, route.path, ...(route.keywords ?? [])].join(' ').toLowerCase()
  return terms.every((term) => searchable.includes(term))
}

export function CommandCenter({ routes }: CommandCenterProps) {
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

  const open = useCallback((nextView: Exclude<CommandCenterView, null>) => {
    if (view === null && document.activeElement instanceof HTMLElement) {
      returnFocusRef.current = document.activeElement
    }
    setView(nextView)
  }, [view])

  const close = useCallback(() => {
    setView(null)
    const returnTarget = returnFocusRef.current
    returnFocusRef.current = null
    if (returnTarget?.isConnected) returnTarget.focus({ preventScroll: true })
  }, [])

  useEffect(() => {
    const onGlobalKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return
      if ((event.metaKey || event.ctrlKey) && !event.altKey && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        open('commands')
        return
      }
      const opensHelp = event.key === '?' || (event.key === '/' && event.shiftKey)
      if (opensHelp && !event.metaKey && !event.ctrlKey && !event.altKey && !isEditableTarget(event.target)) {
        event.preventDefault()
        open('help')
      }
    }
    document.addEventListener('keydown', onGlobalKeyDown)
    return () => document.removeEventListener('keydown', onGlobalKeyDown)
  }, [open])

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

    if (view !== 'commands' || filteredRoutes.length === 0) return
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIndex((current) => (current + 1) % filteredRoutes.length)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIndex((current) => (current - 1 + filteredRoutes.length) % filteredRoutes.length)
    } else if (event.key === 'Enter' && event.target === inputRef.current) {
      event.preventDefault()
      chooseRoute(filteredRoutes[Math.min(activeIndex, filteredRoutes.length - 1)])
    }
  }

  return (
    <>
      <button
        type="button"
        className="command-center-trigger"
        aria-label="Open command center"
        aria-keyshortcuts="Control+K Meta+K"
        onClick={() => open('commands')}
      >
        <Search size={14} aria-hidden="true" />
        <span>Commands</span>
        <kbd>⌘/Ctrl K</kbd>
      </button>

      {view ? (
        <div
          className="command-center-backdrop"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) close()
          }}
        >
          <div
            ref={dialogRef}
            className={`command-center-dialog command-center-dialog--${view}`}
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
            aria-describedby={descriptionId}
            onKeyDown={onDialogKeyDown}
          >
            <header className="command-center-header">
              <div>
                <span className="eyebrow">local navigation</span>
                <h2 id={titleId}>{view === 'commands' ? 'Command center' : 'Keyboard shortcuts'}</h2>
              </div>
              <button
                ref={view === 'help' ? helpCloseRef : undefined}
                type="button"
                className="command-center-close"
                aria-label="Close command center"
                onClick={close}
              >
                <X size={17} aria-hidden="true" />
              </button>
            </header>

            {view === 'commands' ? (
              <>
                <p id={descriptionId} className="command-center-description">Search the pages built into this local application. No cluster data is queried.</p>
                <div className="command-center-search">
                  <Search size={17} aria-hidden="true" />
                  <input
                    ref={inputRef}
                    type="search"
                    role="combobox"
                    aria-label="Search application pages"
                    aria-autocomplete="list"
                    aria-controls={listboxId}
                    aria-expanded="true"
                    aria-activedescendant={filteredRoutes.length > 0 ? `${listboxId}-option-${activeIndex}` : undefined}
                    autoComplete="off"
                    spellCheck="false"
                    value={query}
                    placeholder="Search Overview, Pods, Logs…"
                    onChange={(event) => {
                      setQuery(event.target.value)
                      setActiveIndex(0)
                    }}
                  />
                </div>

                {filteredRoutes.length > 0 ? (
                  <div id={listboxId} className="command-center-results" role="listbox" aria-label="Application pages">
                    {filteredRoutes.map((route, index) => (
                      <button
                        key={route.path}
                        id={`${listboxId}-option-${index}`}
                        type="button"
                        role="option"
                        tabIndex={-1}
                        aria-selected={index === activeIndex}
                        className={index === activeIndex ? 'command-center-result command-center-result--active' : 'command-center-result'}
                        onMouseEnter={() => setActiveIndex(index)}
                        onFocus={() => setActiveIndex(index)}
                        onClick={() => chooseRoute(route)}
                      >
                        <span><strong>{route.label}</strong><small>{route.description}</small></span>
                        <code>{route.path}</code>
                      </button>
                    ))}
                  </div>
                ) : (
                  <p className="command-center-empty" role="status">No application page matches this search.</p>
                )}

                <footer className="command-center-footer">
                  <span><ArrowUp size={12} aria-hidden="true" /><ArrowDown size={12} aria-hidden="true" /> move</span>
                  <span><CornerDownLeft size={12} aria-hidden="true" /> open</span>
                  <span><kbd>Esc</kbd> close</span>
                  <button type="button" onClick={() => setView('help')}><Keyboard size={13} aria-hidden="true" /> ? shortcuts</button>
                </footer>
              </>
            ) : (
              <>
                <p id={descriptionId} className="command-center-description">Navigation stays local and the shortcuts do not read or mutate Kubernetes resources.</p>
                <dl className="command-center-help">
                  <div><dt><kbd>⌘/Ctrl K</kbd></dt><dd>Open page search from anywhere.</dd></div>
                  <div><dt><kbd>?</kbd></dt><dd>Open this help outside editable fields.</dd></div>
                  <div><dt><kbd>↑</kbd> <kbd>↓</kbd></dt><dd>Move through matching pages.</dd></div>
                  <div><dt><kbd>Enter</kbd></dt><dd>Open the selected page.</dd></div>
                  <div><dt><kbd>Esc</kbd></dt><dd>Close and return focus.</dd></div>
                  <div><dt><kbd>Tab</kbd></dt><dd>Move between controls inside the dialog.</dd></div>
                </dl>
                <footer className="command-center-footer command-center-footer--help">
                  <button type="button" onClick={() => setView('commands')}><Search size={13} aria-hidden="true" /> Search pages</button>
                </footer>
              </>
            )}
          </div>
        </div>
      ) : null}
    </>
  )
}
