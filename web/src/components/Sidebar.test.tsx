import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { Sidebar } from './Sidebar'
import { navGroups, settingsNavItem } from '../navigation/tree'

function LocationProbe() {
  return <output aria-label="Current path">{useLocation().pathname}</output>
}

afterEach(cleanup)

describe('Sidebar click contract', () => {
  it('navigates every enabled destination and explains every disabled item', () => {
    const onToggleCompact = vi.fn()
    render(
      <MemoryRouter>
        <Sidebar version="test" compact={false} onToggleCompact={onToggleCompact} collapsedGroups={[]} onToggleGroup={vi.fn()} />
        <LocationProbe />
      </MemoryRouter>,
    )

    for (const group of navGroups) {
      const region = screen.getByRole('region', { name: group.label })
      for (const item of group.items) {
        if (item.path) {
          fireEvent.click(within(region).getByRole('link', { name: item.label }))
          expect(screen.getByLabelText('Current path')).toHaveTextContent(item.path)
          continue
        }
        const disabled = within(region).getByText(item.label).closest('[aria-disabled="true"]')
        expect(disabled).toHaveAttribute('data-tip', expect.stringContaining('available in a future release'))
      }
    }

    fireEvent.click(screen.getByRole('link', { name: settingsNavItem.label }))
    expect(screen.getByLabelText('Current path')).toHaveTextContent(settingsNavItem.path!)
    fireEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    expect(onToggleCompact).toHaveBeenCalledOnce()
  })
})
