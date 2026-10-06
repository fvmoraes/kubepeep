import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { DataTable } from './DataTable'

it('keeps a 50k-row table DOM near the viewport', () => {
  const rows = Array.from({ length: 50_000 }, (_, index) => ({ name: `pod-${index}` }))
  render(<DataTable caption="Pods" rows={rows} getRowKey={(row) => row.name} columns={[{ key: 'name', header: 'Pod', cell: (row) => row.name }]} />)

  expect(screen.getByRole('table', { name: 'Pods' })).toHaveAttribute('aria-rowcount', '50001')
  expect(screen.getByText('pod-0')).toBeInTheDocument()
  expect(screen.getAllByRole('row').length).toBeLessThan(60)
  expect(screen.queryByText('pod-49999')).not.toBeInTheDocument()
})

afterEach(cleanup)

it('sorts numbers in either direction while keeping Job Pods last, and filters values in place', () => {
  const rows = [{ name: 'job', type: 'Job', restarts: 0 }, { name: 'api', type: 'Pod', restarts: 12 }, { name: 'web', type: 'Pod', restarts: 2 }]
  const toggle = vi.fn()
  render(<DataTable rows={rows} rowGroup={(row) => row.type === 'Job' ? 1 : 0} getRowKey={(row) => row.name} selectable onToggleAll={toggle} columns={[
    { key: 'name', header: 'Name', cell: (row) => row.name },
    { key: 'type', header: 'Type', cell: (row) => row.type },
    { key: 'restarts', header: 'Restarts', cell: (row) => row.restarts },
  ]} />)
  const names = () => screen.getAllByRole('row').slice(1).map((row) => within(row).getAllByRole('cell')[1].textContent)
  fireEvent.click(screen.getByRole('button', { name: 'Restarts' }))
  expect(names()).toEqual(['web', 'api', 'job'])
  expect(screen.getByRole('columnheader', { name: /Restarts/ })).toHaveAttribute('aria-sort', 'ascending')
  fireEvent.click(screen.getByRole('button', { name: 'Restarts' }))
  expect(names()).toEqual(['api', 'web', 'job'])
  fireEvent.click(screen.getByRole('button', { name: 'Filter Type' }))
  fireEvent.click(screen.getByRole('checkbox', { name: 'Job' }))
  expect(names()).toEqual(['api', 'web'])
  fireEvent.keyDown(document, { key: 'Escape' })
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select all rows on this page' }))
  expect(toggle).toHaveBeenLastCalledWith(true, [rows[1], rows[2]])
  fireEvent.click(screen.getByRole('button', { name: 'Choose visible columns' }))
  fireEvent.click(screen.getByRole('checkbox', { name: 'Restarts' }))
  expect(screen.queryByRole('columnheader', { name: /Restarts/ })).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Reset columns' }))
  expect(screen.getByRole('columnheader', { name: /Restarts/ })).toBeInTheDocument()
})
