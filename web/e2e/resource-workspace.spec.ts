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
    else if (path.endsWith('/logs/stream')) { await route.fulfill({ contentType: 'text/event-stream', body: `event: meta\ndata: {"generation":"${generation}"}\n\nevent: line\ndata: {"text":"workload log fixture"}\n\n` }); return }
    else if (path.endsWith('/logs')) data = { lines: [{ timestamp: null, text: 'workload log fixture', truncated: false }], truncated: false }
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
    await expect(page.getByLabel('Search resources')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    const font = await page.getByLabel('Search resources').evaluate((element) => parseFloat(getComputedStyle(element).fontSize))
    expect(font).toBe(14)
    const name = await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).boundingBox()
    expect(name!.width).toBeGreaterThanOrEqual(160)
    expect(name!.height).toBeLessThanOrEqual(30)
  }
  await page.screenshot({ path: '/tmp/kubepeep-mobile.png', fullPage: true })
  await menu.getByRole('link', { name: 'Deployments', exact: true }).click()
  await expect(page).toHaveURL(/workloads\/kind\/deployments$/)
  await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
  await expect(page.locator('aside').getByRole('link', { name: 'Deployments', exact: true })).toHaveAttribute('aria-current', 'page')
  await page.getByRole('button', { name: 'Close navigation', exact: true }).click()
  await expect(page.getByRole('form', { name: 'Resource list controls' })).toBeVisible()
})

test.describe('touch navigation', () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })

  test('keeps focus in the menu, closes after navigation and releases scrolling on resize', async ({ page }) => {
    await page.goto('/pods')
    await expect(page.getByRole('table', { name: 'Authorized Pod pages' })).toBeVisible()
    const trigger = page.getByRole('button', { name: 'Open navigation', exact: true })
    const menu = page.getByRole('dialog', { name: 'Application navigation' })
    await trigger.tap()
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('button', { name: 'Close navigation', exact: true })).toBeFocused()
    await page.keyboard.press('Shift+Tab')
    await expect(trigger).not.toBeFocused()
    await page.keyboard.press('Tab')
    expect(await menu.evaluate((element) => element.contains(document.activeElement))).toBe(true)
    await page.keyboard.press('Escape')
    await expect(menu).not.toBeVisible()
    await expect(trigger).toBeFocused()
    await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).tap()
    const resource = page.getByRole('region', { name: `Pod ${pod.name}` })
    await expect(resource).toBeVisible()
    await trigger.tap()
    await page.keyboard.press('Escape')
    await expect(menu).not.toBeVisible()
    await expect(resource).toBeVisible()
    await expect(page).toHaveURL(new RegExp(`/pods/payments/${pod.name}$`))
    await resource.getByRole('button', { name: 'Close resource workspace' }).tap()
    await trigger.tap()
    await menu.getByRole('link', { name: 'Deployments', exact: true }).tap()
    await expect(page).toHaveURL(/\/workloads\/kind\/deployments$/)
    await expect(menu).not.toBeVisible()
    await expect(page.locator('body')).not.toHaveCSS('overflow', 'hidden')
    await trigger.tap()
    await page.setViewportSize({ width: 1024, height: 768 })
    await expect(menu).toHaveCount(0)
    await expect(page.locator('body')).not.toHaveCSS('overflow', 'hidden')
    await expect(page.locator('aside').getByRole('link', { name: 'Deployments', exact: true })).toHaveAttribute('aria-current', 'page')
    await page.setViewportSize({ width: 320, height: 568 })
    await page.getByRole('button', { name: 'Choose visible columns', exact: true }).tap()
    const columns = page.getByRole('dialog', { name: 'Choose visible columns', exact: true })
    await expect(columns).toBeVisible()
    const rect = await columns.boundingBox()
    expect(rect!.x).toBeGreaterThanOrEqual(0)
    expect(rect!.x + rect!.width).toBeLessThanOrEqual(320)
    expect(rect!.y + rect!.height).toBeLessThanOrEqual(568)
    await page.keyboard.press('Escape')
    await page.getByRole('button', { name: 'Open command center' }).tap()
    const commands = page.getByRole('dialog', { name: 'Command center' })
    await expect(commands).toBeVisible()
    await expect(commands.getByRole('button', { name: 'Close command center' })).toBeInViewport()
  })
})

test('adapts lists and details to screen height without losing content or reduced-motion preferences', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/pods')
  for (const [width, height] of [[320, 568], [768, 1024], [1024, 600], [1920, 1080], [2560, 1440]]) {
    await page.setViewportSize({ width, height })
    await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
    const panel = page.getByRole('region', { name: `Pod ${pod.name}` })
    await expect(panel).toBeVisible()
    await expect(panel).toHaveCSS('animation-name', 'none')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    const bounds = await panel.boundingBox()
    expect(bounds!.height).toBeLessThanOrEqual(height * 0.72 + 1)
    await expect(panel.getByRole('button', { name: 'Close resource workspace' })).toBeInViewport()
    await panel.getByRole('tab', { name: 'Logs', exact: true }).click()
    await expect(panel.getByLabel('Log output', { exact: true })).toContainText('workload log fixture')
    await expect(panel.getByRole('button', { name: 'Close resource workspace' })).toBeInViewport()
    await panel.getByRole('button', { name: 'Close resource workspace' }).click()
  }
})

