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
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select all loaded rows' }))
  expect(toggle).toHaveBeenLastCalledWith(true, [rows[1], rows[2]])
  fireEvent.click(screen.getByRole('button', { name: 'Choose visible columns' }))
  fireEvent.click(screen.getByRole('checkbox', { name: 'Restarts' }))
  expect(screen.queryByRole('columnheader', { name: /Restarts/ })).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Reset columns' }))
  expect(screen.getByRole('columnheader', { name: /Restarts/ })).toBeInTheDocument()
})

it('refreshes filter choices and row identities when a loaded page changes', () => {
  type Row = { name: string; status: string }
  const columns = [
    { key: 'name', header: 'Name', cell: (row: Row) => row.name },
    { key: 'status', header: 'Status', cell: (row: Row) => row.status },
  ]
  const getRowKey = (row: Row) => row.name
  const toggle = vi.fn()
  const props = { columns, getRowKey, selectable: true, onToggleRow: toggle }
  const { rerender } = render(<DataTable {...props} rows={[{ name: 'api', status: 'Running' }, { name: 'worker', status: 'Pending' }]} />)
  fireEvent.click(screen.getByRole('button', { name: 'Filter Status' }))
  fireEvent.click(screen.getByRole('checkbox', { name: 'Pending' }))
  expect(screen.queryByRole('checkbox', { name: 'Select row worker' })).not.toBeInTheDocument()

  rerender(<DataTable {...props} rows={[{ name: 'web', status: 'Failed' }, { name: 'api', status: 'Running' }]} />)
  expect(screen.queryByRole('checkbox', { name: 'Pending' })).not.toBeInTheDocument()
  expect(screen.getByRole('checkbox', { name: 'Failed' })).toBeChecked()
  fireEvent.click(screen.getByRole('checkbox', { name: 'Failed' }))
  expect(screen.queryByRole('checkbox', { name: 'Select row web' })).not.toBeInTheDocument()
  fireEvent.keyDown(document, { key: 'Escape' })
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select row api' }))
  expect(toggle).toHaveBeenLastCalledWith('api', true)
})

it('orders extra columns by displayed header after common columns and honors a custom order', () => {
  const columns = [
    { key: 'first-extra', header: <span>Zulu</span>, cell: () => 'z' },
    { key: 'age', header: 'Age', cell: () => '1h' },
    { key: 'second-extra', header: <strong>Alpha</strong>, cell: () => 'a' },
    { key: 'name', header: 'Name', cell: () => 'api' },
    { key: 'third-extra', header: 'beta', cell: () => 'b' },
  ]
  const props = { columns, rows: [{ name: 'api' }] }
  const { rerender } = render(<DataTable {...props} />)
  const headers = () => screen.getAllByRole('columnheader').map(header => header.getAttribute('aria-label')).filter(Boolean)
  expect(headers()).toEqual(['Name', 'Age', 'Alpha', 'beta', 'Zulu'])
  rerender(<DataTable {...props} columnVisibility={{ hidden: [], order: ['first-extra', 'name', 'age', 'third-extra', 'second-extra'], toggle: vi.fn(), reset: vi.fn() }} />)
  expect(headers()).toEqual(['Zulu', 'Name', 'Age', 'beta', 'Alpha'])
})
