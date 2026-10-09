import { expect, test, type Page } from '@playwright/test'
import { readFile } from 'node:fs/promises'
import { gatewayCollections } from '../src/navigation/gateway'

const generation = 'gen_workspace'
const budget = { cpuRequestMillicores: 500, cpuLimitMillicores: 1000, memoryRequestBytes: 134217728, memoryLimitBytes: 268435456 }
const metadata = { namespace: 'payments', name: 'api', uid: 'uid-api', resourceVersion: '17', creationTimestamp: '2026-10-01T00:00:00Z', labels: {} }
const spec = { name: 'api', image: 'example/api:1', ports: [], resources: budget, environment: [{ name: 'MODE', value: 'production' }, { name: 'TOKEN', value: null, source: { kind: 'Secret', name: 'credentials', key: 'token' } }], envFrom: [{ kind: 'ConfigMap', name: 'settings', prefix: 'APP_' }] }
const pod = { namespace: 'payments', name: 'api-with-a-long-name-abcdef', ready: { current: 1, desired: 1 }, restarts: 0, status: 'Running', node: 'worker-1', ip: '10.0.0.1', owner: { kind: 'Deployment', name: 'api' }, ageSeconds: 60, problematic: false, resources: budget, containerResources: { api: budget }, secrets: ['credentials'], configMaps: ['settings'] }
const yaml = 'apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: payments\n  uid: uid-api\n  resourceVersion: "17"\nspec:\n  replicas: 1\n'
const collection = { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }

async function expectFixedSplit(page: Page) {
  const list = await page.locator('.resource-list-pane').boundingBox()
  const detail = await page.locator('.resource-detail-slot').boundingBox()
  expect(detail!.height / page.viewportSize()!.height).toBeCloseTo(0.7, 2)
  expect(list!.height).toBeCloseTo((await page.locator('.resource-split').boundingBox())!.height, 0)
  expect(detail!.y + detail!.height).toBeCloseTo(list!.y + list!.height, 0)
  expect(detail!.y + detail!.height).toBeLessThanOrEqual(page.viewportSize()!.height)
  expect(await page.evaluate(() => ({
    width: document.documentElement.scrollWidth <= innerWidth,
    height: document.documentElement.scrollHeight <= innerHeight,
    scroll: window.scrollY,
  }))).toEqual({ width: true, height: true, scroll: 0 })
  await expect(page.locator('.workspace-header')).toBeInViewport({ ratio: 1 })
}

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
    else if (path.startsWith('/api/v1/resources/') && path.endsWith('/yaml') && request.method() === 'GET') {
      const parts = path.split('/')
      const kind = ({ deployments: 'Deployment', pods: 'Pod', secrets: 'Secret', configmaps: 'ConfigMap', 'cluster-roles': 'ClusterRole', 'storage-classes': 'StorageClass', 'runtime-classes': 'RuntimeClass' } as Record<string, string>)[parts[4]] ?? 'ConfigMap'
      const document = yaml.replace('apiVersion: apps/v1', kind === 'Deployment' ? 'apiVersion: apps/v1' : 'apiVersion: v1').replace('kind: Deployment', `kind: ${kind}`).replace('name: api', `name: ${decodeURIComponent(parts.at(-2)!)}`)
      data = { yaml: document, kind, updateCapability: `yaml.${parts[4]}.update`, generation }
    }
    else if (path.endsWith('/yaml') && request.method() === 'GET') { await route.fulfill({ status: 200, contentType: 'application/yaml', body: yaml }); return }
    else if (path.endsWith('/yaml') && request.method() === 'PUT') data = { accepted: true, action: 'updateResource', resourceVersion: '18', generation }
    else if (path === '/api/v1/permissions') data = { generation, complete: true, truncated: false, errors: [], decisions: url.searchParams.getAll('capability').map((capabilityId) => ({ capabilityId, namespace: url.searchParams.get('namespace') ?? '', resourceName: url.searchParams.get('resourceName') ?? 'api', decision: 'allowed' })) }
    else if (path === '/api/v1/session') data = { csrfToken: 'csrf_workspace', generation }
    else if (path === '/api/v1/hpas') data = [{ namespace: 'payments', name: 'api-hpa', targetKind: 'Deployment', targetName: 'api', resourceTargets: [{ resource: 'cpu', utilization: 60, averageValue: null }] }]
    else if (path === '/api/v1/metrics') data = { complete: true, truncated: false, errors: [], coverage: null, value: { collectedAt: '2026-10-06T12:00:00Z', windowSeconds: 30, pods: [{ namespace: 'payments', pod: pod.name, cpuMillicores: 350, memoryBytes: 249561088, containers: [{ name: 'api', cpuMillicores: 350, memoryBytes: 249561088 }] }], topCPU: [], topMemory: [] } }
    else if (path === '/api/v1/secrets') data = [{ metadata: { ...metadata, name: 'credentials' } }]
    else if (path === '/api/v1/configmaps') data = [{ ...metadata, name: 'settings' }]
    else if (path === '/api/v1/secrets/payments/credentials') data = { apiVersion: 'v1', kind: 'Secret', metadata: { ...metadata, name: 'credentials' } }
    else if (path === '/api/v1/secrets/payments/credentials/data') data = { metadata: { ...metadata, name: 'credentials' }, entries: [{ key: 'token', value: 'c2Vuc2l0aXZlLWZpeHR1cmU=', field: 'data', encoding: 'base64', truncated: false }], truncated: false }
    else if (path === '/api/v1/configmaps/payments/settings') data = { metadata: { ...metadata, name: 'settings' }, entries: [{ key: 'MODE', value: 'production', encoding: 'utf-8', truncated: false }], truncated: false }
    await route.fulfill({ status: 200, json: { data, meta } })
  })
})

for (const entry of [
  { route: '/pods', endpoint: '/api/v1/pods' },
  { route: '/workloads/kind/deployments', endpoint: '/api/v1/workloads' },
  { route: '/configuration/hpas', endpoint: '/api/v1/hpas' },
  { route: '/config/configmaps', endpoint: '/api/v1/configmaps' },
]) {
  test(`default namespace precedes explicit All and is restored on reload in ${entry.route}`, async ({ page }) => {
    const queries: URL[] = []
    page.on('request', (request) => {
      const url = new URL(request.url())
      if (url.pathname === entry.endpoint) queries.push(url)
    })
    await page.goto(entry.route)
    const namespace = page.getByRole('combobox', { name: 'Global namespace' })
    await expect(namespace).toHaveValue('payments')
    await expect.poll(() => queries.length).toBeGreaterThan(0)
    expect(queries.every((url) => url.searchParams.get('namespace') === 'payments')).toBe(true)
    await namespace.selectOption('')
    await expect.poll(() => queries.some((url) => !url.searchParams.has('namespace'))).toBe(true)
    await expect(namespace).toHaveValue('')
    queries.length = 0
    await page.reload()
    await expect(namespace).toHaveValue('payments')
    await expect.poll(() => queries.length).toBeGreaterThan(0)
    expect(queries.every((url) => url.searchParams.get('namespace') === 'payments')).toBe(true)
  })
}

