import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'

import { DataTable } from './DataTable'

it('keeps a 50k-row table DOM near the viewport', () => {
  const rows = Array.from({ length: 50_000 }, (_, index) => ({ name: `pod-${index}` }))
  render(<DataTable caption="Pods" rows={rows} getRowKey={(row) => row.name} columns={[{ key: 'name', header: 'Pod', cell: (row) => row.name }]} />)

  expect(screen.getByRole('table', { name: 'Pods' })).toHaveAttribute('aria-rowcount', '50001')
  expect(screen.getByText('pod-0')).toBeInTheDocument()
  expect(screen.getAllByRole('row').length).toBeLessThan(60)
  expect(screen.queryByText('pod-49999')).not.toBeInTheDocument()
})
