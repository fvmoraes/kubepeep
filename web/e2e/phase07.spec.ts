import { expect, test, type Page } from '@playwright/test'

const viewports = [
  { name: '320x568', width: 320, height: 568 },
  { name: '390x844', width: 390, height: 844 },
  { name: '768x1024', width: 768, height: 1024 },
  { name: '844x390', width: 844, height: 390 },
  { name: '1366x768', width: 1366, height: 768 },
  { name: '1440x900', width: 1440, height: 900 },
  { name: '1920x1080', width: 1920, height: 1080 },
  { name: '2560x1440', width: 2560, height: 1440 },
] as const

const navigationInventory = [
  '/', '/nodes', '/events', '/namespaces', '/leases', '/workloads',
  '/workloads/kind/deployments', '/pods', '/workloads/kind/replicasets',
  '/workloads/kind/daemonsets', '/workloads/kind/statefulsets',
  '/workloads/kind/jobs', '/workloads/kind/cronjobs', '/workloads/custom', '/network/services',
  '/network/endpoints', '/network/endpoint-slices', '/network/ingresses',
  '/network/ingress-classes', '/network/network-policies', '/network/port-forwards',
  '/network/gateway-api/gateways',
  '/helm/releases/secrets',
  '/config/configmaps', '/config/secrets', '/configuration/resource-quotas',
  '/configuration/limit-ranges', '/configuration/hpas', '/configuration/pdbs',
  '/storage/persistent-volumes', '/storage/persistent-volume-claims',
  '/storage/volume-attachments', '/storage/storage-classes', '/storage/csi-nodes',
  '/storage/csi-drivers', '/service-accounts', '/access/roles',
  '/access/role-bindings', '/access/cluster-roles', '/access/cluster-role-bindings',
  '/permissions', '/logs', '/administration/customresourcedefinitions',
  '/administration/priority-classes', '/administration/runtime-classes',
  '/administration/mutating-webhook-configurations', '/administration/validating-webhook-configurations', '/settings',
] as const

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

function status(selection: Record<string, unknown> | null) {
  return {
    version: '0.7.0-rc.1', commit: 'phase07', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection,
  }
}

async function expandSidebarGroups(page: Page) {
  await page.getByRole('navigation', { name: 'Primary navigation' }).evaluate((nav) => {
    nav.querySelectorAll<HTMLButtonElement>('button[aria-expanded="false"]').forEach((button) => button.click())
  })
}

test.beforeEach(async ({ context }) => {
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const generation = 'gen_phase07_inventory'
    let data: unknown = []
    let meta: Record<string, unknown> = { generation }
    if (url.pathname === '/api/v1/status') data = status({
      clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep',
      scopeId: 7, scopeName: 'Finance', scopeMode: 'list', scopeSource: 'saved',
      defaultNamespace: 'payments', namespaceCount: 2, generation,
    })
    else if (url.pathname === '/api/v1/preferences') data = preferences
    else if (url.pathname === '/api/v1/session') data = { csrfToken: 'csrf_phase07', origin: 'http://127.0.0.1:4173', generation, expiresAt: '2026-09-30T18:00:00Z' }
    else if (url.pathname === '/api/v1/stream' || url.pathname.endsWith('/logs/stream')) {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: `event: complete\ndata: ${JSON.stringify({ generation, topic: url.searchParams.get('topic'), completedNamespaces: 2, requestedNamespaces: 2 })}\n\n` })
      return
    } else if (url.searchParams.has('limit')) {
      meta = { generation, page: { limit: Number(url.searchParams.get('limit')), next: '', complete: true, truncated: false, filterScope: 'collection' }, coverage: { requestedNamespaces: 2, completedNamespaces: 2, deniedNamespaces: [], failed: [] } }
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta }) })
  })
})

