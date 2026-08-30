import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { CommandCenter, type CommandRoute } from './CommandCenter'

const routes: readonly CommandRoute[] = [
  { path: '/', label: 'Overview', description: 'Cluster health and operational summary', keywords: ['dashboard'] },
  { path: '/workloads', label: 'Workloads', description: 'Deployments and StatefulSets', keywords: ['deployment'] },
  { path: '/pods', label: 'Pods', description: 'Pod inventory and containers', keywords: ['container'] },
  { path: '/logs', label: 'Logs', description: 'Bounded container log viewer', keywords: ['tail'] },
  { path: '/settings', label: 'Settings', description: 'Allowlisted local preferences', keywords: ['preferences'] },
]

function CurrentPath() {
  return <span data-testid="current-path">{useLocation().pathname}</span>
}

function renderCommandCenter({
  extra,
  onRefresh,
  initialEntries = ['/'],
  initialIndex = initialEntries.length - 1,
}: {
  extra?: React.ReactNode
  onRefresh?: () => void | Promise<unknown>
  initialEntries?: string[]
  initialIndex?: number
} = {}) {
  return render(
    <MemoryRouter initialEntries={initialEntries} initialIndex={initialIndex}>
      {extra}
      <CommandCenter routes={routes} onRefresh={onRefresh} />
      <CurrentPath />
    </MemoryRouter>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.history.replaceState(null, '')
  window.localStorage.clear()
  window.sessionStorage.clear()
})

