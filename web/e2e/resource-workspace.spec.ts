import { expect, test } from '@playwright/test'

const generation = 'gen_workspace'
const budget = { cpuRequestMillicores: 500, cpuLimitMillicores: 1000, memoryRequestBytes: 134217728, memoryLimitBytes: 268435456 }
const metadata = { namespace: 'payments', name: 'api', uid: 'uid-api', resourceVersion: '17', creationTimestamp: '2026-10-01T00:00:00Z', labels: {} }
const spec = { name: 'api', image: 'example/api:1', ports: [], resources: budget, environment: [{ name: 'MODE', value: 'production' }, { name: 'TOKEN', value: null, source: { kind: 'Secret', name: 'credentials', key: 'token' } }], envFrom: [{ kind: 'ConfigMap', name: 'settings', prefix: 'APP_' }] }
const pod = { namespace: 'payments', name: 'api-with-a-long-name-abcdef', ready: { current: 1, desired: 1 }, restarts: 0, status: 'Running', node: 'worker-1', ip: '10.0.0.1', owner: { kind: 'Deployment', name: 'api' }, ageSeconds: 60, problematic: false, resources: budget, containerResources: { api: budget }, secrets: ['credentials'], configMaps: ['settings'] }
const yaml = 'apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: payments\n  uid: uid-api\n  resourceVersion: "17"\nspec:\n  replicas: 1\n'
const collection = { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }

test.beforeEach(async ({ context }) => {
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request(); const url = new URL(request.url()); const path = url.pathname
    let data: unknown = []; const meta = { generation, page: collection, coverage: null }
    if (path === '/api/v1/status') data = { version: 'test', components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((key) => [key, { status: key === 'metrics' ? 'unknown' : 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])), selection: { clusterProfileId: 1, context: 'development', cluster: 'dev', scopeId: 1, scopeMode: 'list', scopeName: 'Payments', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 1, generation } }
    else if (path === '/api/v1/preferences') data = { version: 1, ui: { language: 'en' }, logs: { wrap: false, timestamps: true, tailLines: 200 }, dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] }, filters: Object.fromEntries(['pods', 'workloads', 'events', 'logs'].map((name) => [name, { version: 1, items: [] }])) }
    else if (path === '/api/v1/namespace-scopes/1') data = { namespaces: ['payments'] }
    else if (path === '/api/v1/stream') { await route.fulfill({ status: 503, json: { code: 'AUTHORIZATION_UNAVAILABLE', message: 'Live updates unavailable.' } }); return }
    else if (path === '/api/v1/pods') data = [pod]
    else if (path === `/api/v1/pods/payments/${pod.name}`) data = { metadata: { ...metadata, name: pod.name }, summary: pod, conditions: [], containers: [{ spec, type: 'regular', ready: true, restartCount: 0, state: 'running' }], initContainers: [], ephemeralContainers: [], relatedEvents: [] }
    else if (path === '/api/v1/workloads') data = url.searchParams.getAll('kind').includes('replicasets') ? [] : [{ ...metadata, kind: 'Deployment', ready: 1, desired: 1, available: 1, updated: 1, status: 'Healthy', ageSeconds: 60 }]
    else if (path === '/api/v1/workloads/deployments/payments/api') data = { metadata, kind: 'Deployment', ready: 1, desired: 1, available: 1, updated: 1, status: 'Healthy', selector: {}, conditions: [], containers: [spec], resources: budget, secrets: ['credentials'], configMaps: ['settings'], related: [{ kind: 'Pod', namespace: 'payments', name: pod.name }] }
    else if (path.endsWith('/yaml') && request.method() === 'GET') { await route.fulfill({ status: 200, contentType: 'application/yaml', body: yaml }); return }
    else if (path.endsWith('/yaml') && request.method() === 'PUT') data = { accepted: true, action: 'updateDeployment', resourceVersion: '18', generation }
    else if (path === '/api/v1/permissions') data = { generation, complete: true, truncated: false, errors: [], decisions: url.searchParams.getAll('capability').map((capabilityId) => ({ capabilityId, namespace: 'payments', resourceName: 'api', decision: 'allowed' })) }
    else if (path === '/api/v1/session') data = { csrfToken: 'csrf_workspace', generation }
    else if (path === '/api/v1/hpas') data = [{ namespace: 'payments', name: 'api-hpa', targetKind: 'Deployment', targetName: 'api', resourceTargets: [{ resource: 'cpu', utilization: 60, averageValue: null }] }]
    else if (path === '/api/v1/metrics') data = { complete: true, truncated: false, errors: [], coverage: null, value: { collectedAt: '2026-10-06T12:00:00Z', windowSeconds: 30, pods: [{ namespace: 'payments', pod: pod.name, cpuMillicores: 350, memoryBytes: 249561088, containers: [{ name: 'api', cpuMillicores: 350, memoryBytes: 249561088 }] }], topCPU: [], topMemory: [] } }
    else if (path === '/api/v1/secrets/payments/credentials') data = { apiVersion: 'v1', kind: 'Secret', metadata: { ...metadata, name: 'credentials' } }
    else if (path === '/api/v1/secrets/payments/credentials/data') data = { metadata: { ...metadata, name: 'credentials' }, entries: [{ key: 'token', value: 'sensitive-fixture', encoding: 'utf-8', truncated: false }], truncated: false }
    else if (path === '/api/v1/configmaps/payments/settings') data = { metadata: { ...metadata, name: 'settings' }, entries: [{ key: 'MODE', value: 'production', encoding: 'utf-8', truncated: false }], truncated: false }
    await route.fulfill({ status: 200, json: { data, meta } })
  })
})