test('changing context restores its own default before the first resource query', async ({ page }) => {
  let context = 'development'
  let selectedGeneration = generation
  const selection = () => ({ clusterProfileId: 1, context, cluster: context, scopeId: context === 'development' ? 1 : 2, scopeMode: 'list', scopeName: context, scopeSource: 'saved', defaultNamespace: context === 'development' ? 'payments' : 'stage-apps', namespaceCount: 2, generation: selectedGeneration })
  await page.route('**/api/v1/status', (route) => route.fulfill({ json: { data: { version: 'test', selection: selection(), components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((key) => [key, { status: key === 'metrics' ? 'unknown' : 'healthy' }])) } } }))
  await page.route('**/api/v1/cluster/profiles', (route) => route.fulfill({ json: { data: [{ id: 1, name: 'Local', context, isDefault: true, kubeconfigFiles: [{ position: 0, displayPath: '~/.kube/config' }] }] } }))
  await page.route('**/api/v1/contexts?*', (route) => route.fulfill({ json: { data: ['development', 'staging'].map((name) => ({ clusterProfileId: 1, name, cluster: name, selected: name === context })) } }))
  await page.route('**/api/v1/session', (route) => route.fulfill({ json: { data: { csrfToken: 'csrf', generation: selectedGeneration } } }))
  await page.route('**/api/v1/contexts/select', async (route) => {
    context = route.request().postDataJSON().context
    selectedGeneration = 'gen_staging'
    await route.fulfill({ json: { data: selection(), meta: { generation: selectedGeneration } } })
  })
  await page.route('**/api/v1/namespace-scopes/2', (route) => route.fulfill({ json: { data: { namespaces: ['alpha', 'stage-apps'], defaultNamespace: 'stage-apps' } } }))
  const namespaces: string[] = []
  await page.route('**/api/v1/pods?*', (route) => {
    namespaces.push(new URL(route.request().url()).searchParams.get('namespace') ?? 'All')
    return route.fulfill({ json: { data: [{ ...pod, namespace: selection().defaultNamespace }], meta: { generation: selectedGeneration, page: collection } } })
  })
  await page.goto('/pods')
  const namespace = page.getByRole('combobox', { name: 'Global namespace' })
  await expect(namespace).toHaveValue('payments')
  await namespace.selectOption('')
  await expect.poll(() => namespaces.includes('All')).toBe(true)
  namespaces.length = 0
  await page.getByRole('combobox', { name: 'Kubernetes context', exact: true }).selectOption('staging')
  await expect(namespace).toHaveValue('stage-apps')
  await expect.poll(() => namespaces.includes('stage-apps')).toBe(true)
  expect(namespaces).not.toContain('All')
  expect(namespaces).not.toContain('payments')
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
    expect(name!.width).toBeGreaterThanOrEqual(width < 600 ? 110 : 120)
    expect(await page.locator('.data-table').evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true)
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
  for (const [width, height] of [[320, 568], [768, 1024], [844, 390], [1024, 600], [1920, 1080], [2560, 1440]]) {
    await page.setViewportSize({ width, height })
    await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
    const panel = page.getByRole('region', { name: `Pod ${pod.name}` })
    await expect(panel).toBeVisible()
    await expect(panel).toHaveCSS('animation-name', 'none')
    await expectFixedSplit(page)
    await expect(panel.getByRole('button', { name: 'Close resource workspace' })).toBeInViewport()
    await panel.getByRole('tab', { name: 'Logs', exact: true }).click()
    await expect(panel.getByLabel('Log output', { exact: true })).toContainText('workload log fixture')
    // Fractional grid tracks can clip a subpixel at the terminal border.
    await expect(panel.getByLabel('Log output', { exact: true })).toBeInViewport({ ratio: 0.99 })
    expect((await panel.getByLabel('Log output', { exact: true }).boundingBox())!.height).toBeGreaterThan(25)
    await expectFixedSplit(page)
    await expect(panel.getByRole('button', { name: 'Close resource workspace' })).toBeInViewport()
    await panel.getByRole('button', { name: 'Close resource workspace' }).click()
  }
})

