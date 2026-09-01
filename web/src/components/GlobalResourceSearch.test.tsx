import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { CommandCenter, type CommandRoute } from './CommandCenter'

const routes: CommandRoute[] = [
  { path: '/pods', label: 'Pods', description: 'Pod inventory' },
]

const resources: CommandRoute[] = [
  { path: '/pods/payments/api-abc', label: 'api-abc', description: 'Pod · payments', keywords: ['Pod', 'payments', 'pods'] },
  { path: '/config/secrets/ops/store', label: 'store', description: 'Secret · ops', keywords: ['Secret', 'ops', 'secrets'] },
]

function renderPalette(getResources: () => readonly CommandRoute[]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/pods']}>
        <CommandCenter routes={routes} getResources={getResources} />
        <Routes>
          <Route path="/pods" element={<div>pods page</div>} />
          <Route path="/pods/:namespace/:name" element={<div>pod detail</div>} />
          <Route path="/config/secrets/:namespace/:name" element={<div>secret detail</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('command center global resource search (F7-04)', () => {
  it('resolves resource entries only when the palette opens and navigates on Enter', async () => {
    const getResources = vi.fn(() => resources)
    renderPalette(getResources)
    expect(getResources).not.toHaveBeenCalled()

    fireEvent.keyDown(document, { key: 'k', ctrlKey: true })
    expect(getResources).toHaveBeenCalledTimes(1)

    const input = screen.getByRole('combobox', { name: 'Search application pages' })
    fireEvent.change(input, { target: { value: 'api-abc' } })
    const option = screen.getByRole('option', { name: /api-abc/ })
    expect(option).toHaveAttribute('aria-selected', 'true')
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(screen.getByText('pod detail')).toBeInTheDocument()
  })

  it('searches identifiers of loaded resources without opening network requests', () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    renderPalette(() => resources)
    fireEvent.keyDown(document, { key: 'k', ctrlKey: true })
    fireEvent.change(screen.getByRole('combobox', { name: 'Search application pages' }), { target: { value: 'payments pod' } })
    expect(screen.getByRole('option', { name: /api-abc/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /store/ })).not.toBeInTheDocument()
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})