test('menus, common filters, HPA bars and resizing stay consistent', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/pods')
  const menu = page.getByRole('navigation', { name: 'Workloads resources', exact: true })
  await expect(menu.getByRole('link', { name: 'Pods', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.locator('aside').getByRole('link', { name: 'Pods', exact: true })).toHaveAttribute('aria-current', 'page')
  const cpu = page.getByRole('meter', { name: 'CPU', exact: true })
  await expect(cpu).toHaveAttribute('aria-valuetext', '350 m / 500 m · 70.0%')
  await expect(cpu.locator('..')).toHaveAttribute('data-tone', 'warning')
  await expect(page.getByRole('meter', { name: 'Memory', exact: true }).locator('..')).toHaveAttribute('data-tone', 'danger')
  await page.screenshot({ path: '/tmp/kubepeep-desktop.png', fullPage: true })
  for (const width of [900, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await expect(page.getByLabel('Search this bounded page')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    const font = await page.getByLabel('Search this bounded page').evaluate((element) => parseFloat(getComputedStyle(element).fontSize))
    expect(font).toBe(14)
    const name = await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).boundingBox()
    expect(name!.width).toBeGreaterThanOrEqual(160)
    expect(name!.height).toBeLessThan(70)
  }
  await page.screenshot({ path: '/tmp/kubepeep-mobile.png', fullPage: true })
  await menu.getByRole('link', { name: 'Deployments', exact: true }).click()
  await expect(page).toHaveURL(/workloads\/kind\/deployments$/)
  await expect(page.locator('aside').getByRole('link', { name: 'Deployments', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('region', { name: 'Resource list controls' })).toBeVisible()
})

test('Pod env references, ConfigMap entries and explicit Secret reveal work', async ({ page }) => {
  let secretReads = 0
  page.on('request', (request) => { if (request.url().endsWith('/secrets/payments/credentials/data')) secretReads++ })
  await page.goto('/pods')
  await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
  await expect(page.getByRole('heading', { name: 'Secrets', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'ConfigMaps', exact: true })).toBeVisible()
  await page.getByRole('tab', { name: 'Data / Env', exact: true }).click()
  await expect(page.getByText('production', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'ConfigMap: settings' }).click()
  await page.getByRole('tab', { name: 'Data', exact: true }).click()
  await page.getByText('MODE · utf-8', { exact: true }).click()
  await expect(page.getByText('production', { exact: true })).toBeVisible()
  await page.goto('/pods')
  await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
  await page.getByRole('tab', { name: 'Data / Env', exact: true }).click()
  await page.getByRole('button', { name: 'Secret: credentials / token' }).click()
  await page.getByRole('tab', { name: 'Data', exact: true }).click()
  expect(secretReads).toBe(0)
  await page.getByRole('button', { name: 'Reveal data' }).click()
  await page.getByText('token · utf-8', { exact: true }).click()
  await expect(page.getByText('sensitive-fixture', { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: 'Overview', exact: true }).click()
  await page.getByRole('tab', { name: 'Data', exact: true }).click()
  await expect(page.getByText('sensitive-fixture', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Reveal data' })).toBeVisible()
  expect(secretReads).toBe(1)
})

test('Deployment YAML saves with unchanged identity, version and CSRF', async ({ page }) => {
  await page.goto('/workloads/deployments/payments/api')
  await page.getByRole('tab', { name: 'YAML', exact: true }).click()
  await page.getByRole('button', { name: 'Load authorized YAML' }).click()
  await page.getByRole('button', { name: 'Edit YAML', exact: true }).click()
  await page.getByLabel('Deployment YAML', { exact: true }).fill(yaml.replace('replicas: 1', 'replicas: 2'))
  const request = page.waitForRequest((request) => request.method() === 'PUT' && request.url().endsWith('/yaml'))
  await page.getByRole('button', { name: 'Save Deployment', exact: true }).click()
  const saved = await request
  expect(saved.headers()['x-kubepeep-csrf']).toBe('csrf_workspace')
  expect(saved.postDataJSON()).toMatchObject({ expectedUid: 'uid-api', expectedResourceVersion: '17', expectedGeneration: generation, yaml: expect.stringContaining('replicas: 2') })
  await expect(page.getByText('Deployment saved. Load YAML to inspect the new version.')).toBeVisible()
  await page.getByRole('tab', { name: 'Logs', exact: true }).click()
  await page.getByRole('link', { name: 'Open workload logs', exact: true }).click()
  await expect(page).toHaveURL(/\/logs\?workload=/)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Logs', exact: true })).toBeVisible()
})
