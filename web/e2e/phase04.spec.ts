import { expect, test } from '@playwright/test'

const generation = 'gen_phase04'
const viewports = [
  { name: '1366x768', width: 1366, height: 768 },
  { name: '1440x900', width: 1440, height: 900 },
  { name: '1920x1080', width: 1920, height: 1080 },
  { name: '2560x1440', width: 2560, height: 1440 },
] as const

const status = {
  version: 'test', commit: 'phase04', buildDate: 'test', port: 2748,
  components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
  selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 7, scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 2, generation },
}

const preferences = {
  version: 1,
  ui: { language: 'en' },
  logs: { wrap: false, timestamps: true, tailLines: 200 },
  dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] },
  filters: {
    workloads: { version: 1, items: [] }, pods: { version: 1, items: [] },
    events: { version: 1, items: [] }, logs: { version: 1, items: [] },
  },
  columns: { hidden: {} },
}

const pods = Array.from({ length: 30 }, (_, index) => ({
  namespace: index % 2 ? 'billing' : 'payments',
  name: `api-${String(index + 1).padStart(2, '0')}`,
  status: index === 3 ? 'Pending' : 'Running',
  ready: { current: index === 3 ? 0 : 1, desired: 1 },
  restarts: index === 5 ? 4 : 0,
  node: `worker-${index % 3 + 1}`,
  ip: `10.0.0.${index + 10}`,
  owner: { kind: 'Deployment', name: 'api' },
  ageSeconds: 60 + index * 17,
  problematic: index === 3 || index === 5,
}))

test.beforeEach(async ({ context }) => {
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    let data: unknown = []
    let meta: Record<string, unknown> = { generation }
    if (url.pathname === '/api/v1/status') data = status
    else if (url.pathname === '/api/v1/preferences') data = preferences
    else if (url.pathname === '/api/v1/cluster/profiles') data = []
    else if (url.pathname === '/api/v1/namespace-scopes/7') data = { namespaces: ['payments', 'billing'] }
    else if (url.pathname === '/api/v1/session') data = { csrfToken: 'csrf_phase04', origin: 'http://127.0.0.1:4173', generation, expiresAt: '2026-09-29T18:00:00Z' }
    else if (url.pathname === '/api/v1/pods' && url.searchParams.has('limit')) {
      const namespace = url.searchParams.getAll('namespace')
      const search = (url.searchParams.get('search') ?? '').toLowerCase()
      data = pods.filter((pod) => (namespace.length === 0 || namespace.includes(pod.namespace)) && (!search || pod.name.includes(search)))
      meta = { generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }, coverage: { requestedNamespaces: 2, completedNamespaces: 2, deniedNamespaces: [], failed: [] } }
    } else if (url.pathname === '/api/v1/stream') {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: `event: complete\ndata: ${JSON.stringify({ generation, topic: url.searchParams.get('topic'), completedNamespaces: 2, requestedNamespaces: 2 })}\n\n` })
      return
    } else if (/^\/api\/v1\/pods\/(payments|billing)\/api-\d+$/.test(url.pathname)) {
      const summary = pods.find((pod) => url.pathname.endsWith(`/${pod.name}`)) ?? pods[0]
      data = {
        metadata: { namespace: summary.namespace, name: summary.name, uid: `uid-${summary.name}`, resourceVersion: '17', creationTimestamp: '2026-09-29T10:00:00Z', labels: { app: 'api' } },
        summary, conditions: [{ type: 'Ready', status: summary.status === 'Running' ? 'True' : 'False', reason: 'ContainersReady', message: null, lastTransitionTime: '2026-09-29T10:00:00Z' }],
        containers: [{ spec: { name: 'api', image: 'example/api:1', ports: [{ name: 'http', containerPort: 8080, protocol: 'TCP' }] }, type: 'regular', ready: true, restartCount: summary.restarts, state: 'running', reason: null }],
        initContainers: [], ephemeralContainers: [], relatedEvents: [],
      }
    }
    if (request.method() === 'GET' || request.method() === 'PUT' || request.method() === 'POST' || request.method() === 'DELETE') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta }) })
    }
  })
})

for (const viewport of viewports) {
  test(`uses the full content width without global overflow at ${viewport.name}`, async ({ page }) => {
    await page.setViewportSize(viewport)
    await page.goto('/pods')
    const table = page.getByRole('table', { name: 'Authorized Pod pages' })
    await expect(table).toBeVisible()
    expect(await table.getByRole('row').count()).toBeGreaterThan(5)
    expect(await table.getByRole('row').count()).toBeLessThan(32)
    await expect(page.getByRole('button', { name: 'Choose visible columns' })).toBeVisible()

    const measurements = await page.evaluate(() => {
      const controls = document.querySelector<HTMLElement>('[aria-label="Resource list controls"]')
      const main = document.querySelector<HTMLElement>('main')
      const table = document.querySelector<HTMLElement>('table')
      const controlsBox = controls?.getBoundingClientRect()
      const mainBox = main?.getBoundingClientRect()
      const tableBox = table?.getBoundingClientRect()
      return {
        viewportWidth: document.documentElement.clientWidth,
        documentWidth: document.documentElement.scrollWidth,
        controlsHeightRatio: controlsBox && mainBox ? controlsBox.height / mainBox.height : 1,
        contentWidthRatio: tableBox && mainBox ? tableBox.width / mainBox.width : 0,
      }
    })
    expect(measurements.documentWidth).toBeLessThanOrEqual(measurements.viewportWidth + 1)
    expect(measurements.controlsHeightRatio).toBeLessThanOrEqual(0.30)
    expect(measurements.contentWidthRatio).toBeGreaterThanOrEqual(0.70)
    console.log(`Phase 04 viewport ${viewport.name}: ${JSON.stringify(measurements)}`)
    await page.screenshot({ path: `/tmp/kubepeep-phase04-${viewport.name}.png` })
  })
}

test('keeps filters functional and exposes the complete Pod workspace tabs', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/pods')
  await expect(page.getByRole('table', { name: 'Authorized Pod pages' })).toBeVisible()
  await page.getByLabel('Search resources').fill('api-01')
  await expect(page.getByRole('button', { name: 'Open Pod api-01 in payments' })).toBeVisible()
  await expect(page.getByRole('button', { name: /Open Pod/ })).toHaveCount(1)

  await page.getByRole('button', { name: 'Open Pod api-01 in payments' }).click()
  const workspace = page.getByRole('region', { name: 'Pod api-01' })
  await expect(workspace).toBeVisible()
  for (const tab of ['Overview', 'Investigation', 'Logs', 'YAML', 'Events', 'Metrics', 'Containers', 'Data / Env', 'Actions']) {
    await expect(workspace.getByRole('tab', { name: tab })).toBeVisible()
  }
  await workspace.getByRole('tab', { name: 'Containers' }).click()
  await expect(workspace.getByRole('columnheader', { name: 'Image' })).toBeVisible()
})
