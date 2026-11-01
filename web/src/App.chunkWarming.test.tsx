import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

// Hoisted before App is imported, so Shell receives the spy rather than the
// real warm. App.test.tsx cannot do this: it imports App statically.
const warmSectionChunk = vi.fn()
vi.mock('./navigation/sectionChunks', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./navigation/sectionChunks')>()),
  warmSectionChunk,
}))

const { App } = await import('./App')

function renderAtRoute(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  warmSectionChunk.mockClear()
})

// The route chunk has no data dependency, but the Outlet that mounts it sits
// behind the scope and namespace gates. The behaviour-preservation claim has
// two halves: the download must start while the gate is still blocking, and
// the section must still not mount until the gate clears. Until now both
// rested on reading the code.
describe('route chunk warming alongside the render gates', () => {
  it('asks for the section chunk while bootstrap is still pending', async () => {
    // Status never resolves, so every gate below stays closed for the whole
    // test and nothing can mount behind it.
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => undefined)))
    renderAtRoute('/storage/persistent-volume-claims')

    await waitFor(() => expect(warmSectionChunk).toHaveBeenCalledWith('/storage/persistent-volume-claims'))

    // The shell chrome is up, which is what makes the warm possible this
    // early, but the section itself never reached the document.
    expect(screen.getByRole('navigation', { name: 'Primary navigation' })).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.queryByRole('grid')).not.toBeInTheDocument()
  })

  it('re-asks when the route changes, not only on first mount', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => undefined)))
    const { unmount } = renderAtRoute('/logs')
    await waitFor(() => expect(warmSectionChunk).toHaveBeenCalledWith('/logs'))
    unmount()

    warmSectionChunk.mockClear()
    renderAtRoute('/settings')
    await waitFor(() => expect(warmSectionChunk).toHaveBeenCalledWith('/settings'))
    expect(warmSectionChunk).not.toHaveBeenCalledWith('/logs')
  })

  it('asks for nothing when the path is outside the route table', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => undefined)))
    renderAtRoute('/not-a-route')
    await waitFor(() => expect(warmSectionChunk).toHaveBeenCalledWith('/not-a-route'))
    // The map answers null for an unknown path, which sectionChunks.test.ts
    // pins; Shell still reports the route rather than guessing a chunk.
  })
})