test('uses restrained initial loading and preserves rows during a slow automatic refresh', async ({ page }) => {
  await page.clock.install()
  let first = true
  let requests = 0
  let releaseInitial!: () => void
  let releaseRefresh!: () => void
  const initial = new Promise<void>((resolve) => { releaseInitial = resolve })
  const refresh = new Promise<void>((resolve) => { releaseRefresh = resolve })
  await page.route('**/api/v1/pods?*', async (route) => {
    const initialRequest = first
    first = false
    requests++
    await (initialRequest ? initial : refresh)
    await route.fulfill({ json: { data: [{ ...pod, restarts: initialRequest ? 0 : 7 }], meta: { generation, page: collection } } })
  })
  await page.goto('/pods')
  const loading = page.getByRole('status', { name: 'Loading resources…', exact: true })
  await expect(loading).toBeVisible()
  await expect(loading.locator('.loading-feedback')).toHaveCSS('opacity', '1')
  await expect(loading).toHaveAttribute('aria-busy', 'true')
  releaseInitial()
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table).toBeVisible()
  await expect(loading).toHaveCount(0)
  const row = table.locator('tbody tr')
  await expect(row.getByRole('cell', { name: '0', exact: true })).toBeVisible()
  await page.clock.fastForward(16_000)
  await expect.poll(() => requests).toBeGreaterThan(1)
  await expect(table).toBeVisible()
  await expect(row.getByRole('cell', { name: '0', exact: true })).toBeVisible()
  await expect(loading).toHaveCount(0)
  releaseRefresh()
  await expect(row.getByRole('cell', { name: '7', exact: true })).toBeVisible()
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
  await expect(page.getByLabel('Log output', { exact: true })).toContainText('workload log fixture')
  await expect(page.locator('.workspace-panel').getByRole('alert')).toHaveCount(0)
  await expect(page).toHaveURL(/\/workloads\/deployments\/payments\/api$/)
  await expect(page.getByRole('region', { name: 'Deployment api' })).toBeVisible()
  await expect(page.getByRole('table', { name: 'Authorized workload pages' })).toBeVisible()
})

