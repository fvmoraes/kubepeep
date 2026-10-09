import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GlobalNamespaceProvider, useGlobalNamespace } from './GlobalNamespace'
import { GlobalNamespaceSelect } from '../components/GlobalNamespaceSelect'

function Probe() {
  const namespace = useGlobalNamespace()
  return <output data-testid="effective">{namespace.ready ? namespace.value || 'All' : 'waiting'}</output>
}

function json(data: unknown) {
  return new Response(JSON.stringify({ data }), { headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

describe('default namespace selection', () => {
  it('selects the saved default before scope loading and restores it on context changes', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(json({ namespaces: ['alpha', 'production', 'staging'] }))))
    const tree = (generation: string, defaultNamespace: string) => <QueryClientProvider client={client}>
      <GlobalNamespaceProvider generation={generation} scopeId={1} scopeMode="list" defaultNamespace={defaultNamespace}>
        <Probe /><GlobalNamespaceSelect />
      </GlobalNamespaceProvider>
    </QueryClientProvider>
    const view = render(tree('prod', 'production'))
    expect(screen.getByTestId('effective')).toHaveTextContent('production')
    expect(screen.getByRole('combobox')).toHaveValue('production')
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled())
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['production', 'Namespace: All', 'alpha', 'staging'])
    fireEvent.change(screen.getByRole('combobox'), { target: { value: '' } })
    expect(screen.getByTestId('effective')).toHaveTextContent('All')
    view.rerender(tree('stage', 'staging'))
    expect(screen.getByTestId('effective')).toHaveTextContent('staging')
    expect(screen.getByRole('combobox')).toHaveValue('staging')
  })

  it('waits for a legacy scope without a default instead of implicitly selecting All', async () => {
    let resolve!: (value: Response) => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((done) => { resolve = done })))
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <GlobalNamespaceProvider generation="legacy" scopeId={1} scopeMode="list"><Probe /><GlobalNamespaceSelect /></GlobalNamespaceProvider>
    </QueryClientProvider>)
    expect(screen.getByTestId('effective')).toHaveTextContent('waiting')
    expect(screen.queryByRole('option', { name: 'Namespace: All' })).not.toBeInTheDocument()
    await waitFor(() => expect(resolve).toBeDefined())
    resolve(json({ namespaces: ['zeta', 'alpha'], defaultNamespace: 'zeta' }))
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveValue('zeta'))
  })

  it('keeps the saved default when namespace discovery is denied', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response('{}', { status: 403 }))))
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <GlobalNamespaceProvider generation="restricted" scopeId={1} scopeMode="list" defaultNamespace="payments"><Probe /><GlobalNamespaceSelect /></GlobalNamespaceProvider>
    </QueryClientProvider>)
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled())
    expect(screen.getByRole('combobox')).toHaveValue('payments')
    expect(screen.getByTestId('effective')).toHaveTextContent('payments')
  })
})
