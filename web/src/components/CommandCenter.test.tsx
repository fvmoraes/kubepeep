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

function renderCommandCenter(extra?: React.ReactNode) {
  return render(
    <MemoryRouter>
      {extra}
      <CommandCenter routes={routes} />
      <CurrentPath />
    </MemoryRouter>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
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
    renderCommandCenter(<label>Scratch input<input aria-label="Scratch input" /></label>)

    const editable = screen.getByRole('textbox', { name: 'Scratch input' })
    editable.focus()
    fireEvent.keyDown(editable, { key: '?', shiftKey: true })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    fireEvent.keyDown(document.body, { key: '?', shiftKey: true })
    const dialog = screen.getByRole('dialog', { name: 'Keyboard shortcuts' })
    expect(dialog).toHaveClass('command-center-dialog--help')
    expect(dialog).toHaveTextContent('Open page search from anywhere.')
    expect(dialog).toHaveTextContent('Open this help outside editable fields.')

    fireEvent.click(within(dialog).getByRole('button', { name: 'Search pages' }))
    expect(screen.getByRole('dialog', { name: 'Command center' })).toHaveClass('command-center-dialog--commands')
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
