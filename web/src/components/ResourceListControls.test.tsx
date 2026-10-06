import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { ResourceListControls } from './ResourceListControls'

afterEach(() => { cleanup(); vi.useRealTimers() })
it('debounces string search, submits on Enter and cancels on unmount', async () => {
  vi.useFakeTimers()
  const apply = vi.fn(), change = vi.fn()
  const view = render(<ResourceListControls search="" appliedSearch="" onSearchChange={change} onApply={apply} />)
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
  fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'api' } })
  expect(change).toHaveBeenCalledWith('api')
  view.rerender(<ResourceListControls search="api" appliedSearch="" onSearchChange={change} onApply={apply} />)
  await act(() => vi.advanceTimersByTimeAsync(249))
  expect(apply).not.toHaveBeenCalled()
  await act(() => vi.advanceTimersByTimeAsync(1))
  expect(apply).toHaveBeenCalledTimes(1)
  view.rerender(<ResourceListControls search="other" appliedSearch="api" onSearchChange={change} onApply={apply} />)
  fireEvent.submit(screen.getByRole('form', { name: 'Resource list controls' }))
  expect(apply).toHaveBeenCalledTimes(2)
  view.unmount()
  await act(() => vi.advanceTimersByTimeAsync(500))
  expect(apply).toHaveBeenCalledTimes(2)
})
it('keeps the search shortcut without filter toolbars', () => {
  render(<ResourceListControls search="" appliedSearch="" onSearchChange={vi.fn()} onApply={vi.fn()} />)
  expect(screen.getByRole('searchbox')).toHaveAttribute('data-app-shortcut', 'search')
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  expect(screen.queryByText('More filters')).not.toBeInTheDocument()
})