test('contains list, logs, environment and YAML scrolling inside the fixed split', async ({ page }, testInfo) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.setViewportSize({ width: 1440, height: 900 })
  const environment = Array.from({ length: 100 }, (_, index) => ({ name: `SETTING_${index}`, value: `value-${index}` }))
  const lines = Array.from({ length: 200 }, (_, index) => ({ timestamp: null, text: `Application log line ${index}`, truncated: false }))
  const longYaml = yaml + Array.from({ length: 200 }, (_, index) => `# document line ${index}`).join('\n')
  await page.route('**/api/v1/pods?*', (route) => route.fulfill({ json: { data: [pod, ...Array.from({ length: 80 }, (_, index) => ({ ...pod, name: `worker-${index}` }))], meta: { generation, page: collection } } }))
  await page.route(`**/api/v1/pods/payments/${pod.name}`, (route) => route.fulfill({ json: { data: { metadata: { ...metadata, name: pod.name }, summary: pod, conditions: [], containers: [{ spec: { ...spec, environment }, type: 'regular', ready: true, restartCount: 0, state: 'running' }], initContainers: [], ephemeralContainers: [], relatedEvents: [] }, meta: { generation } } }))
  await page.route('**/api/v1/pods/*/*/logs**', (route) => new URL(route.request().url()).pathname.endsWith('/stream')
    ? route.fulfill({ contentType: 'text/event-stream', body: `event: meta\ndata: {"generation":"${generation}"}\n\n` + lines.map((line) => `event: line\ndata: ${JSON.stringify(line)}\n\n`).join('') })
    : route.fulfill({ json: { data: { lines, truncated: false }, meta: { generation } } }))
  await page.route('**/api/v1/resources/pods/*/*/yaml', (route) => route.fulfill({ json: { data: { yaml: longYaml, kind: 'Pod', updateCapability: 'yaml.pods.update', generation } } }))
  await page.goto('/pods')
  await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
  const panel = page.locator('.workspace-panel')
  const header = panel.locator('.workspace-header')
  const headerTop = (await header.boundingBox())!.y
  const list = page.locator('.resource-collection > .data-table')
  await list.evaluate((element) => { element.scrollTop = element.scrollHeight })
  expect(await list.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  expect((await header.boundingBox())!.y).toBe(headerTop)
  await panel.getByRole('tab', { name: 'Logs', exact: true }).click()
  const logs = panel.getByLabel('Log output', { exact: true })
  await expect(logs).toContainText('Application log line 199')
  await logs.hover()
  await page.mouse.wheel(0, 500)
  await expect.poll(() => logs.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  await expectFixedSplit(page)
  await page.screenshot({ path: testInfo.outputPath('fixed-split-desktop.png') })
  await page.setViewportSize({ width: 390, height: 844 })
  await expectFixedSplit(page)
  await page.screenshot({ path: testInfo.outputPath('fixed-split-mobile.png') })
  await page.setViewportSize({ width: 1440, height: 900 })
  const listScroll = await list.evaluate((element) => element.scrollTop)
  await panel.getByRole('tab', { name: 'Data / Env', exact: true }).click()
  const tab = panel.getByRole('tabpanel')
  await expect(tab).toContainText('SETTING_99')
  await tab.evaluate((element) => { element.scrollTop = element.scrollHeight })
  expect(await tab.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  await expect(tab.getByText('SETTING_99', { exact: true })).toBeInViewport()
  await panel.getByRole('tab', { name: 'YAML', exact: true }).click()
  await panel.getByRole('button', { name: 'Load authorized YAML' }).click()
  const document = panel.getByRole('region', { name: 'YAML document' })
  await expect(document).toContainText('document line 199')
  await panel.getByRole('textbox', { name: 'Search in YAML' }).fill('document line 199')
  await panel.getByRole('button', { name: 'Next match' }).click()
  expect(await document.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  await expect(document.locator('mark')).toBeInViewport()
  expect(await tab.evaluate((element) => element.scrollTop)).toBe(0)
  expect(await panel.locator('.workspace-body').evaluate((element) => element.scrollTop)).toBe(0)
  expect(await list.evaluate((element) => element.scrollTop)).toBe(listScroll)
  expect((await header.boundingBox())!.y).toBe(headerTop)
  await expectFixedSplit(page)
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

for (const emptyTerminal of [false, true]) {
  test(`keeps authorized Pods visible when partial pagination ends with ${emptyTerminal ? 'an empty terminal page' : 'the last rows'}`, async ({ page }) => {
    await page.clock.install()
    let requests = 0
    await page.route('**/api/v1/pods?*', async (route) => {
      requests++
      const next = new URL(route.request().url()).searchParams.has('continue')
      await route.fulfill({ json: {
        data: next ? emptyTerminal ? [] : [{ ...pod, name: 'last-authorized-pod' }] : [pod],
        meta: {
          generation,
          page: { ...collection, complete: false, truncated: true, next: next ? '' : 'partial-next' },
          coverage: { requestedNamespaces: 2, completedNamespaces: 1, deniedNamespaces: [], failed: [{ namespace: 'restricted', code: 'AUTHORIZATION_UNAVAILABLE', message: 'Authorization could not be confirmed.' }] },
        },
      } })
    })
    await page.goto('/pods')
    const table = page.getByRole('table', { name: 'Authorized Pod pages' })
    await expect(table).toBeVisible()
    await page.getByRole('button', { name: 'Load next page' }).click()
    await expect(page.getByRole('button', { name: 'Load next page' })).toBeDisabled()
    await expect(table.getByRole('button', { name: `Open Pod ${pod.name} in payments` })).toBeVisible()
    const warning = page.getByRole('note').filter({ hasText: 'Partial result' })
    await expect(warning).toContainText('1 failed')
    await warning.click()
    await expect(page.getByText(/restricted · AUTHORIZATION_UNAVAILABLE/)).toBeVisible()
    await expect(page.getByText('Resource request failed', { exact: true })).toHaveCount(0)
    await page.clock.fastForward(16_000)
    await expect.poll(() => requests).toBe(4)
    await expect(table).toBeVisible()
    await expect(page.getByRole('button', { name: 'Load next page' })).toBeDisabled()
  })
}

for (const [status, code] of [[403, 'FORBIDDEN'], [409, 'GENERATION_CHANGED']] as const) {
  test(`stops Pod polling after ${status}/${code}`, async ({ page }) => {
    await page.clock.install()
    let requests = 0
    await page.route('**/api/v1/pods?*', async (route) => {
      requests++
      await route.fulfill({ status, json: { code, message: 'Access could not be granted.' } })
    })
    await page.goto('/pods')
    await expect(page.getByText(code, { exact: true })).toBeVisible()
    const initialRequests = requests
    await page.clock.runFor(31_000)
    expect(requests).toBe(initialRequests)
    await expect(page.getByRole('table', { name: 'Authorized Pod pages' })).toHaveCount(0)
  })
}

for (const [status, code] of [[503, 'AUTHORIZATION_UNAVAILABLE'], [401, 'AUTHENTICATION_UNAVAILABLE']] as const) {
 for (const initiallyUnavailable of [true, false]) {
  test(`recovers from Pod ${code} ${initiallyUnavailable ? 'on initial load' : 'after loading rows'} without navigation`, async ({ page }) => {
    await page.clock.install()
    let unavailable = initiallyUnavailable
    let requests = 0
    let recovering = false
    let releaseRecovery!: () => void
    const recovery = new Promise<void>((resolve) => { releaseRecovery = resolve })
    await page.route('**/api/v1/pods?*', async (route) => {
      requests++
      if (unavailable) {
        await route.fulfill({ status, json: { code, message: status === 401 ? 'Cluster credentials expired; retrying authentication.' : 'Authorization could not be confirmed.' } })
        return
      }
      if (recovering) await recovery
      await route.fulfill({ json: { data: [pod], meta: { generation, page: collection } } })
    })
    await page.goto('/pods')
    const table = page.getByRole('table', { name: 'Authorized Pod pages' })
    if (!initiallyUnavailable) {
      await expect(table).toBeVisible()
      unavailable = true
      await page.clock.fastForward(16_000)
    }
    await expect(page.getByText(code, { exact: true })).toBeVisible()
    await expect(table).toHaveCount(0)
    await expect(page.getByLabel('Resource live updates')).toHaveText('Auto · 10s')
    // A cached unknown decision can outlive the first interval. Keep retrying
    // without displaying cached rows or issuing an immediate retry burst.
    for (let attempt = 0; attempt < 2; attempt++) {
      const previousRequests = requests
      await page.clock.fastForward(16_000)
      await expect.poll(() => requests).toBe(previousRequests + 1)
      await expect(page.getByText(code, { exact: true })).toBeVisible()
      await expect(table).toHaveCount(0)
    }
    unavailable = false
    recovering = true
    const previousRequests = requests
    await page.clock.fastForward(16_000)
    await expect.poll(() => requests).toBe(previousRequests + 1)
    await expect(table).toHaveCount(0)
    releaseRecovery()
    await expect(table).toBeVisible()
    await expect(page.getByText(code, { exact: true })).toHaveCount(0)
    await expect(page).toHaveURL(/\/pods$/)
  })
 }
}

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
  await page.getByText('MODE · UTF-8 · text', { exact: true }).click()
  await expect(page.getByText('production', { exact: true })).toBeVisible()
  await page.goto('/pods')
  await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
  await page.getByRole('tab', { name: 'Data / Env', exact: true }).click()
  await page.getByRole('button', { name: 'Secret: credentials / token' }).click()
  await page.getByRole('tab', { name: 'Data', exact: true }).click()
  expect(secretReads).toBe(0)
  await page.getByRole('button', { name: 'Reveal data' }).click()
  await page.getByText('token · data · Base64 · encoded value', { exact: true }).click()
  await expect(page.getByText('c2Vuc2l0aXZlLWZpeHR1cmU=', { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: 'Overview', exact: true }).click()
  await page.getByRole('tab', { name: 'Data', exact: true }).click()
  await expect(page.getByText('c2Vuc2l0aXZlLWZpeHR1cmU=', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Reveal data' })).toBeVisible()
  expect(secretReads).toBe(1)
})

test('Deployment YAML saves with unchanged identity, version and CSRF', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/workloads/deployments/payments/api')
  await page.getByRole('tab', { name: 'YAML', exact: true }).click()
  await page.getByRole('button', { name: 'Load authorized YAML' }).click()
  await page.getByRole('button', { name: 'Edit YAML', exact: true }).click()
  const editor = page.getByLabel('Deployment YAML', { exact: true })
  await editor.fill(yaml + '# editable line\n'.repeat(200))
  await editor.evaluate((element) => { element.scrollTop = element.scrollHeight })
  expect(await editor.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  await expect(editor).toBeInViewport({ ratio: 0.99 })
  await expect(page.getByRole('button', { name: 'Save Deployment', exact: true })).toBeInViewport()
  await expectFixedSplit(page)
  await editor.fill(yaml.replace('replicas: 1', 'replicas: 2'))
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
  await expectFixedSplit(page)
  await expect(page.locator('.resource-detail-slot').getByLabel('Aggregated log output')).toBeInViewport({ ratio: 1 })
  expect(logRequests.some((path) => path.includes('aaa-job'))).toBe(false)
  await expect(page).toHaveURL(/\/pods$/)
  await page.getByRole('button', { name: 'Close logs', exact: true }).click()
  await expect(page.locator('.resource-detail-slot')).toBeHidden()
  expect((await page.locator('.resource-list-pane').boundingBox())!.height).toBe((await page.locator('.resource-split').boundingBox())!.height)
})

test('follows live lines and downloads session, visible, full and previous logs in place', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  let follows = 0
  const downloads: string[] = []
  const start = Date.now() - 1_200_000
  const first = Array.from({ length: 1200 }, (_, index) => ({ timestamp: new Date(start + index * 1000).toISOString(), text: `initial-${index}`, truncated: false }))
  const sse = (lines: unknown[], reason: string) => `event: meta\ndata: ${JSON.stringify({ generation })}\n\n` + lines.map((line) => `event: line\ndata: ${JSON.stringify(line)}\n\n`).join('') + `event: end\ndata: ${JSON.stringify({ generation, reason, truncated: false })}\n\n`
  await page.route('**/api/v1/pods/*/*/logs/stream?*', (route) => {
    follows++
    const lines = follows === 1 ? first : [first.at(-1)!, { timestamp: new Date().toISOString(), text: `live-${follows}`, truncated: false }]
    return route.fulfill({ contentType: 'text/event-stream', body: sse(lines, 'upstream_eof') })
  })
  await page.route('**/api/v1/pods/*/*/logs/download/stream?*', (route) => {
    const url = new URL(route.request().url())
    expect(url.searchParams.has('tailLines')).toBe(false)
    expect(route.request().headers()['x-kubepeep-csrf']).toBe('csrf_workspace')
    downloads.push(url.searchParams.get('previous')!)
    return route.fulfill({ contentType: 'text/event-stream', body: sse([{ text: url.searchParams.get('previous') === 'true' ? 'previous-container-history' : 'full-retained-history', truncated: false }], 'completed') })
  })
  await page.goto('/pods')
  await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
  await page.getByRole('tab', { name: 'Logs', exact: true }).click()
  const panel = page.locator('.workspace-panel')
  const output = panel.getByLabel('Log output', { exact: true })
  await expect(output).toContainText('live-2')
  await expect(output).not.toContainText('initial-0\n')
  expect(await output.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop)).toBeLessThan(2)
  await output.evaluate((element) => { element.scrollTop = 0 })
  await expect(panel.getByRole('button', { name: 'Follow', exact: true })).toHaveAttribute('aria-pressed', 'false')
  const nextWhileReading = follows + 1
  await expect(output).toContainText(`live-${nextWhileReading}`, { timeout: 15000 })
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0)
  await panel.getByRole('button', { name: 'Follow', exact: true }).click()
  await panel.getByRole('button', { name: 'Pause', exact: true }).click()
  const pausedText = await output.textContent()
  const next = follows + 1
  await expect.poll(() => follows, { timeout: 15000 }).toBeGreaterThanOrEqual(next)
  await expect(output).toHaveText(pausedText!)

  async function download(scope: string) {
    await panel.getByLabel('Download log scope').selectOption(scope)
    const ready = page.waitForEvent('download')
    await panel.getByRole('button', { name: 'Download', exact: true }).click()
    const file = await ready
    expect(file.suggestedFilename()).toContain(`-${scope}-`)
    return readFile((await file.path())!, 'utf8')
  }
  // The archive includes rows evicted from the viewer and lines received while paused.
  const session = await download('session')
  expect(session).toContain('initial-0\n')
  expect(session).toContain(`live-${next}`)
  expect(session.match(/initial-1199\n/g)).toHaveLength(1)
  await panel.getByLabel('Search Pod logs').fill('live-2')
  const visible = await download('visible')
  expect(visible).toContain('live-2')
  expect(visible).not.toContain('initial-')
  expect(visible).not.toContain(`live-${next}`)
  expect(await download('all')).toContain('full-retained-history')
  expect(await download('previous')).toContain('previous-container-history')
  expect(downloads).toEqual(['false', 'true'])
  await panel.getByLabel('Search Pod logs').fill('')
  await panel.getByRole('button', { name: 'Resume', exact: true }).click()
  await expect(output).toContainText(`live-${next}`)
  await expectFixedSplit(page)
  await page.screenshot({ path: testInfo.outputPath('live-downloads-desktop.png') })
  await page.setViewportSize({ width: 390, height: 844 })
  await expectFixedSplit(page)
  await expect.poll(() => output.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop)).toBeLessThan(2)
  await page.screenshot({ path: testInfo.outputPath('live-downloads-mobile.png') })
})

test('marks partial log exports and never saves denied or canceled downloads', async ({ page }) => {
  let requests = 0
  let release!: () => void
  let delivered = false
  let saved = 0
  page.on('download', () => { saved++ })
  const pending = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/v1/pods/*/*/logs/download/stream?*', async (route) => {
    requests++
    if (requests === 2) {
      await route.fulfill({ status: 403, json: { code: 'FORBIDDEN', message: 'Download denied.' } })
      return
    }
    if (requests === 3) await pending
    await route.fulfill({ contentType: 'text/event-stream', body: `event: meta\ndata: ${JSON.stringify({ generation })}\n\nevent: line\ndata: {"text":"partial export"}\n\nevent: end\ndata: ${JSON.stringify({ generation, reason: 'limit_reached', truncated: true })}\n\n` })
    if (requests === 3) delivered = true
  })
  await page.goto('/pods')
  await page.getByRole('button', { name: `Open Pod ${pod.name} in payments` }).click()
  await page.getByRole('tab', { name: 'Logs', exact: true }).click()
  const panel = page.locator('.workspace-panel')
  await expect(panel.getByLabel('Log output', { exact: true })).toContainText('workload log fixture')
  await panel.getByLabel('Download log scope').selectOption('all')
  const downloading = page.waitForEvent('download')
  await panel.getByRole('button', { name: 'Download', exact: true }).click()
  expect((await downloading).suggestedFilename()).toMatch(/-partial\.log$/)
  await expect(panel.getByText('Download is partial: the server reported a size or line limit.')).toBeVisible()
  await panel.getByRole('button', { name: 'Download', exact: true }).click()
  await expect(panel.getByText('Download denied.')).toBeVisible()
  expect(saved).toBe(1)
  await panel.getByRole('button', { name: 'Download', exact: true }).click()
  await expect.poll(() => requests).toBe(3)
  await expect(panel.getByLabel('Download log scope')).toBeDisabled()
  await panel.getByRole('button', { name: 'Cancel download', exact: true }).click()
  release()
  await expect.poll(() => delivered).toBe(true)
  await expect(panel.getByRole('button', { name: 'Download', exact: true })).toBeEnabled()
  expect(saved).toBe(1)
  await expect(panel.getByLabel('Log output', { exact: true })).toContainText('workload log fixture')
})

