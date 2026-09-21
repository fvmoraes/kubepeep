import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { PanelErrorBoundary } from './PanelErrorBoundary'

it('contains a render crash and restores only the failed panel on retry', () => {
  let shouldCrash = true
  const onRetry = vi.fn(() => { shouldCrash = false })
  const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
  function Panel() {
    if (shouldCrash) throw new Error('broken metrics chart')
    return <p>metrics restored</p>
  }
  try {
    render(<><p>summary remains visible</p><PanelErrorBoundary name="Pod metrics" onRetry={onRetry}><Panel /></PanelErrorBoundary></>)
    expect(screen.getByText('summary remains visible')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('Pod metrics could not be displayed')
    fireEvent.click(screen.getByRole('button', { name: 'Retry Pod metrics' }))
    expect(onRetry).toHaveBeenCalledOnce()
    expect(screen.getByText('metrics restored')).toBeInTheDocument()
  } finally {
    consoleError.mockRestore()
  }
})