for (const viewport of viewports) {
  test(`reaudits every navigation click without overflow at ${viewport.name}`, async ({ page }) => {
    test.setTimeout(120_000)
    await page.setViewportSize(viewport)
    await page.goto('/')
    for (const path of navigationInventory) {
      const menuTrigger = page.getByRole('button', { name: 'Open navigation', exact: true })
      if (await menuTrigger.isVisible()) await menuTrigger.click()
      await expandSidebarGroups(page)
      const navigation = page.getByRole('navigation', { name: 'Primary navigation', includeHidden: true })
      const link = navigation.locator(`a[href="${path}"]`)
      await expect(link).toBeVisible()
      await link.click()
      await expect(page).toHaveURL(path === '/' ? /\/$/ : new RegExp(`${path.replace(/\//g, '\\/')}$`))
      await expect(link).toHaveAttribute('aria-current', 'page')
      await expect(page.locator('.resource-family-nav a[aria-current="page"]')).toBeInViewport({ ratio: 0.95 })
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
      expect(overflow, `${path} overflow at ${viewport.name}`).toBeLessThanOrEqual(1)
    }
  })
}

test('activates each persisted context default before loading resources and restores it after reopen', async ({ page, context }) => {
  let current = {
    clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep',
    scopeId: null as number | null, scopeName: null as string | null, scopeMode: null as string | null,
    scopeSource: 'none', defaultNamespace: null as string | null, namespaceCount: 0, generation: 'gen_dev_open',
  }
  const selectedIDs: number[] = []
  const prematureResourceRequests: string[] = []

  await context.unroute('**/api/v1/**')
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const expectedID = current.context === 'production' ? 10 : 9
    const expectedName = current.context === 'production' ? 'Production' : 'Finance'
    let data: unknown = []
    let meta: Record<string, unknown> = { generation: current.generation }
    if (url.pathname === '/api/v1/status') data = status(current)
    else if (url.pathname === '/api/v1/preferences') data = preferences
    else if (url.pathname === '/api/v1/cluster/profiles') data = []
    else if (url.pathname === '/api/v1/namespace-scopes') data = [{
      id: expectedID, clusterProfileId: 1, context: current.context, name: expectedName,
      mode: 'list', namespaces: [current.context === 'production' ? 'prod' : 'payments'],
      defaultNamespace: current.context === 'production' ? 'prod' : 'payments', isDefault: true,
      version: 1, createdAt: '2026-09-29T12:00:00Z', updatedAt: '2026-09-29T12:00:00Z',
    }]
    else if (url.pathname === '/api/v1/session') data = { csrfToken: 'csrf_default', origin: 'http://127.0.0.1:4173', generation: current.generation, expiresAt: '2026-09-30T18:00:00Z' }
    else if (/^\/api\/v1\/namespace-scopes\/\d+\/select$/.test(url.pathname) && request.method() === 'POST') {
      const id = Number(url.pathname.split('/')[4])
      selectedIDs.push(id)
      current = {
        ...current, scopeId: id, scopeName: expectedName, scopeMode: 'list', scopeSource: 'saved',
        defaultNamespace: current.context === 'production' ? 'prod' : 'payments', namespaceCount: 1,
        generation: `${current.generation}_active`,
      }
      data = current
      meta = { generation: current.generation }
    } else if (url.pathname === '/api/v1/pods' && url.searchParams.has('limit')) {
      if (current.scopeId === null) prematureResourceRequests.push(url.pathname)
      meta = { generation: current.generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }, coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] } }
    } else if (url.pathname === '/api/v1/stream') {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: `event: complete\ndata: ${JSON.stringify({ generation: current.generation, topic: 'pods', completedNamespaces: 1, requestedNamespaces: 1 })}\n\n` })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta }) })
  })

  await page.goto('/pods')
  await expect.poll(() => selectedIDs).toEqual([9])
  await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()

  current = {
    ...current, context: 'production', cluster: 'prod-cluster', scopeId: null, scopeName: null,
    scopeMode: null, scopeSource: 'none', defaultNamespace: null, namespaceCount: 0,
    generation: 'gen_prod_open',
  }
  await page.reload()
  await expect.poll(() => selectedIDs).toEqual([9, 10])
  await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()
  expect(prematureResourceRequests).toEqual([])
})