test('associated resources open their exact filtered inventory and preserve workspace back/forward', async ({ page }) => {
  const queries: URL[] = []
  page.on('request', (request) => { const url = new URL(request.url()); if (url.pathname === '/api/v1/configmaps') queries.push(url) })
  await page.goto('/workloads/kind/deployments?search=api#inventory')
  const table = page.getByRole('table', { name: 'Authorized workload pages' })
  await table.getByRole('button', { name: 'Open Deployment api in payments' }).click()
  const panel = page.locator('.workspace-panel')
  await panel.getByRole('button', { name: 'Open ConfigMap settings' }).click()
  await expect(panel).toHaveAttribute('aria-label', 'ConfigMap settings')
  await expect(table).toHaveCount(0)
  await expect(page.getByText('Exact name:')).toContainText('settings')
  await expect(page.getByRole('combobox', { name: 'Global namespace' })).toHaveValue('payments')
  await expect.poll(() => queries.some((url) => url.searchParams.get('fieldSelector') === 'metadata.name=settings' && url.searchParams.get('namespace') === 'payments')).toBe(true)
  await panel.getByRole('button', { name: 'Go to previous resource' }).click()
  await expect(panel).toHaveAttribute('aria-label', 'Deployment api')
  await expect(table).toBeVisible()
  await expect(page.getByLabel('Search resources')).toHaveValue('api')
  await panel.getByRole('button', { name: 'Go to next resource' }).click()
  await expect(panel).toHaveAttribute('aria-label', 'ConfigMap settings')
  await panel.getByRole('button', { name: 'Close resource workspace' }).click()
  await expect(page).toHaveURL(/\/config\/configmaps\?focus=settings&namespace=payments$/)
  await expect(panel).toHaveCount(0)
  await page.getByRole('button', { name: 'Clear object filter' }).click()
  await expect(page.getByText('Exact name:')).toHaveCount(0)
  await expect.poll(() => queries.some((url) => !url.searchParams.has('fieldSelector'))).toBe(true)
})

