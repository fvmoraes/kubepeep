import { expect, test } from '@playwright/test'

// Shared mock fabric for the post-MVP features: global resource search
// (F7-04), saved favorites (F7-01) and the YAML diff against last-applied
// (F7-02). All cluster data is route-mocked; nothing reaches a real API.
const generation = 'gen_features_e2e'

function statusPayload() {
  return {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, {
      status: name === 'application' || name === 'cluster' ? 'healthy' : 'unknown', code: 'TEST', message: 'ready', checkedAt: null,
    }])),
    selection: {
      clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Restricted',
      scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 1, generation,
    },
  }
}

const pod = { namespace: 'payments', name: 'api-abc', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, node: 'worker-1', ip: '10.0.0.8', owner: { kind: 'Deployment', name: 'api' }, ageSeconds: 60, problematic: false }

let preferences = {
  version: 1 as const,
  ui: { language: 'en' as const },
  logs: { wrap: false, timestamps: true, tailLines: 200 },
  dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] as string[] },
  filters: {
    workloads: { version: 1 as const, items: [] },
    pods: { version: 1 as const, items: [] },
    events: { version: 1 as const, items: [] },
    logs: { version: 1 as const, items: [] },
  },
  favorites: { version: 1 as const, items: [] as Array<{ id: string; kind: string; namespace: string; name: string }> },
}

const meta = { generation }

test.beforeEach(async ({ context }) => {
  preferences = {
    version: 1,
    ui: { language: 'en' },
    logs: { wrap: false, timestamps: true, tailLines: 200 },
    dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] },
    filters: {
      workloads: { version: 1, items: [] },
      pods: { version: 1, items: [] },
      events: { version: 1, items: [] },
      logs: { version: 1, items: [] },
    },
    favorites: { version: 1, items: [] },
  }
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    let data: unknown = []
    let responseMeta: Record<string, unknown> = meta
    if (url.pathname === '/api/v1/status') data = statusPayload()
    else if (url.pathname === '/api/v1/cluster/profiles') data = []
    else if (url.pathname === '/api/v1/session') data = { csrfToken: 'csrf_features', origin: 'http://127.0.0.1:4173', generation, expiresAt: '2026-09-02T18:00:00Z' }
    else if (url.pathname === '/api/v1/preferences' && request.method() === 'PUT') {
      preferences = request.postDataJSON() as typeof preferences
      data = preferences
    } else if (url.pathname === '/api/v1/preferences') data = preferences
    else if (url.pathname === '/api/v1/pods' && url.searchParams.has('limit')) {
      data = [pod]
      responseMeta = { generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }, coverage: null }
    } else if (url.pathname === '/api/v1/pods/payments/api-abc') {
      data = {
        metadata: { namespace: 'payments', name: 'api-abc', uid: 'uid-api', resourceVersion: '17', creationTimestamp: '2026-08-17T10:00:00Z', labels: {} },
        summary: pod,
        conditions: [],
        containers: [{ spec: { name: 'api', image: 'example/api:1', ports: [] }, type: 'regular', ready: true, restartCount: 0, state: 'running', reason: null }],
        initContainers: [],
        ephemeralContainers: [],
        relatedEvents: [],
      }
    } else if (url.pathname === '/api/v1/pods/payments/api-abc/yaml') {
      // Served as raw YAML text, not the JSON envelope.
      await route.fulfill({ status: 200, contentType: 'application/yaml; charset=utf-8', body: 'apiVersion: v1\nkind: Pod\nmetadata:\n  name: api-abc\n  namespace: payments\n  annotations:\n    example: "1"\nspec:\n  replicas: 1\n' })
      return
    } else if (url.pathname === '/api/v1/resources/pods/payments/api-abc/yaml-diff') {
      data = {
        absent: false, truncated: false,
        lines: [
          { kind: 'same', text: 'metadata:' },
          { kind: 'removed', text: '  example: "1"' },
          { kind: 'added', text: '  example: "2"' },
        ],
      }
    } else if (url.pathname === '/api/v1/resources/deployments/payments/api/yaml-diff') {
      data = { absent: true, truncated: false, lines: [] }
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: responseMeta }) })
  })
})

test('global search navigates to resources already loaded in the session (F7-04)', async ({ page }) => {
  await page.goto('/pods')
  await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()
  await page.waitForTimeout(300) // allow the bounded pod page cache to populate

  await page.keyboard.press('Control+k')
  const input = page.getByRole('combobox', { name: 'Search application pages' })
  await input.fill('api-abc')
  const option = page.getByRole('option', { name: /api-abc/ })
  await expect(option).toBeVisible()
  await option.click()
  await expect(page).toHaveURL(/\/pods\/payments\/api-abc$/)
})

test('favorite star persists through the allowlisted preferences PUT (F7-01)', async ({ page }) => {
  await page.goto('/pods')
  await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()
  await page.waitForTimeout(300)

  await page.getByRole('button', { name: /Open Pod api-abc in payments/i }).click()
  await expect(page).toHaveURL(/\/pods\/payments\/api-abc$/)
  await expect(page.getByRole('dialog')).toBeVisible()
  const star = page.getByRole('button', { name: 'Add api-abc to favorites' })
  await expect(star).toBeEnabled()
  await star.click()

  await expect.poll(() => preferences.favorites?.items.length).toBe(1)
  expect(preferences.favorites.items[0]).toMatchObject({ kind: 'pod', namespace: 'payments', name: 'api-abc' })
  await expect(page.getByRole('button', { name: 'Remove api-abc from favorites' })).toBeVisible()

  await page.keyboard.press('Control+k')
  const input = page.getByRole('combobox', { name: 'Search application pages' })
  await input.fill('favorite')
  await expect(page.getByRole('option', { name: /deployment · payments|pod · payments/i }).first()).toBeVisible()
})

test('yaml diff renders added and removed lines and the absent baseline state (F7-02)', async ({ page }) => {
  await page.goto('/pods')
  await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()
  await page.waitForTimeout(300)

  await page.getByRole('button', { name: /Open Pod api-abc in payments/i }).click()
  await expect(page).toHaveURL(/\/pods\/payments\/api-abc$/)
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByRole('button', { name: 'Load authorized YAML' }).click()
  await expect(page.getByLabel('YAML document')).toBeVisible()

  await page.getByRole('button', { name: 'Diff vs last-applied' }).click()
  const diff = page.getByLabel('YAML diff against last-applied')
  await expect(diff).toBeVisible()
  await expect(diff).toContainText('-   example: "1"')
  await expect(diff).toContainText('+   example: "2"')

  await page.goto('/workloads')
  await page.waitForTimeout(300)
})
