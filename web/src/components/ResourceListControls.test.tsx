import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ResourceListControls } from './ResourceListControls'

const sortOptions = [
  { value: 'identity', label: 'Namespace and name' },
  { value: 'age', label: 'Age' },
] as const

afterEach(cleanup)

describe('ResourceListControls', () => {
  it('distinguishes applied filters from a pending search and exposes server ordering', () => {
    const onApply = vi.fn()
    const onClear = vi.fn()
    const onSortChange = vi.fn()
    const onOrderChange = vi.fn()

    render(<ResourceListControls
      search="draft"
      appliedSearch="running"
      onSearchChange={vi.fn()}
      onApply={onApply}
      onRefresh={vi.fn()}
      onClear={onClear}
      activeFilters={[{ id: 'namespace', label: 'Namespace', value: 'payments, ops' }]}
      sort="age"
      order="desc"
      appliedSort="identity"
      appliedOrder="asc"
      defaultSort="identity"
      defaultOrder="asc"
      hasPendingChanges
      sortOptions={sortOptions}
      onSortChange={onSortChange}
      onOrderChange={onOrderChange}
    />)

    expect(screen.getByText('running')).toBeInTheDocument()
    expect(screen.getByText('payments, ops')).toBeInTheDocument()
    expect(screen.getByLabelText('Search this bounded page')).toHaveValue('draft')
    expect(within(screen.getByLabelText('Applied resource list state')).queryByText('draft')).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Filter changes pending')
    expect(screen.getByText('Namespace and name · ascending')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Sort this bounded page'), { target: { value: 'identity' } })
    fireEvent.change(screen.getByLabelText('Order'), { target: { value: 'asc' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))

    expect(onSortChange).toHaveBeenCalledWith('identity')
    expect(onOrderChange).toHaveBeenCalledWith('asc')
    expect(onApply).toHaveBeenCalledOnce()
    expect(onClear).toHaveBeenCalledOnce()
  })

  it('reports an unfiltered default query and disables redundant clear', () => {
    render(<ResourceListControls
      search=""
      appliedSearch=""
      onSearchChange={vi.fn()}
      onApply={vi.fn()}
      onRefresh={vi.fn()}
      onClear={vi.fn()}
      sort="identity"
      order="asc"
      appliedSort="identity"
      appliedOrder="asc"
      defaultSort="identity"
      defaultOrder="asc"
      hasPendingChanges={false}
      sortOptions={sortOptions}
      onSortChange={vi.fn()}
      onOrderChange={vi.fn()}
    />)

    expect(screen.getByText('None')).toBeInTheDocument()
    expect(screen.getByText('Namespace and name · ascending')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clear filters' })).toBeDisabled()
  })
})