const inventoryExamples: Array<{ path: string; api: string; row: Record<string, unknown>; open: string; expected: string; cluster?: boolean; nested?: boolean; detail?: Record<string, unknown> }> = [
  ...Object.entries(gatewayCollections).map(([api, kind]) => ({ path: `/network/gateway-api/${api}`, api, row: { kind, apiVersion: 'gateway.networking.k8s.io/v1', status: 'Ready', className: 'edge-class', hosts: ['api.example.test'], addresses: ['10.0.0.1'], listeners: 1, rules: 1, conditions: [], related: [] }, open: `Open ${kind} api`, expected: 'api.example.test', cluster: api === 'gateway-classes' })),
  { path: '/configuration/resource-quotas', api: 'resource-quotas', row: { hard: { pods: '20' }, used: { pods: '3' } }, open: 'Open quota api in payments', expected: 'pods: 20' },
  { path: '/configuration/limit-ranges', api: 'limit-ranges', row: { items: [{ type: 'Container', max: { cpu: '4' } }] }, open: 'Open limit range api in payments', expected: 'cpu: 4' },
  { path: '/configuration/hpas', api: 'hpas', row: { targetKind: 'Deployment', targetName: 'scaled-api', minReplicas: 1, maxReplicas: 10, currentReplicas: 2, desiredReplicas: 3, metricNames: ['cpu'], resourceTargets: [], conditions: [] }, open: 'Open autoscaler api in payments', expected: 'scaled-api' },
  { path: '/configuration/pdbs', api: 'pdbs', row: { minAvailable: { isInt: false, string: '50%' }, currentHealthy: 2, desiredHealthy: 1, disruptionsAllowed: 1 }, open: 'Open budget api in payments', expected: '50%' },
  { path: '/service-accounts', api: 'service-accounts', row: {}, open: 'Open ServiceAccount api in payments', expected: 'uid-api' },
  ...['roles', 'cluster-roles'].map((api) => ({ path: `/access/${api}`, api, row: { ruleCount: 1, rules: [{ apiGroups: [''], resources: ['pods'], verbs: ['get', 'list'] }] }, open: 'Open Role api', expected: 'verbs: get; list', cluster: api.startsWith('cluster-') })),
  ...['role-bindings', 'cluster-role-bindings'].map((api) => ({ path: `/access/${api}`, api, row: { roleRefKind: 'Role', roleRefName: 'read-pods', subjects: [{ kind: 'ServiceAccount', name: 'reader', namespace: 'payments' }] }, open: 'Open Binding api', expected: 'read-pods', cluster: api.startsWith('cluster-') })),
  { path: '/administration/runtime-classes', api: 'runtime-classes', row: { handler: 'runc' }, open: 'Open RuntimeClass api', expected: 'runc', cluster: true },
  { path: '/administration/priority-classes', api: 'priority-classes', row: { value: 1000, globalDefault: false, preemptionPolicy: 'Never' }, open: 'Open PriorityClass api', expected: 'Never', cluster: true },
  { path: '/administration/customresourcedefinitions', api: 'customresourcedefinitions', row: { kind: 'Widget', group: 'widgets.example', scope: 'Namespaced', versions: [{ name: 'v1', storage: true, served: true }] }, open: 'Open CRD api', expected: 'widgets.example', cluster: true },
  ...['mutating-webhook-configurations', 'validating-webhook-configurations'].map((api) => ({ path: `/administration/${api}`, api, row: { webhookCount: 1, webhooks: [{ name: 'admission.example', failurePolicy: 'Fail' }] }, open: 'Open webhook configuration api', expected: 'admission.example', cluster: true })),
  { path: '/storage/persistent-volume-claims', api: 'persistent-volume-claims', row: { status: 'Bound', volumeName: 'volume', capacity: '1Gi' }, open: 'Open claim api in payments', expected: 'volume' },
  { path: '/storage/persistent-volumes', api: 'persistent-volumes', row: { status: 'Bound', capacity: '1Gi', storageClass: 'fast-disk' }, open: 'Open api', expected: 'fast-disk', cluster: true },
  { path: '/storage/storage-classes', api: 'storage-classes', row: { provisioner: 'csi.example', default: false, volumeBindingMode: 'WaitForFirstConsumer' }, open: 'Open api', expected: 'WaitForFirstConsumer', cluster: true },
  { path: '/storage/volume-attachments', api: 'volume-attachments', row: { nodeName: 'worker-2', attacher: 'csi.example', attached: true }, open: 'Open api', expected: 'worker-2', cluster: true },
  { path: '/storage/csi-nodes', api: 'csi-nodes', row: { driverCount: 1, drivers: [{ name: 'csi.example', nodeID: 'worker-2' }] }, open: 'Open api', expected: 'csi.example', cluster: true },
  { path: '/storage/csi-drivers', api: 'csi-drivers', row: { attachRequired: true, fsGroupPolicy: 'ReadWriteOnceWithFSType' }, open: 'Open api', expected: 'ReadWriteOnceWithFSType', cluster: true },
  { path: '/nodes', api: 'nodes', row: { roles: ['worker'], status: 'Ready', ready: true, kubeletVersion: 'v1.35.2', internalIP: '10.0.0.5', conditions: [], capacity: { cpu: '8' }, allocatable: { cpu: '7' } }, open: 'Open Node api', expected: 'v1.35.2', cluster: true },
  { path: '/leases', api: 'leases', row: { holderName: 'worker', durationSeconds: 30, renewTime: null }, open: 'Open Lease api in payments', expected: 'worker' },
  { path: '/network/services', api: 'services', row: { type: 'ClusterIP', clusterIPs: ['10.0.0.1'], ports: [] }, open: 'Open services api in payments', expected: '10.0.0.1', nested: true },
  { path: '/network/ingresses', api: 'ingresses', row: { className: 'nginx', hosts: ['api.example'], paths: [], tlsHosts: [] }, open: 'Open ingresses api in payments', expected: 'api.example', nested: true },
  { path: '/network/endpoint-slices', api: 'endpoint-slices', row: { addressType: 'IPv4', endpoints: [{ addresses: ['10.0.0.9'], conditions: { ready: true } }], ports: [] }, open: 'Open endpoint-slices api in payments', expected: '10.0.0.9', nested: true },
  { path: '/network/endpoints', api: 'endpoints', row: { readyCount: 1, notReadyCount: 0, subsets: [] }, open: 'Open endpoints api in payments', expected: 'Ready addresses' },
  { path: '/network/ingress-classes', api: 'ingress-classes', row: { controller: 'ingress.example', default: false }, open: 'Open ingress-classes api', expected: 'ingress.example', cluster: true },
  { path: '/network/network-policies', api: 'network-policies', row: { podSelector: 'app=api', ruleSummary: [], policyTypes: ['Ingress'] }, open: 'Open network-policies api in payments', expected: 'app=api' },
  { path: '/config/configmaps', api: 'configmaps', row: {}, detail: { entries: [{ key: 'MODE', value: 'production', encoding: 'utf-8' }] }, open: 'Open ConfigMap api in payments', expected: 'MODE' },
  { path: '/config/secrets', api: 'secrets', row: {}, detail: { type: 'Opaque' }, open: 'Open Secret api in payments', expected: 'Opaque' },
]

for (const example of inventoryExamples) {
  test(`${example.path} loads selected data and shares full list / 70% overlay behavior`, async ({ page }) => {
    const identity = { ...metadata, ...(example.cluster ? { namespace: undefined } : {}) }
    const row = { ...identity, ageSeconds: 60, ...example.row }
    let detailReads = 0
    await page.route(`**/api/v1/${example.api}?*`, (route) => route.fulfill({ json: { data: [row], meta: { generation, page: collection } } }))
    await page.route(`**/api/v1/${example.api}/**`, (route) => {
      detailReads++
      expect(new URL(route.request().url()).pathname).toBe(`/api/v1/${example.api}/${example.cluster ? '' : 'payments/'}api`)
      return route.fulfill({ json: { data: { metadata: identity, ...(example.nested ? { summary: row } : row), ...example.detail }, meta: { generation } } })
    })
    await page.goto(example.path)
    const table = page.getByRole('table').first()
    await expect(table).toBeVisible()
    await table.getByRole('checkbox', { name: 'Select all loaded rows' }).check()
    await expect(page.getByRole('toolbar', { name: 'Bulk actions' })).toContainText('1 selected')
    await expect(page.getByRole('button', { name: 'Delete selected' })).toHaveCount(0)
    await page.getByRole('button', { name: 'Clear selection' }).click()
    const inventory = page.locator('.resource-collection')
    const initial = (await inventory.boundingBox())!
    const pane = (await page.locator('.resource-list-pane').boundingBox())!
    expect(initial.y + initial.height).toBeCloseTo(pane.y + pane.height, 0)
    await expect(page.getByRole('searchbox', { name: 'Search resources' })).toBeVisible()
    await expect(table.getByRole('button', { name: 'Choose visible columns', exact: true })).toBeVisible()
    for (let iteration = 0; iteration < 2; iteration++) {
      await table.getByRole('button', { name: example.open, exact: true }).click()
      const panel = page.locator('.workspace-panel')
      await expect(panel.getByRole('tabpanel')).toContainText(example.expected)
      await expectFixedSplit(page)
      expect((await inventory.boundingBox())!.height).toBeCloseTo(initial.height, 0)
      await panel.getByRole('button', { name: 'Close resource workspace' }).click()
      await expect(panel).toHaveCount(0)
      await expect(page.locator('.resource-detail-slot')).toBeHidden()
      await expect(page).toHaveURL(new RegExp(`${example.path}$`))
      expect((await inventory.boundingBox())!.height).toBeCloseTo(initial.height, 0)
    }
    expect(detailReads).toBeGreaterThan(0)
  })
}

