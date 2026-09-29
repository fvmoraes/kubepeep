import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { PanelErrorBoundary } from './PanelErrorBoundary'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('keeps another panel usable after a render failure and retries only the failed panel', () => {
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  let failed = true
  const onRetry = vi.fn(() => { failed = false })
  function Metrics() {
    if (failed) throw new Error('render failed')
    return <p>Metrics recovered</p>
  }

  render(<>
    <PanelErrorBoundary name="Metrics" onRetry={onRetry}><Metrics /></PanelErrorBoundary>
    <button type="button">Use logs</button>
  </>)

  expect(screen.getByRole('alert')).toHaveTextContent('Metrics could not be displayed')
  expect(screen.getByRole('button', { name: 'Use logs' })).toBeEnabled()
  fireEvent.click(screen.getByRole('button', { name: 'Retry Metrics' }))
  expect(onRetry).toHaveBeenCalledOnce()
  expect(screen.getByText('Metrics recovered')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Use logs' })).toBeEnabled()
})
