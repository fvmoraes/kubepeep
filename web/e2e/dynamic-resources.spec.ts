import { expect, test, type Page } from '@playwright/test'
import type { DynamicResource, Preferences } from '../src/api/types'

const sql: DynamicResource = { group: 'sql.cnrm.cloud.google.com', version: 'v1beta1', resource: 'sqldatabases', kind: 'SQLDatabase', namespaced: true, shortNames: ['sqldb'] }
const node: DynamicResource = { group: '', version: 'v1', resource: 'nodes', kind: 'Node', namespaced: false, shortNames: ['no'] }
const emptyPreferences = (): Preferences => ({ version: 1, ui: { language: 'en' }, logs: { wrap: false, timestamps: true, tailLines: 200 }, dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] }, filters: { pods: { version: 1, items: [] }, workloads: { version: 1, items: [] }, events: { version: 1, items: [] }, logs: { version: 1, items: [] } }, columns: { hidden: {} }, customViews: [] })
const pageMeta = { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }
async function mockViews(page: Page) {
  const state = { preferences: emptyPreferences(), context: 'development', writes: [] as string[], inventory: [] as URL[], documents: 0 }
  await page.route('**/api/v1/**', async route => {
    const request = route.request(); const url = new URL(request.url()); const path = url.pathname
    const generation = `gen-${state.context}`
    let data: unknown = []
    if (path === '/api/v1/status') data = { version: 'test', selection: { clusterProfileId: 1, context: state.context, cluster: state.context, scopeId: 1, scopeMode: 'list', scopeName: 'Payments', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 2, generation }, components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map(key => [key, { status: 'healthy' }])) }
    else if (path === '/api/v1/preferences') { if (request.method() === 'PUT') state.preferences = request.postDataJSON(); data = state.preferences }
    else if (path === '/api/v1/session') data = { csrfToken: 'csrf', generation }
    else if (path === '/api/v1/namespace-scopes/1') data = { namespaces: ['payments', 'other'], defaultNamespace: 'payments' }
    else if (path === '/api/v1/resource-discovery') data = { resources: [sql, node], failures: [], truncated: false }
    else if (path.startsWith('/api/v1/dynamic-resources/')) {
      const parts = path.split('/'); const isNode = parts[6] === 'nodes'
      const item = { name: isNode ? 'worker-1' : 'billing', namespace: isNode ? undefined : 'payments', kind: isNode ? 'Node' : 'SQLDatabase', uid: 'u1', resourceVersion: 'rv1', ageSeconds: 180, columns: [{ name: 'Name', type: 'string', format: 'name' }, { name: 'Status', type: 'string' }, { name: 'Instance', type: 'string' }, { name: 'Message', type: 'string', priority: 1 }], cells: [isNode ? 'worker-1' : 'billing', 'Ready', 'production-db', 'Private networking enabled'] }
      if (path.endsWith('/yaml')) { state.documents++; data = { yaml: 'apiVersion: sql.cnrm.cloud.google.com/v1beta1\nkind: SQLDatabase\nmetadata:\n  name: billing\nspec:\n  value: c2VjcmV0\n', generation } }
      else if (parts.length === 8) { state.inventory.push(url); data = [item] } else data = item
    }
    else if (path === '/api/v1/pods') data = [{ name: 'api-with-a-very-long-resource-name', namespace: 'payments', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, containerCount: 1, containers: [{ name: 'api', state: 'running', ready: true }], node: 'worker-1', ageSeconds: 20 }]
    else if (path === '/api/v1/workloads') data = [{ name: 'api', namespace: 'payments', kind: 'Deployment', status: 'Healthy', ready: 1, desired: 1, available: 1, updated: 1, ageSeconds: 20 }]
    else if (path === '/api/v1/stream') { await route.fulfill({ status: 503, json: { code: 'CLUSTER_UNAVAILABLE', message: 'Fixture stream unavailable' } }); return }
    if (!['GET', 'HEAD'].includes(request.method())) state.writes.push(path)
    await route.fulfill({ json: { data, meta: { generation, page: pageMeta, coverage: null } } })
  })
  return state
}