for (const target of [
  ...Object.entries(gatewayCollections).map(([collection, kind]) => ({ collection, kind, name: 'edge', namespace: collection === 'gateway-classes' ? null : 'payments', path: `/network/gateway-api/${collection}/${collection === 'gateway-classes' ? '' : 'payments/'}edge`, apiVersion: 'gateway.networking.k8s.io/v1beta1' })),
  { collection: 'pods', kind: 'Pod', name: pod.name, namespace: 'payments', path: `/pods/payments/${pod.name}`, apiVersion: 'v1' },
  { collection: 'configmaps', kind: 'ConfigMap', name: 'settings', namespace: 'payments', path: '/config/configmaps/payments/settings', apiVersion: 'v1' },
  { collection: 'secrets', kind: 'Secret', name: 'credentials', namespace: 'payments', path: '/config/secrets/payments/credentials', apiVersion: 'v1' },
  { collection: 'cluster-roles', kind: 'ClusterRole', name: 'system:discovery', namespace: null, path: '/access/cluster-roles/system%3Adiscovery', apiVersion: 'rbac.authorization.k8s.io/v1' },
]) {
  test(`${target.kind} YAML edits in the fixed detail pane and retains conflicts`, async ({ page }, testInfo) => {
    const document = JSON.stringify({ apiVersion: target.apiVersion, kind: target.kind, metadata: { name: target.name, ...(target.namespace ? { namespace: target.namespace } : {}), uid: 'uid-api', resourceVersion: '17', labels: { edit: 'before' } } }, null, 2)
    let reads = 0, writes = 0
    await page.route(`**/api/v1/resources/${target.collection}/**/yaml`, async (route) => {
      if (route.request().method() === 'PUT') {
        writes++
        expect(route.request().headers()['x-kubepeep-csrf']).toBe('csrf_workspace')
        expect(route.request().postDataJSON()).toMatchObject({ action: 'updateResource', target: { namespace: target.namespace ?? '', kind: target.kind, name: target.name }, expectedUid: 'uid-api', expectedResourceVersion: '17' })
        await route.fulfill(writes === 1 ? { status: 409, json: { code: 'CONFLICT', message: 'Changed' } } : { json: { data: { accepted: true, resourceVersion: '18' } } })
      } else {
        reads++
        await route.fulfill({ json: { data: { yaml: document, kind: target.kind, updateCapability: `yaml.${target.collection}.update`, generation } } })
      }
    })
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(target.path)
    await page.getByRole('tab', { name: 'YAML', exact: true }).click()
    expect(reads).toBe(0)
    await page.getByRole('button', { name: 'Load authorized YAML' }).click()
    await page.getByRole('button', { name: 'Edit YAML', exact: true }).click()
    const editor = page.getByLabel(`${target.kind} YAML`, { exact: true })
    const draft = document.replace('"before"', '"after"') + '\n# editable line\n'.repeat(100)
    await editor.fill(draft)
    const save = page.getByRole('button', { name: `Save ${target.kind}`, exact: true })
    await save.click()
    await expect(page.getByRole('alert')).toContainText('draft is preserved')
    await expect(editor).toHaveValue(draft)
    await expectFixedSplit(page)
    await expect(save).toBeInViewport()
    if (target.kind === 'Pod') {
      await page.screenshot({ path: testInfo.outputPath('yaml-editor-desktop.png') })
      await page.setViewportSize({ width: 390, height: 844 })
      await expectFixedSplit(page)
      await expect(editor).toBeInViewport({ ratio: 0.99 })
      await expect(save).toBeInViewport()
      await page.screenshot({ path: testInfo.outputPath('yaml-editor-mobile.png') })
    }
    await page.getByRole('tab', { name: 'Overview', exact: true }).click()
    await page.getByRole('tab', { name: 'YAML', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Edit YAML', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: 'Load authorized YAML' }).click()
    await page.getByRole('button', { name: 'Edit YAML', exact: true }).click()
    await editor.fill(document.replace('"before"', '"after"'))
    await save.click()
    await expect(page.getByText(`${target.kind} saved. Load YAML to inspect the new version.`)).toBeVisible()
    expect(reads).toBe(2)
    expect(writes).toBe(2)
  })
}

for (const [kind, plural] of [['Deployment', 'deployments'], ['ReplicaSet', 'replicasets'], ['DaemonSet', 'daemonsets'], ['StatefulSet', 'statefulsets'], ['Job', 'jobs'], ['CronJob', 'cronjobs']]) {
  test(`${kind} loads its own detail including nullable workload arrays`, async ({ page }) => {
    const row = { ...metadata, kind, ready: 1, desired: 1, available: 1, updated: 1, status: 'Healthy', ageSeconds: 60 }
    await page.route('**/api/v1/workloads?*', (route) => route.fulfill({ json: { data: [row], meta: { generation, page: collection } } }))
    await page.route(`**/api/v1/workloads/${plural}/payments/api`, (route) => route.fulfill({ json: { data: { ...row, metadata, containers: [spec], conditions: null, related: null }, meta: { generation } } }))
    await page.goto(`/workloads/kind/${plural}`)
    await page.getByRole('button', { name: `Open ${kind} api in payments`, exact: true }).click()
    const panel = page.locator('.workspace-panel')
    await expect(panel.getByRole('tabpanel')).toContainText('example/api:1')
    await expect(panel).toHaveAttribute('aria-label', `${kind} api`)
    await expectFixedSplit(page)
    await panel.getByRole('button', { name: 'Close resource workspace' }).click()
    await expect(panel).toHaveCount(0)
    await page.reload()
    await expect(page.locator('.resource-detail-slot')).toBeHidden()
    const inventory = (await page.locator('.resource-collection').boundingBox())!
    const pane = (await page.locator('.resource-list-pane').boundingBox())!
    expect(inventory.y + inventory.height).toBeCloseTo(pane.y + pane.height, 0)
  })
}

test('Gateway relationships switch namespace, exact inventory and cluster scope', async ({ page }) => {
  await page.route('**/api/v1/namespace-scopes/1', (route) => route.fulfill({ json: { data: { namespaces: ['payments', 'infra'], defaultNamespace: 'payments' } } }))
  const reads: URL[] = []
  const routeRow = { ...metadata, kind: 'HTTPRoute', apiVersion: 'gateway.networking.k8s.io/v1', status: 'Ready', hosts: [], addresses: [], conditions: [], related: [{ apiGroup: 'gateway.networking.k8s.io', kind: 'Gateway', namespace: 'infra', name: 'edge' }] }
  const gateway = { ...routeRow, namespace: 'infra', name: 'edge', kind: 'Gateway', className: 'public', related: [{ apiGroup: 'gateway.networking.k8s.io', kind: 'GatewayClass', namespace: '', name: 'public' }] }
  await page.route('**/api/v1/http-routes**', (route) => route.fulfill({ json: { data: new URL(route.request().url()).pathname.endsWith('/api') ? routeRow : [routeRow], meta: { generation, page: collection } } }))
  await page.route('**/api/v1/gateways**', (route) => {
    const url = new URL(route.request().url()); reads.push(url)
    return route.fulfill({ json: { data: url.pathname.endsWith('/edge') ? gateway : [gateway], meta: { generation, page: collection } } })
  })
  await page.route('**/api/v1/gateway-classes**', (route) => {
    const url = new URL(route.request().url()); reads.push(url)
    const item = { ...gateway, namespace: '', kind: 'GatewayClass', name: 'public', related: [] }
    return route.fulfill({ json: { data: url.pathname.endsWith('/public') ? item : [item], meta: { generation, page: collection } } })
  })
  await page.goto('/network/gateway-api/http-routes')
  await page.getByRole('button', { name: 'Open HTTPRoute api', exact: true }).click()
  const panel = page.locator('.workspace-panel')
  await panel.getByRole('button', { name: 'Open Gateway edge', exact: true }).click()
  await expect(panel).toHaveAttribute('aria-label', 'Gateway edge')
  await expect(page.getByLabel('Global namespace', { exact: true })).toHaveValue('infra')
  await expect.poll(() => reads.some((url) => url.pathname === '/api/v1/gateways' && url.searchParams.get('namespace') === 'infra' && url.searchParams.get('fieldSelector') === 'metadata.name=edge')).toBe(true)
  await panel.getByRole('button', { name: 'Open GatewayClass public', exact: true }).click()
  await expect(panel).toHaveAttribute('aria-label', 'GatewayClass public')
  await expect.poll(() => reads.some((url) => url.pathname === '/api/v1/gateway-classes' && !url.searchParams.has('namespace') && url.searchParams.get('fieldSelector') === 'metadata.name=public')).toBe(true)
  await expectFixedSplit(page)
})

test('100 loaded Pods authorize and delete all exact targets in bounded batches', async ({ page }) => {
  const rows = Array.from({ length: 100 }, (_, index) => ({ ...pod, name: `pod-${index.toString().padStart(3, '0')}` }))
  const permissionBatches: string[][] = []
  const deleted: string[] = []
  await page.route('**/api/v1/pods?*', (route) => route.fulfill({ json: { data: rows.filter((row) => !deleted.includes(row.name)), meta: { generation, page: collection } } }))
  await page.route('**/api/v1/permissions?*', (route) => {
    const url = new URL(route.request().url()), names = url.searchParams.getAll('resourceName')
    permissionBatches.push(names)
    expect(names.length).toBeLessThanOrEqual(20)
    return route.fulfill({ json: { data: { generation, complete: true, truncated: false, errors: [], decisions: names.flatMap((resourceName) => url.searchParams.getAll('capability').map((capabilityId) => ({ capabilityId, namespace: 'payments', resourceName, decision: 'allowed' }))) } } })
  })
  await page.route('**/api/v1/pods/payments/**', (route) => {
    const path = new URL(route.request().url()).pathname, name = path.split('/')[5]
    if (route.request().method() !== 'GET') {
      expect(route.request().headers()['x-kubepeep-csrf']).toBe('csrf_workspace')
      expect(route.request().postDataJSON()).toMatchObject({ confirmed: true, expectedUid: `uid-${name}`, expectedResourceVersion: '17', expectedGeneration: generation, target: { name, namespace: 'payments', kind: 'Pod' } })
      deleted.push(name)
      return route.fulfill({ json: { data: { accepted: true, generation } } })
    }
    return route.fulfill({ json: { data: { metadata: { ...metadata, name, uid: `uid-${name}` }, summary: { ...pod, name }, conditions: [], containers: [], initContainers: [], ephemeralContainers: [], relatedEvents: [] }, meta: { generation } } })
  })
  await page.goto('/pods')
  await page.getByRole('checkbox', { name: 'Select all loaded rows' }).check()
  await expect(page.getByRole('toolbar', { name: 'Bulk actions' })).toContainText('100 selected')
  await expect(page.getByRole('button', { name: 'Restart selected' })).toBeEnabled()
  const remove = page.getByRole('button', { name: 'Delete selected', exact: true })
  await expect(remove).toBeEnabled()
  expect(new Set(permissionBatches.flat()).size).toBe(100)
  await remove.click()
  const dialog = page.getByRole('alertdialog', { name: 'Delete 100 Pods' })
  await dialog.getByRole('checkbox', { name: 'I understand this action cannot be undone.' }).check()
  await dialog.getByRole('button', { name: 'Delete selected' }).click()
  await expect(page.getByText('Deleted 100 Pods', { exact: true })).toBeVisible({ timeout: 20_000 })
  expect(new Set(deleted).size).toBe(100)
})

test('HPA replica capacity uses green, yellow and red percentage bars', async ({ page }) => {
  await page.route('**/api/v1/hpas?*', (route) => route.fulfill({ json: { data: [20, 80, 95].map((currentReplicas) => ({ ...metadata, name: `scale-${currentReplicas}`, minReplicas: 1, maxReplicas: 100, currentReplicas, desiredReplicas: currentReplicas, targetKind: 'Deployment', targetName: 'api', resourceTargets: [], conditions: [], metricNames: [] })), meta: { generation, page: collection } } }))
  await page.goto('/configuration/hpas')
  for (const [value, tone] of [[20, 'healthy'], [80, 'warning'], [95, 'danger']] as const) {
    const row = page.getByRole('row').filter({ has: page.getByRole('button', { name: `Open autoscaler scale-${value} in payments` }) })
    const meter = row.getByRole('meter', { name: 'HPA replica capacity' })
    await expect(meter).toHaveAttribute('aria-valuenow', `${value}`)
    await expect(meter.locator('..')).toHaveAttribute('data-tone', tone)
  }
})

test('visited Pods stay warm behind Deployments and return without a loading gap', async ({ page }) => {
  await page.clock.install()
  let reads = 0
  await page.route('**/api/v1/pods?*', (route) => {
    reads++
    return route.fulfill({ json: { data: [{ ...pod, name: `warm-${reads}` }], meta: { generation, page: collection } } })
  })
  await page.goto('/pods')
  await expect(page.getByRole('button', { name: 'Open Pod warm-1 in payments' })).toBeVisible()
  const menu = page.getByRole('navigation', { name: 'Workloads resources', exact: true })
  await menu.getByRole('link', { name: 'Deployments', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Open Deployment api in payments' })).toBeVisible()
  await page.clock.fastForward(10_001)
  await expect.poll(() => reads).toBe(2)
  await menu.getByRole('link', { name: 'Pods', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Open Pod warm-2 in payments' })).toBeVisible()
  expect(reads).toBe(2)
})

test('Pods show starting separately and container state dots; column order survives reload', async ({ page }) => {
  const containers = [
    { name: 'app', type: 'regular', state: 'running', status: 'running' },
    { name: 'sidecar', type: 'regular', state: 'waiting', status: 'starting', reason: 'ContainerCreating' },
    { name: 'setup', type: 'init', state: 'terminated', status: 'inactive', reason: 'Completed' },
  ]
  let prefs = { version: 1, ui: { language: 'en' }, columns: { hidden: {}, order: {} } }
  await page.route('**/api/v1/preferences', async (route) => {
    if (route.request().method() === 'PUT') prefs = route.request().postDataJSON()
    await route.fulfill({ json: { data: prefs } })
  })
  await page.route('**/api/v1/pods?*', (route) => route.fulfill({ json: { data: [{ ...pod, starting: true, problematic: false, containers, containerCount: 3 }], meta: { generation, page: collection } } }))
  await page.goto('/pods')
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table.getByText('Starting', { exact: true })).toBeVisible()
  await expect(table.getByText('problem', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Choose visible columns', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Choose visible columns', exact: true })).toBeVisible()
  expect(await table.locator('.table-column-chooser').evaluate(element => element.scrollLeft)).toBe(0)
  await page.getByRole('checkbox', { name: 'Containers', exact: true }).check()
  await page.keyboard.press('Escape')
  const cell = table.getByRole('cell').filter({ has: page.locator('[title^="app (regular)"]') })
  await expect(cell).toContainText('3')
  await expect(cell.locator('[title^="app (regular)"]')).toHaveClass(/bg-kp-green/)
  await expect(cell.locator('[title^="sidecar (regular)"]')).toHaveClass(/bg-kp-yellow/)
  await expect(cell.locator('[title^="setup (init)"]')).toHaveClass(/bg-kp-overlay/)
  await page.getByRole('button', { name: 'Choose visible columns', exact: true }).click()
  for (let index = 0; index < 5; index++) {
    await page.getByRole('button', { name: 'Move Containers left', exact: true }).click()
    await expect.poll(() => (prefs.columns.order as Record<string, string[]>).pods?.indexOf('containers')).toBe(7 - index)
  }
  await expect.poll(() => Object.keys(prefs.columns.order)).toContain('pods')
  await page.keyboard.press('Escape')
  const before = await table.getByRole('columnheader').allTextContents()
  expect(before.findIndex((value) => value.includes('Containers'))).toBeLessThan(before.findIndex((value) => value.includes('Ready')))
  await page.reload()
  await expect(table).toBeVisible()
  await expect.poll(() => table.getByRole('columnheader').allTextContents()).toEqual(before)
})

test('automatic refresh defaults to ten seconds, stops globally, and resumes on enable', async ({ page }) => {
  await page.clock.install()
  let reads = 0
  await page.route('**/api/v1/pods?*', (route) => {
    reads++
    return route.fulfill({ json: { data: [pod], meta: { generation, page: collection } } })
  })
  await page.goto('/pods')
  await expect(page.getByRole('table', { name: 'Authorized Pod pages' })).toBeVisible()
  const toggle = page.getByRole('button', { name: 'Automatic refresh every 10 seconds' })
  await expect(toggle).toHaveAttribute('aria-pressed', 'true')
  expect(reads).toBe(1)
  await page.clock.fastForward(9_000)
  expect(reads).toBe(1)
  await page.clock.fastForward(1_000)
  await expect.poll(() => reads).toBe(2)
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-pressed', 'false')
  await page.clock.fastForward(30_000)
  expect(reads).toBe(2)
  await toggle.click()
  await page.clock.fastForward(10_000)
  await expect.poll(() => reads).toBe(3)
  await page.reload()
  await expect(toggle).toHaveAttribute('aria-pressed', 'true')
})

test('kubeconfig import browses files, retains conflict drafts and makes a new source selectable', async ({ page }, testInfo) => {
  const profiles = [{ id: 1, name: 'Current', context: 'development', isDefault: true, kubeconfigFiles: [{ position: 0, displayPath: '~/.kube/current' }] }]
  await page.route('**/api/v1/cluster/profiles', (route) => route.fulfill({ json: { data: profiles } }))
  await page.route('**/api/v1/contexts?*', (route) => route.fulfill({ json: { data: [{ clusterProfileId: Number(new URL(route.request().url()).searchParams.get('clusterProfileId')), name: 'development', cluster: 'dev', selected: true }] } }))
  let saved = false
  await page.route('**/api/v1/cluster/profiles/import', async (route) => {
    expect(route.request().headers()['x-kubepeep-csrf']).toBe('csrf_workspace')
    const body = route.request().postDataJSON()
    if (!saved) {
      saved = true
      expect(body.content).toContain('example.invalid')
      await route.fulfill({ status: 409, json: { code: 'KUBECONFIG_CONFLICT', message: 'Existing entries were preserved.' } })
      return
    }
    expect(body).toEqual({ path: '~/clusters/dev.yaml' })
    const profile = { id: 2, name: 'Imported', context: 'development', isDefault: false, kubeconfigFiles: [{ position: 0, displayPath: '~/clusters/dev.yaml' }] }
    profiles.push(profile)
    await route.fulfill({ json: { data: profile } })
  })
  await page.goto('/pods')
  await page.getByRole('button', { name: 'Add kubeconfig' }).click()
  const dialog = page.getByRole('dialog', { name: 'Add kubeconfig' })
  await expect(dialog).toBeVisible()
  const content = 'apiVersion: v1\nkind: Config\nclusters: [{name: dev, cluster: {server: https://example.invalid}}]'
  await dialog.getByLabel('Kubeconfig file', { exact: true }).setInputFiles({ name: 'config.yaml', mimeType: 'application/yaml', buffer: Buffer.from(content) })
  await expect(dialog.getByLabel('Kubeconfig YAML')).toHaveValue(content)
  await page.screenshot({ path: testInfo.outputPath('kubeconfig-import-desktop.png') })
  await dialog.getByRole('button', { name: 'Save kubeconfig' }).click()
  await expect(dialog.getByRole('alert')).toContainText('Existing entries were preserved')
  await expect(dialog.getByLabel('Kubeconfig YAML')).toHaveValue(content)
  await dialog.getByRole('button', { name: 'Use local path' }).click()
  await dialog.getByLabel('Local kubeconfig path').fill('~/clusters/dev.yaml')
  await dialog.getByRole('button', { name: 'Add source', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByLabel('Kubeconfig source', { exact: true })).toHaveValue('2')
  await expect(page.getByLabel('Kubernetes context', { exact: true })).toHaveValue('')
  await expect(page.getByLabel('Kubernetes context', { exact: true }).getByRole('option', { name: 'development · dev' })).toHaveCount(1)
})

test('context colors persist and isolate identical names in different sources', async ({ page }, testInfo) => {
  let profileId = 1
  let prefs = { version: 1, ui: { language: 'en', contextColors: [] as Array<{ clusterProfileId: number; context: string; color: string }> } }
  await page.route('**/api/v1/preferences', async (route) => {
    if (route.request().method() === 'PUT') prefs = route.request().postDataJSON()
    await route.fulfill({ json: { data: prefs } })
  })
  await page.route('**/api/v1/status', (route) => route.fulfill({ json: { data: { version: 'test', components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((key) => [key, { status: 'healthy' }])), selection: { clusterProfileId: profileId, context: 'development', cluster: 'dev', scopeId: 1, scopeMode: 'list', defaultNamespace: 'payments', namespaceCount: 1, generation } } } }))
  await page.route('**/api/v1/cluster/profiles', (route) => route.fulfill({ json: { data: [1, 2].map((id) => ({ id, name: `Source ${id}`, context: 'development', isDefault: id === 1, kubeconfigFiles: [{ position: 0, displayPath: `~/.kube/source-${id}` }] })) } }))
  await page.route('**/api/v1/contexts?*', (route) => route.fulfill({ json: { data: [{ clusterProfileId: Number(new URL(route.request().url()).searchParams.get('clusterProfileId')), name: 'development', cluster: 'dev', selected: true }] } }))
  await page.route('**/api/v1/contexts/select', (route) => {
    profileId = route.request().postDataJSON().clusterProfileId
    return route.fulfill({ json: { data: { clusterProfileId: profileId, context: 'development', cluster: 'dev', generation } } })
  })
  await page.goto('/pods')
  const shell = page.locator('.app-shell')
  await page.getByRole('button', { name: 'Context color', exact: true }).click()
  await page.getByRole('button', { name: 'Development — blue' }).click()
  await expect(shell).toHaveAttribute('data-context-color', '#38bdf8')
  await page.keyboard.press('Escape')
  await page.reload()
  await expect(shell).toHaveAttribute('data-context-color', '#38bdf8')
  await page.getByLabel('Kubeconfig source', { exact: true }).selectOption('2')
  await page.getByLabel('Kubernetes context', { exact: true }).selectOption('development')
  await expect(shell).not.toHaveAttribute('data-context-color')
  await page.getByRole('button', { name: 'Context color', exact: true }).click()
  await page.getByRole('button', { name: 'Production — red' }).click()
  await expect(shell).toHaveAttribute('data-context-color', '#f87171')
  await page.keyboard.press('Escape')
  await page.screenshot({ path: testInfo.outputPath('context-red-desktop.png') })
  expect(prefs.ui.contextColors).toHaveLength(2)
  await page.getByLabel('Kubeconfig source', { exact: true }).selectOption('1')
  await page.getByLabel('Kubernetes context', { exact: true }).selectOption('development')
  await expect(shell).toHaveAttribute('data-context-color', '#38bdf8')
})
