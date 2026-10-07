import { expect, test, type Page } from '@playwright/test'
import { readFile } from 'node:fs/promises'

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
  expect(detail!.height / (list!.height + detail!.height)).toBeCloseTo(page.viewportSize()!.height <= 600 ? 0.7 : 0.6, 2)
  expect(detail!.y).toBeGreaterThanOrEqual(list!.y + list!.height)
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
  const list = page.getByRole('region', { name: 'Page content', exact: true })
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
    await expect(warning).toHaveAttribute('title', 'restricted: AUTHORIZATION_UNAVAILABLE')
    await expect(page.getByText('Resource request failed', { exact: true })).toHaveCount(0)
    await page.clock.fastForward(16_000)
    await expect.poll(() => requests).toBe(4)
    await expect(table).toBeVisible()
    await expect(page.getByRole('button', { name: 'Load next page' })).toBeDisabled()
  })
}

for (const [status, code] of [[401, 'AUTHENTICATION_UNAVAILABLE'], [403, 'FORBIDDEN'], [409, 'GENERATION_CHANGED']] as const) {
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

for (const initiallyUnavailable of [true, false]) {
  test(`recovers from unavailable Pod authorization ${initiallyUnavailable ? 'on initial load' : 'after loading rows'} without navigation`, async ({ page }) => {
    await page.clock.install()
    let unavailable = initiallyUnavailable
    let requests = 0
    let recovering = false
    let releaseRecovery!: () => void
    const recovery = new Promise<void>((resolve) => { releaseRecovery = resolve })
    await page.route('**/api/v1/pods?*', async (route) => {
      requests++
      if (unavailable) {
        await route.fulfill({ status: 503, json: { code: 'AUTHORIZATION_UNAVAILABLE', message: 'Authorization could not be confirmed.' } })
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
    await expect(page.getByText('AUTHORIZATION_UNAVAILABLE', { exact: true })).toBeVisible()
    await expect(table).toHaveCount(0)
    await expect(page.getByLabel('Resource live updates')).toHaveText('Auto · 15s')
    // A cached unknown decision can outlive the first interval. Keep retrying
    // without displaying cached rows or issuing an immediate retry burst.
    for (let attempt = 0; attempt < 2; attempt++) {
      const previousRequests = requests
      await page.clock.fastForward(16_000)
      await expect.poll(() => requests).toBe(previousRequests + 1)
      await expect(page.getByText('AUTHORIZATION_UNAVAILABLE', { exact: true })).toBeVisible()
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
    await expect(page.getByText('AUTHORIZATION_UNAVAILABLE', { exact: true })).toHaveCount(0)
    await expect(page).toHaveURL(/\/pods$/)
  })
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
    await expectFixedSplit(page)
    await expect(table).toBeVisible()
    expect((await panel.boundingBox())!.y).toBeGreaterThan((await table.boundingBox())!.y)
    await panel.getByRole('button', { name: 'Close resource workspace' }).click()
    await expect(page).toHaveURL(new RegExp(`${example.path}$`))
    await expect(table).toBeVisible()
  })
}

for (const target of [
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
