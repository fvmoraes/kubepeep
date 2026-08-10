import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { NamespaceScope, SelectionSummary } from '../api/client'
import { NamespaceScopeEditor, NamespaceScopeForm } from './NamespaceScopeEditor'

const selection: SelectionSummary = {
  clusterProfileId: 7,
  context: 'development',
  cluster: 'dev-cluster',
  scopeId: null,
  scopeName: null,
  scopeMode: null,
  scopeSource: 'none',
  defaultNamespace: null,
  namespaceCount: 0,
  generation: 'gen_42',
}

function renderForm(csrfToken: string | null = null) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><NamespaceScopeForm selection={selection} csrfToken={csrfToken} /></QueryClientProvider>)
}

function counter(label: string): HTMLElement {
  return screen.getByText(label).parentElement!
}

const activeScope: NamespaceScope = {
  id: 1, clusterProfileId: 7, name: 'Active', context: 'development', mode: 'list', namespaces: ['payments', 'billing'],
  defaultNamespace: 'payments', version: 3, createdAt: '2026-08-10T12:00:00Z', updatedAt: '2026-08-10T12:00:00Z',
}

const otherScope: NamespaceScope = {
  id: 2, clusterProfileId: 7, name: 'Other', context: 'development', mode: 'single', namespaces: ['invoices'],
  defaultNamespace: 'invoices', version: 5, createdAt: '2026-08-10T12:00:00Z', updatedAt: '2026-08-10T12:00:00Z',
}

function apiJSON(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(status < 400 ? { data } : data), { status, headers: { 'Content-Type': 'application/json' } })
}

function editorStatus() {
  return {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: 'healthy', code: 'TEST', message: 'test', checkedAt: null }])),
    selection: { ...selection, scopeId: 1, scopeName: 'Active', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 2 },
  }
}

