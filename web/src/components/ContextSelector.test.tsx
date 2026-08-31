import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ContextSelector } from './ContextSelector'

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(status < 400 ? { data } : data), { status, headers: { 'Content-Type': 'application/json' } })
}

function renderSelector() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><ContextSelector selection={null} /></QueryClientProvider>)
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('context selector cancellation and states', () => {
  it('shows loading and a distinct offline state', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => undefined)))
    const view = renderSelector()
    expect(screen.getByRole('status')).toHaveTextContent('Loading kubeconfigs')
    view.unmount()

    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('offline')))
    renderSelector()
    expect(await screen.findByText('The local API is offline.')).toBeInTheDocument()
  })

  it('aborts the previous context listing when the kubeconfig changes', async () => {
    let firstAborted = false
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/cluster/profiles') {
        return Promise.resolve(json([
          { id: 1, name: 'Development', context: null, isDefault: true, kubeconfigFiles: [{ position: 0, displayPath: '~/.kube/development' }] },
          { id: 2, name: 'Staging', context: null, isDefault: false, kubeconfigFiles: [
            { position: 1, displayPath: '~/.kube/overrides' },
            { position: 0, displayPath: '~/.kube/staging' },
          ] },
        ]))
      }
      if (url.endsWith('clusterProfileId=1')) {
        return new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => {
            firstAborted = true
            reject(new DOMException('aborted', 'AbortError'))
          })
        })
      }
      if (url.endsWith('clusterProfileId=2')) {
        return Promise.resolve(json([{ clusterProfileId: 2, name: 'staging', cluster: 'stage-cluster', selected: false }]))
      }
      if (url === '/api/v1/session') {
        return Promise.resolve(json({ csrfToken: 'nonce', origin: 'http://127.0.0.1', generation: 'gen_1', expiresAt: '2099-01-01T00:00:00Z' }))
      }
      return Promise.resolve(json({ code: 'NOT_FOUND', message: 'missing' }, 404))
    })
    vi.stubGlobal('fetch', fetch)
    renderSelector()

    const profiles = await screen.findByRole('combobox', { name: 'Kubeconfig source' })
    expect(screen.getByRole('option', { name: '~/.kube/development' })).toBeInTheDocument()
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(expect.stringContaining('clusterProfileId=1'), expect.anything()))
    fireEvent.change(profiles, { target: { value: '2' } })

    expect(await screen.findByRole('option', { name: '~/.kube/staging + ~/.kube/overrides' })).toBeInTheDocument()
    expect(await screen.findByRole('option', { name: 'staging · stage-cluster' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Kubernetes context' })).toHaveAttribute('data-app-shortcut', 'context-selector')
    expect(screen.getByRole('combobox', { name: 'Kubernetes context' })).toHaveAttribute('aria-keyshortcuts', 'Control+O Meta+O')
    await waitFor(() => expect(firstAborted).toBe(true))
  })

  it('reports a kubeconfig whose selected context no longer exists', async () => {
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      const url = String(input)
      if (url === '/api/v1/cluster/profiles') {
        return Promise.resolve(json([{ id: 1, name: 'Development', context: 'removed-context', isDefault: true, kubeconfigFiles: [] }]))
      }
      if (url.startsWith('/api/v1/contexts?')) {
        return Promise.resolve(json([]))
      }
      return Promise.resolve(json({ csrfToken: 'nonce', generation: 'gen_1' }))
    }))
    renderSelector()

    expect(await screen.findByText('No contexts exist in this kubeconfig.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Select' })).toBeDisabled()
  })

  it('cancels a pending selection before submitting a newer context intention', async () => {
    let firstSelectionAborted = false
    let selectionCalls = 0
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/cluster/profiles') {
        return Promise.resolve(json([{ id: 1, name: 'Development', context: 'alpha', isDefault: true, kubeconfigFiles: [] }]))
      }
      if (url.startsWith('/api/v1/contexts?')) {
        return Promise.resolve(json([
          { clusterProfileId: 1, name: 'alpha', cluster: 'dev', selected: true },
          { clusterProfileId: 1, name: 'beta', cluster: 'dev', selected: false },
        ]))
      }
      if (url === '/api/v1/session') {
        return Promise.resolve(json({ csrfToken: 'nonce', origin: 'http://127.0.0.1', generation: 'gen_1', expiresAt: '2099-01-01T00:00:00Z' }))
      }
      if (url === '/api/v1/contexts/select') {
        selectionCalls += 1
        if (selectionCalls === 1) {
          return new Promise<Response>((_resolve, reject) => init?.signal?.addEventListener('abort', () => {
            firstSelectionAborted = true
            reject(new DOMException('aborted', 'AbortError'))
          }))
        }
        return Promise.resolve(json({ clusterProfileId: 1, context: 'beta', cluster: 'dev', scopeId: null, scopeName: null, scopeMode: null, scopeSource: 'none', defaultNamespace: null, namespaceCount: 0, generation: 'gen_2' }))
      }
      if (url === '/api/v1/status') {
        return Promise.resolve(json({}))
      }
      return Promise.resolve(json([]))
    })
    vi.stubGlobal('fetch', fetch)
    renderSelector()

    const contexts = await screen.findByRole('combobox', { name: 'Kubernetes context' })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Select' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'Select' }))
    await waitFor(() => expect(selectionCalls).toBe(1))
    fireEvent.change(contexts, { target: { value: 'beta' } })
    fireEvent.click(screen.getByRole('button', { name: /Select|Switching/ }))

    await waitFor(() => expect(firstSelectionAborted).toBe(true))
    await waitFor(() => expect(selectionCalls).toBe(2))
  })
})
