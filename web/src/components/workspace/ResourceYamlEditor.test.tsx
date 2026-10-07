import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { SelectionSummary } from '../../api/types'
import { ResourceYamlEditor } from './ResourceYamlEditor'

const selection = { clusterProfileId: 1, context: 'dev', generation: 'gen_yaml' } as SelectionSummary
const json = (data: unknown, status = 200) => new Response(JSON.stringify(status === 200 ? { data } : data), { status, headers: { 'Content-Type': 'application/json' } })
function setup(collection = 'pods', kind = 'Pod', namespace: string | null = 'payments', allowed = true, saveStatus = 200) {
  const document = JSON.stringify({ apiVersion: 'v1', kind, metadata: { name: 'sample', ...(namespace ? { namespace } : {}), uid: 'uid-sample', resourceVersion: '17' }, data: { key: 'original-value' } }, null, 2)
  const requests: { path: string; init?: RequestInit }[] = []
  const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input); requests.push({ path, init })
    if (path.startsWith('/api/v1/permissions')) return json({ generation: selection.generation, complete: true, errors: [], decisions: [{ capabilityId: `yaml.${collection}.update`, namespace: namespace ?? '', resourceName: 'sample', decision: allowed ? 'allowed' : 'denied' }] })
    if (path === '/api/v1/session') return json({ csrfToken: 'csrf-yaml', generation: selection.generation })
    if (path.endsWith('/yaml')) return init?.method === 'PUT'
      ? json(saveStatus === 200 ? { accepted: true, resourceVersion: '18' } : { code: saveStatus === 409 ? 'CONFLICT' : 'VALIDATION_FAILED', message: 'Rejected' }, saveStatus)
      : json({ yaml: document, kind, generation: selection.generation, updateCapability: `yaml.${collection}.update` })
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(<QueryClientProvider client={client}><ResourceYamlEditor collection={collection} namespace={namespace} name="sample" selection={selection} /></QueryClientProvider>)
  return { document, requests, client, view, fetchMock }
}
afterEach(() => { cleanup(); vi.unstubAllGlobals() })

it.each([['pods', 'Pod', 'payments'], ['configmaps', 'ConfigMap', 'payments'], ['secrets', 'Secret', 'payments'], ['nodes', 'Node', null], ['cluster-roles', 'ClusterRole', null]])('edits and saves %s with exact authorization and optimistic preconditions', async (collection, kind, namespace) => {
  const { requests, document, client } = setup(collection!, kind!, namespace)
  expect(requests.some(({ path }) => path.endsWith('/yaml'))).toBe(false)
  fireEvent.click(screen.getByRole('button', { name: 'Load authorized YAML' }))
  const edit = await screen.findByRole('button', { name: 'Edit YAML' })
  await waitFor(() => expect(edit).toBeEnabled())
  fireEvent.click(edit)
  fireEvent.change(screen.getByLabelText(`${kind} YAML`, { exact: true }), { target: { value: document.replace('original-value', 'edited-value') } })
  fireEvent.click(screen.getByRole('button', { name: `Save ${kind}` }))
  expect(await screen.findByText(`${kind} saved. Load YAML to inspect the new version.`)).toBeVisible()
  const saved = requests.find(({ init }) => init?.method === 'PUT')!
  expect(saved.path).toBe(`/api/v1/resources/${collection}/${namespace ? namespace + '/' : ''}sample/yaml`)
  expect(saved.init?.headers).toMatchObject({ 'X-KubePeep-CSRF': 'csrf-yaml' })
  expect(JSON.parse(String(saved.init?.body))).toMatchObject({ confirmed: true, action: 'updateResource', consequenceCode: 'UPDATE_RESOURCE', expectedGeneration: 'gen_yaml', expectedUid: 'uid-sample', expectedResourceVersion: '17', target: { namespace: namespace ?? '', kind, name: 'sample' }, yaml: expect.stringContaining('edited-value') })
  expect(JSON.stringify(client.getQueryCache().getAll().map((query) => query.state.data))).not.toContain('original-value')
  expect(client.getMutationCache().getAll()).toHaveLength(0)
})

it.each([400, 409])('preserves the draft after a %s response', async (status) => {
  const { document } = setup('pods', 'Pod', 'payments', true, status)
  fireEvent.click(screen.getByRole('button', { name: 'Load authorized YAML' }))
  const edit = await screen.findByRole('button', { name: 'Edit YAML' })
  await waitFor(() => expect(edit).toBeEnabled()); fireEvent.click(edit)
  const draft = document.replace('original-value', 'unsaved-value')
  fireEvent.change(screen.getByLabelText('Pod YAML'), { target: { value: draft } })
  fireEvent.click(screen.getByRole('button', { name: 'Save Pod' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(/draft is preserved/i)
  expect(screen.getByLabelText('Pod YAML')).toHaveValue(draft)
  expect(screen.getByRole('button', { name: 'Save Pod' })).toBeEnabled()
})

it('blocks changed identity and invalid documents before sending any write', async () => {
  const { document, requests } = setup()
  fireEvent.click(screen.getByRole('button', { name: 'Load authorized YAML' }))
  const edit = await screen.findByRole('button', { name: 'Edit YAML' })
  await waitFor(() => expect(edit).toBeEnabled()); fireEvent.click(edit)
  for (const draft of ['spec: [invalid', document.replace('uid-sample', 'another-uid')]) {
    fireEvent.change(screen.getByLabelText('Pod YAML'), { target: { value: draft } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Pod' }))
    await screen.findByRole('alert')
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save Pod' })).toBeEnabled())
  }
  expect(requests.some(({ init }) => init?.method === 'PUT')).toBe(false)
})

it('keeps authorized reads available when updating is denied', async () => {
  setup('secrets', 'Secret', 'payments', false)
  fireEvent.click(screen.getByRole('button', { name: 'Load authorized YAML' }))
  expect(await screen.findByRole('button', { name: 'Edit YAML' })).toBeDisabled()
  expect(screen.getByText(/Read only/)).toBeVisible()
  expect(screen.queryByRole('textbox', { name: 'Secret YAML' })).not.toBeInTheDocument()
})

it('aborts an in-flight YAML read on unmount and leaves no payload in query caches', async () => {
  const { fetchMock, view, client } = setup('secrets', 'Secret')
  let signal: AbortSignal | null | undefined
  fetchMock.mockImplementationOnce(async (_input, init) => {
    signal = init?.signal
    return new Promise<Response>(() => undefined)
  })
  fireEvent.click(screen.getByRole('button', { name: 'Load authorized YAML' }))
  await waitFor(() => expect(signal).toBeDefined())
  view.unmount()
  expect(signal?.aborted).toBe(true)
  expect(JSON.stringify(client.getQueryCache().getAll().map((query) => query.state.data))).not.toContain('original-value')
})