function renderEditor() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><NamespaceScopeEditor /></QueryClientProvider>)
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('namespace scope editor', () => {
  it('shows profile/context, chips, and all four independent counters', () => {
    renderForm()
    fireEvent.change(screen.getByRole('textbox', { name: 'Namespace input' }), {
      target: { value: '["payments","","payments","Bad_Name","billing"]' },
    })

    expect(screen.getByRole('textbox', { name: 'Scope cluster profile' })).toHaveValue('7')
    expect(screen.getByRole('textbox', { name: 'Scope context' })).toHaveValue('development')
    expect(counter('valid')).toHaveTextContent('2')
    expect(counter('duplicates removed')).toHaveTextContent('1')
    expect(counter('empty removed')).toHaveTextContent('1')
    expect(counter('invalid')).toHaveTextContent('1')
    expect(screen.getByRole('button', { name: 'Remove namespace payments' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove invalid namespace Bad_Name' })).toBeInTheDocument()
  })

  it('supports single, list, and all semantics without ever showing a wildcard item', () => {
    renderForm()
    const input = screen.getByRole('textbox', { name: 'Namespace input' })
    fireEvent.change(input, { target: { value: 'payments billing' } })
    expect(screen.getByRole('alert')).toHaveTextContent('exactly one namespace')

    fireEvent.click(screen.getByRole('radio', { name: 'list' }))
    expect(screen.queryByText(/exactly one namespace/)).not.toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Default namespace' })).toHaveValue('payments')

    fireEvent.click(screen.getByRole('radio', { name: 'all' }))
    expect(screen.queryByRole('textbox', { name: 'Namespace input' })).not.toBeInTheDocument()
    expect(screen.getByRole('note')).toHaveTextContent('stores no wildcard')
    expect(screen.queryByText('*')).not.toBeInTheDocument()
    expect(counter('valid')).toHaveTextContent('0')
  })

  it('removes individual chips and clears the entire input', () => {
    renderForm()
    const input = screen.getByRole('textbox', { name: 'Namespace input' })
    fireEvent.change(input, { target: { value: 'payments billing invoices' } })

    fireEvent.click(screen.getByRole('button', { name: 'Remove namespace billing' }))
    expect(input).toHaveValue('payments\ninvoices')
    expect(screen.queryByText('billing')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Clear' }))
    expect(input).toHaveValue('')
    expect(counter('valid')).toHaveTextContent('0')
  })

  it('surfaces malformed structured input and keeps save fail-closed', () => {
    renderForm()
    fireEvent.change(screen.getByRole('textbox', { name: 'Namespace input' }), { target: { value: '["payments",]' } })
    fireEvent.change(screen.getByRole('textbox', { name: 'Scope name' }), { target: { value: 'Broken scope' } })

    expect(screen.getByRole('alert')).toHaveTextContent('JSON namespace input is malformed')
    expect(screen.getByRole('button', { name: 'Save scope' })).toBeDisabled()
  })

  it('validates the complete raw input in one no-store server request', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: {
      valid: ['payments'], validCount: 1, duplicateCount: 2, discardedEmptyCount: 3,
      invalid: [{ input: 'Bad_Name', code: 'INVALID_NAMESPACE_NAME' }], invalidCount: 1,
      existence: { checked: false, reasonCode: 'NAMESPACE_LIST_FORBIDDEN' },
    } }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetch)
    renderForm('ephemeral-token')
    fireEvent.change(screen.getByRole('textbox', { name: 'Namespace input' }), { target: { value: 'payments' } })

    fireEvent.click(screen.getByRole('button', { name: 'Validate with cluster' }))

    expect(await screen.findByText('Existence was not checked: NAMESPACE_LIST_FORBIDDEN.')).toBeInTheDocument()
    expect(counter('duplicates removed')).toHaveTextContent('2')
    expect(counter('empty removed')).toHaveTextContent('3')
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledWith('/api/v1/namespace-scopes/validate', expect.objectContaining({
      method: 'POST', cache: 'no-store', credentials: 'same-origin',
      headers: expect.objectContaining({ 'X-KubePeep-CSRF': 'ephemeral-token' }),
    }))
  })

  it('updates an existing scope with version and generation preconditions', async () => {
    const fetch = vi.fn().mockResolvedValue(apiJSON({ ...activeScope, name: 'Updated', version: 4 }))
    vi.stubGlobal('fetch', fetch)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(<QueryClientProvider client={client}>
      <NamespaceScopeForm selection={{ ...selection, scopeId: 1, scopeName: 'Active', scopeMode: 'list', scopeSource: 'saved', namespaceCount: 2 }} scope={activeScope} csrfToken="csrf_edit" />
    </QueryClientProvider>)

    fireEvent.change(screen.getByRole('textbox', { name: 'Scope name' }), { target: { value: 'Updated' } })
    fireEvent.click(screen.getByRole('button', { name: 'Update scope' }))

    expect(await screen.findByText('Scope “Updated” was updated.')).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledWith('/api/v1/namespace-scopes/1', expect.objectContaining({
      method: 'PUT',
      headers: expect.objectContaining({ 'X-KubePeep-CSRF': 'csrf_edit' }),
      body: expect.stringContaining('"version":3'),
    }))
    expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual(expect.objectContaining({ expectedGeneration: 'gen_42', name: 'Updated' }))
  })

  it('selects a saved scope with the current generation precondition', async () => {
    let selectBody: unknown
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(apiJSON(editorStatus()))
      if (path === '/api/v1/session') return Promise.resolve(apiJSON({ csrfToken: 'csrf_scope', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-10T13:00:00Z' }))
      if (path === '/api/v1/namespace-scopes?limit=100') return Promise.resolve(apiJSON([activeScope, otherScope]))
      if (path === '/api/v1/namespace-scopes/2/select') {
        selectBody = JSON.parse(String(init?.body))
        return Promise.resolve(apiJSON({ ...editorStatus().selection, scopeId: 2, scopeName: 'Other', generation: 'gen_43' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetch)
    renderEditor()

    const select = await screen.findByRole('button', { name: 'Select Other' })
    await waitFor(() => expect(select).toBeEnabled())
    fireEvent.click(select)

    await waitFor(() => expect(fetch.mock.calls.some(([input]) => String(input) === '/api/v1/namespace-scopes/2/select')).toBe(true))
    expect(selectBody).toEqual({ expectedGeneration: 'gen_42' })
  })

  it('requires an explicit replacement before deleting the active scope', async () => {
    let deleteBody: unknown
    const fetch = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/v1/status') return Promise.resolve(apiJSON(editorStatus()))
      if (path === '/api/v1/session') return Promise.resolve(apiJSON({ csrfToken: 'csrf_scope', origin: 'http://127.0.0.1:2748', generation: 'gen_42', expiresAt: '2026-08-10T13:00:00Z' }))
      if (path === '/api/v1/namespace-scopes?limit=100') return Promise.resolve(apiJSON([activeScope, otherScope]))
      if (path === '/api/v1/namespace-scopes/1') {
        deleteBody = JSON.parse(String(init?.body))
        return Promise.resolve(apiJSON({ ...editorStatus().selection, scopeId: 2, scopeName: 'Other', generation: 'gen_43' }))
      }
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetch)
    renderEditor()

    const deleteButton = await screen.findByRole('button', { name: 'Delete Active' })
    await waitFor(() => expect(deleteButton).toBeEnabled())
    fireEvent.click(deleteButton)
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Confirm delete Active' })).toBeDisabled()

    fireEvent.change(screen.getByRole('combobox', { name: 'Replacement scope' }), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Confirm delete Active' }))

    await waitFor(() => expect(deleteBody).toEqual({ confirmed: true, version: 3, replacementScopeId: 2, expectedGeneration: 'gen_42' }))
  })
})