test('discover, pin and inspect CRDs with printer columns and a fixed overlay', async ({ page }, testInfo) => {
  const state = await mockViews(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/workloads/custom?add=1')
  const dialog = page.getByRole('dialog', { name: 'Add custom resource' })
  await expect(dialog).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search available resources' }).fill('sqldb')
  await expect(dialog.getByRole('button', { name: 'Pin Node core v1' })).toHaveCount(0)
  await page.screenshot({ path: testInfo.outputPath('discovery-desktop.png') })
  await dialog.getByRole('button', { name: 'Pin SQLDatabase sql.cnrm.cloud.google.com v1beta1' }).click()
  await expect(page.getByRole('button', { name: 'Open SQLDatabase billing' })).toBeVisible()
  expect(state.preferences.customViews?.[0].items).toEqual([sql])
  expect(state.inventory.at(-1)?.searchParams.get('namespace')).toBe('payments')
  await expect(page.getByRole('columnheader', { name: 'Status', exact: true })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: 'Message', exact: true })).toHaveCount(0)
  const initial = (await page.locator('.resource-list-pane').boundingBox())!
  await page.getByRole('button', { name: 'Open SQLDatabase billing' }).click()
  await expect(page.getByRole('region', { name: 'SQLDatabase billing' })).toBeVisible()
  await expect(page.getByText('Private networking enabled', { exact: true })).toBeVisible()
  expect((await page.locator('.resource-detail-slot').boundingBox())!.height / 900).toBeCloseTo(.7, 2)
  expect((await page.locator('.resource-list-pane').boundingBox())!.height).toBeCloseTo(initial.height, 0)
  expect(state.documents).toBe(0)
  await page.screenshot({ path: testInfo.outputPath('custom-detail-desktop.png') })
  await page.getByRole('tab', { name: 'YAML', exact: true }).click()
  expect(state.documents).toBe(0)
  await page.getByRole('button', { name: 'Load authorized YAML' }).click()
  await expect(page.getByText('value: c2VjcmV0', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: /Apply|Edit YAML/ })).toHaveCount(0)
  await page.getByRole('button', { name: 'Close resource workspace' }).click()
  await expect(page.locator('.resource-detail-slot')).not.toBeVisible()
  await page.reload()
  await expect(page.getByRole('button', { name: 'Open SQLDatabase billing' })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('button', { name: 'Add custom resource', exact: true }).click()
  await expect(dialog).toBeInViewport({ ratio: .99 })
  await page.screenshot({ path: testInfo.outputPath('discovery-mobile.png') })
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  expect(state.writes.every(path => path === '/api/v1/preferences')).toBe(true)
})

test('pins native cluster resources, reorders views and isolates contexts', async ({ page }) => {
  const state = await mockViews(page)
  state.preferences.customViews = [{ clusterProfileId: 1, context: 'development', cluster: 'development', items: [sql, node] }]
  await page.goto('/workloads/custom')
  await page.getByRole('navigation', { name: 'Pinned resource views' }).getByRole('link', { name: 'Node core' }).click()
  await expect(page.getByRole('button', { name: 'Open Node worker-1' })).toBeVisible()
  expect(state.inventory.at(-1)?.searchParams.has('namespace')).toBe(false)
  await page.getByRole('button', { name: 'Manage Node core v1' }).click()
  await page.getByRole('button', { name: 'Move left', exact: true }).click()
  await expect.poll(() => state.preferences.customViews?.[0].items[0].kind).toBe('Node')
  await page.reload()
  await expect(page.getByRole('navigation', { name: 'Pinned resource views' }).getByRole('link').first()).toHaveText('Node core')
  await page.getByRole('button', { name: 'Manage Node core v1' }).click()
  await page.getByRole('button', { name: 'Remove view', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Open SQLDatabase billing' })).toBeVisible()
  expect(state.preferences.customViews?.[0].items).toEqual([sql])
  state.context = 'staging'
  await page.reload()
  await expect(page.getByText('Your resource views, in this context')).toBeVisible()
  expect(state.preferences.customViews?.[0].items).toEqual([sql])
  state.context = 'development'
  await page.reload()
  await expect(page.getByRole('button', { name: 'Open SQLDatabase billing' })).toBeVisible()
})

test('initial columns fit the viewport, then preserve visibility and drag order across reloads', async ({ page }, testInfo) => {
  const state = await mockViews(page)
  await page.goto('/pods')
  const headers = () => page.locator('table thead th[aria-label]').evaluateAll(elements => elements.map(element => element.getAttribute('aria-label')))
  await expect.poll(headers).toEqual(['Namespace', 'Pod', 'Status', 'Ready', 'Restarts', 'CPU', 'Memory', 'Type', 'Age'])
  for (const width of [1280, 1440, 1920]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.locator('.data-table').evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true)
  }
  await page.screenshot({ path: testInfo.outputPath('initial-pod-columns.png') })
  await page.setViewportSize({ width: 390, height: 844 })
  await expect.poll(headers).toEqual(['Namespace', 'Pod', 'Status'])
  expect(await page.locator('.data-table').evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('initial-pod-columns-mobile.png') })
  await page.setViewportSize({ width: 1440, height: 900 })
  await expect.poll(headers).toHaveLength(9)
  await page.getByRole('button', { name: 'Choose visible columns' }).click()
  await page.getByRole('checkbox', { name: 'Containers', exact: true }).check()
  await expect.poll(() => state.preferences.columns?.hidden.pods).toEqual(['node', 'owner', 'ip'])
  await page.keyboard.press('Escape')
  await page.getByRole('columnheader', { name: 'Age', exact: true }).dragTo(page.getByRole('columnheader', { name: 'Status', exact: true }))
  await expect.poll(() => state.preferences.columns?.order?.pods?.slice(0, 3)).toEqual(['namespace', 'name', 'age'])
  await page.reload()
  await expect.poll(headers).toEqual(['Namespace', 'Pod', 'Age', 'Status', 'Ready', 'Restarts', 'CPU', 'Memory', 'Type', 'Containers'])
  await page.goto('/workloads/kind/deployments')
  await expect.poll(headers).toEqual(['Namespace', 'Name', 'Status', 'Ready / Progress', 'Type', 'Age'])
  expect(state.preferences.columns?.order?.pods?.slice(0, 3)).toEqual(['namespace', 'name', 'age'])
})