test('shows exact Pod logs and selected aggregates below the inventory on the same page', async ({ page }) => {
  const worker = { ...pod, name: 'worker' }
  const job = { ...pod, name: 'aaa-job', owner: { kind: 'Job', name: 'batch' } }
  const logRequests: string[] = []
  await page.route('**/api/v1/pods?*', (route) => route.fulfill({ json: { data: [job, pod, worker], meta: { generation, page: collection, coverage: null } } }))
  await page.route('**/api/v1/pods/payments/worker', (route) => route.fulfill({ json: { data: { metadata: { ...metadata, name: worker.name }, summary: worker, containers: [{ spec }], initContainers: [], ephemeralContainers: [], conditions: [] }, meta: { generation } } }))
  await page.route('**/api/v1/pods/*/*/logs**', (route) => {
    const url = new URL(route.request().url()); logRequests.push(url.pathname)
    const name = url.pathname.split('/')[5]
    if (url.pathname.endsWith('/stream')) return route.fulfill({ contentType: 'text/event-stream', body: `event: meta\ndata: {"generation":"${generation}"}\n\nevent: line\ndata: {"text":"log from ${name}"}\n\n` })
    return route.fulfill({ json: { data: { lines: [{ timestamp: null, text: `log from ${name}`, truncated: false }], truncated: false }, meta: { generation } } })
  })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/pods')
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table.locator('tbody tr').last()).toContainText('aaa-job')
  await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
  const details = page.getByRole('region', { name: `Pod ${pod.name}` })
  await details.getByRole('tab', { name: 'Logs', exact: true }).click()
  await expect(details.getByLabel('Log output', { exact: true })).toContainText(`log from ${pod.name}`)
  expect(logRequests.every((path) => path.includes(`/${pod.name}/logs`))).toBe(true)
  expect((await details.boundingBox())!.y).toBeGreaterThan((await table.boundingBox())!.y)
  await expect(page).toHaveURL(new RegExp(`/pods/payments/${pod.name}$`))
  await page.screenshot({ path: '/tmp/kubepeep-inline-logs-desktop.png', fullPage: true })
  await page.setViewportSize({ width: 390, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: '/tmp/kubepeep-inline-logs-mobile.png', fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.getByRole('button', { name: 'Close resource workspace' }).click()
  await page.getByRole('checkbox', { name: `Select row payments/${pod.name}`, exact: true }).check()
  await page.getByRole('checkbox', { name: 'Select row payments/worker', exact: true }).check()
  await page.getByRole('button', { name: 'Aggregate logs', exact: true }).click()
  await expect(page.getByLabel('Aggregated log output')).toContainText(`log from ${pod.name}`)
  await expect(page.getByLabel('Aggregated log output')).toContainText('log from worker')
  expect(logRequests.some((path) => path.includes('aaa-job'))).toBe(false)
  await expect(page).toHaveURL(/\/pods$/)
})

test('keeps the source list, search and column selection while inspecting related objects', async ({ page }) => {
  await page.goto('/workloads/kind/deployments?search=api#inventory')
  const table = page.getByRole('table', { name: 'Authorized workload pages' })
  await expect(table).toBeVisible()
  await page.getByRole('button', { name: 'Choose visible columns', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Available', exact: true }).uncheck()
  await page.keyboard.press('Escape')
  await table.getByRole('button', { name: 'Open Deployment api in payments' }).click()
  const panel = page.locator('.workspace-panel')
  await expect(panel).toHaveAttribute('role', 'region')
  await panel.getByRole('button', { name: 'Open ConfigMap settings' }).click()
  await expect(panel).toHaveAttribute('aria-label', 'ConfigMap settings')
  await expect(table).toBeVisible()
  await expect(table.getByRole('columnheader', { name: /Available/ })).toHaveCount(0)
  await expect(page.getByLabel('Search resources')).toHaveValue('api')
  await panel.getByRole('button', { name: 'Go to previous resource' }).click()
  await expect(panel).toHaveAttribute('aria-label', 'Deployment api')
  await panel.getByRole('button', { name: 'Go to next resource' }).click()
  await expect(panel).toHaveAttribute('aria-label', 'ConfigMap settings')
  await panel.getByRole('button', { name: 'Close resource workspace' }).click()
  await expect(page).toHaveURL(/\/workloads\/kind\/deployments\?search=api#inventory$/)
  await expect(table).toBeVisible()
  await table.getByRole('button', { name: 'Open Deployment api in payments' }).click()
  await page.locator('aside').getByRole('link', { name: 'Pods', exact: true }).click()
  await expect(panel).toHaveCount(0)
  await expect(page.getByRole('table', { name: 'Authorized Pod pages' })).toBeVisible()
})

for (const example of [
  { path: '/configuration/resource-quotas', api: 'resource-quotas', row: { hard: {}, used: {} }, open: 'Open quota api in payments' },
  { path: '/service-accounts', api: 'service-accounts', row: {}, open: 'Open ServiceAccount api in payments' },
  { path: '/access/roles', api: 'roles', row: { ruleCount: 2 }, open: 'Open Role api' },
  { path: '/administration/runtime-classes', api: 'runtime-classes', row: { handler: 'runc' }, open: 'Open RuntimeClass api' },
  { path: '/storage/persistent-volume-claims', api: 'persistent-volume-claims', row: { status: 'Bound', volumeName: 'volume', capacity: '1Gi' }, open: 'Open claim api in payments' },
  { path: '/leases', api: 'leases', row: { holderName: 'worker', durationSeconds: 30, renewTime: null }, open: 'Open Lease api in payments' },
  { path: '/network/services', api: 'services', row: { type: 'ClusterIP', clusterIPs: ['10.0.0.1'] }, open: 'Open services api in payments' },
  { path: '/config/configmaps', api: 'configmaps', row: {}, open: 'Open ConfigMap api in payments' },
]) {
  test(`${example.path} shares the compact table and inline inspection behavior`, async ({ page }) => {
    await page.route(`**/api/v1/${example.api}?*`, (route) => route.fulfill({ json: { data: [{ ...metadata, ageSeconds: 60, ...example.row }], meta: { generation, page: collection } } }))
    await page.route(`**/api/v1/${example.api}/**`, (route) => route.fulfill({ json: { data: { metadata }, meta: { generation } } }))
    await page.goto(example.path)
    const table = page.getByRole('table').first()
    await expect(page.getByRole('searchbox', { name: 'Search resources' })).toBeVisible()
    await expect(page.getByRole('button', { name: /Apply filters|Refresh/ })).toHaveCount(0)
    await expect(table.getByRole('button', { name: 'Choose visible columns', exact: true })).toBeVisible()
    await table.getByRole('button', { name: example.open, exact: true }).click()
    const panel = page.locator('.workspace-panel')
    await expect(panel).toBeVisible()
    await expect(panel).toHaveAttribute('role', 'region')
    await expect(table).toBeVisible()
    expect((await panel.boundingBox())!.y).toBeGreaterThan((await table.boundingBox())!.y)
    await panel.getByRole('button', { name: 'Close resource workspace' }).click()
    await expect(page).toHaveURL(new RegExp(`${example.path}$`))
    await expect(table).toBeVisible()
  })
}