describe('CommandCenter', () => {
  it('exposes a visible trigger and opens page search with Ctrl/Cmd+K', () => {
    renderCommandCenter()

    const trigger = screen.getByRole('button', { name: 'Open command center' })
    expect(trigger).toBeVisible()
    expect(trigger).toHaveAttribute('aria-keyshortcuts', 'Control+K Meta+K')

    fireEvent.keyDown(document, { key: 'k', ctrlKey: true })
    expect(screen.getByRole('dialog', { name: 'Command center' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Search application pages' })).toHaveFocus()

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    fireEvent.keyDown(document, { key: 'k', metaKey: true })
    expect(screen.getByRole('dialog', { name: 'Command center' })).toBeInTheDocument()
  })

  it('opens shortcut help with ? only outside editable fields', () => {
    const onRefresh = vi.fn()
    renderCommandCenter({ extra: <label>Scratch input<input aria-label="Scratch input" /></label>, onRefresh })

    const editable = screen.getByRole('textbox', { name: 'Scratch input' })
    editable.focus()
    fireEvent.keyDown(editable, { key: '?', shiftKey: true })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    fireEvent.keyDown(document.body, { key: '?', shiftKey: true })
    const dialog = screen.getByRole('dialog', { name: 'Keyboard shortcuts' })
    expect(dialog).toHaveClass('command-center-dialog--help')
    expect(dialog).toHaveTextContent('Refresh repeats only active read queries; no shortcut mutates Kubernetes resources.')
    expect(dialog).toHaveTextContent('Open page search from anywhere.')
    expect(dialog).toHaveTextContent('Refresh active read-only views instead of reloading the browser.')
    expect(dialog).toHaveTextContent('Focus and select the current page search')
    expect(dialog).toHaveTextContent('Open and focus the Kubernetes context selector when it is available.')
    expect(dialog).toHaveTextContent('Go back when local application history has an earlier entry.')
    expect(dialog).toHaveTextContent('Open this help outside editable fields.')

    expect(fireEvent.keyDown(within(dialog).getByRole('button', { name: 'Close command center' }), { key: 'r', ctrlKey: true })).toBe(true)
    expect(onRefresh).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Search pages' }))
    expect(screen.getByRole('dialog', { name: 'Command center' })).toHaveClass('command-center-dialog--commands')
  })

  it('refreshes and focuses safe targets with Ctrl/Meta only outside editors', () => {
    const onRefresh = vi.fn()
    renderCommandCenter({
      onRefresh,
      extra: <>
        <label>Page search<input data-app-shortcut="search" aria-label="Page search" defaultValue="pods" /></label>
        <label>Context<select data-app-shortcut="context-selector" aria-label="Context"><option>development</option></select></label>
        <div contentEditable role="textbox" aria-label="Notes" />
      </>,
    })

    const pageSearch = screen.getByRole('textbox', { name: 'Page search' }) as HTMLInputElement
    const context = screen.getByRole('combobox', { name: 'Context' })
    const notes = screen.getByRole('textbox', { name: 'Notes' })
    const showPicker = vi.fn()
    Object.defineProperty(context, 'showPicker', { configurable: true, value: showPicker })

    pageSearch.focus()
    expect(fireEvent.keyDown(pageSearch, { key: 'r', ctrlKey: true })).toBe(true)
    expect(fireEvent.keyDown(pageSearch, { key: 'k', metaKey: true })).toBe(true)
    expect(onRefresh).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    notes.focus()
    expect(fireEvent.keyDown(notes, { key: 'r', metaKey: true })).toBe(true)
    expect(onRefresh).not.toHaveBeenCalled()

    expect(fireEvent.keyDown(document.body, { key: 'r', ctrlKey: true })).toBe(false)
    expect(fireEvent.keyDown(document.body, { key: 'r', metaKey: true })).toBe(false)
    expect(onRefresh).toHaveBeenCalledTimes(2)

    expect(fireEvent.keyDown(document.body, { key: 'f', metaKey: true })).toBe(false)
    expect(pageSearch).toHaveFocus()
    expect(pageSearch.selectionStart).toBe(0)
    expect(pageSearch.selectionEnd).toBe(pageSearch.value.length)

    document.body.focus()
    expect(fireEvent.keyDown(document.body, { key: 'o', ctrlKey: true })).toBe(false)
    expect(context).toHaveFocus()
    expect(showPicker).toHaveBeenCalledOnce()
    expect(fireEvent.keyDown(context, { key: 'ArrowDown' })).toBe(true)
  })

  it('ignores composing, Process and repeated shortcut events', () => {
    const onRefresh = vi.fn()
    renderCommandCenter({ onRefresh })

    expect(fireEvent.keyDown(document.body, { key: 'r', ctrlKey: true, isComposing: true })).toBe(true)
    expect(fireEvent.keyDown(document.body, { key: 'Process', ctrlKey: true })).toBe(true)
    expect(fireEvent.keyDown(document.body, { key: 'r', ctrlKey: true, keyCode: 229 })).toBe(true)
    expect(fireEvent.keyDown(document.body, { key: 'r', ctrlKey: true, which: 229 })).toBe(true)
    expect(fireEvent.keyDown(document.body, { key: 'r', ctrlKey: true, repeat: true })).toBe(true)
    expect(onRefresh).not.toHaveBeenCalled()

    fireEvent.keyDown(document.body, { key: 'k', metaKey: true, isComposing: true })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it.each([
    ['Control', { ctrlKey: true }],
    ['Meta', { metaKey: true }],
  ])('goes back through router history with %s+B', (_label, modifier) => {
    window.history.replaceState({ idx: 1 }, '')
    renderCommandCenter({ initialEntries: ['/pods', '/logs'] })
    expect(screen.getByTestId('current-path')).toHaveTextContent('/logs')

    expect(fireEvent.keyDown(document.body, { key: 'b', ...modifier })).toBe(false)
    expect(screen.getByTestId('current-path')).toHaveTextContent('/pods')
  })

  it('leaves browser Find and Open untouched when no matching target is available', () => {
    renderCommandCenter()

    expect(fireEvent.keyDown(document.body, { key: 'f', ctrlKey: true })).toBe(true)
    expect(fireEvent.keyDown(document.body, { key: 'o', metaKey: true })).toBe(true)
    expect(fireEvent.keyDown(document.body, { key: 'b', ctrlKey: true })).toBe(true)
  })

  it('filters only the supplied static routes without issuing a request', () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    renderCommandCenter()

    fireEvent.click(screen.getByRole('button', { name: 'Open command center' }))
    const search = screen.getByRole('combobox', { name: 'Search application pages' })
    expect(screen.getAllByRole('option')).toHaveLength(routes.length)

    fireEvent.change(search, { target: { value: 'deployment' } })
    expect(screen.getByRole('option', { name: /Workloads/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /Pods/ })).not.toBeInTheDocument()

    fireEvent.change(search, { target: { value: 'cluster credential secret' } })
    expect(screen.getByText('No application page matches this search.')).toBeInTheDocument()
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(window.localStorage).toHaveLength(0)
    expect(window.sessionStorage).toHaveLength(0)
  })

  it('wraps arrow selection and opens the selected route with Enter', () => {
    renderCommandCenter()
    const trigger = screen.getByRole('button', { name: 'Open command center' })
    trigger.focus()
    fireEvent.click(trigger)

    const search = screen.getByRole('combobox', { name: 'Search application pages' })
    fireEvent.keyDown(search, { key: 'ArrowUp' })
    expect(screen.getByRole('option', { name: /Settings/ })).toHaveAttribute('aria-selected', 'true')

    fireEvent.keyDown(search, { key: 'ArrowDown' })
    fireEvent.keyDown(search, { key: 'ArrowDown' })
    expect(screen.getByRole('option', { name: /Workloads/ })).toHaveAttribute('aria-selected', 'true')
    fireEvent.keyDown(search, { key: 'Enter' })

    expect(screen.getByTestId('current-path')).toHaveTextContent('/workloads')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })

  it('traps Tab focus and restores the opener after Escape', () => {
    renderCommandCenter()
    const trigger = screen.getByRole('button', { name: 'Open command center' })
    trigger.focus()
    fireEvent.click(trigger)

    const dialog = screen.getByRole('dialog')
    const controls = within(dialog).getAllByRole('button')
    const first = controls[0]
    const last = controls.at(-1)
    expect(last).toBeDefined()

    last?.focus()
    fireEvent.keyDown(last as HTMLElement, { key: 'Tab' })
    expect(first).toHaveFocus()

    first.focus()
    fireEvent.keyDown(first, { key: 'Tab', shiftKey: true })
    expect(last).toHaveFocus()

    fireEvent.keyDown(dialog, { key: 'Escape' })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })
})
