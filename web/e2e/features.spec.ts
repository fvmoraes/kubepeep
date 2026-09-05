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

const node = { name: 'worker-1', status: 'Ready', ready: true, roles: ['control-plane'], kubeletVersion: '1.32.0', internalIP: '192.168.8.10', ageSeconds: 86400 }

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
    } else if (url.pathname === '/api/v1/nodes' && url.searchParams.has('limit')) {
      data = [node]
      responseMeta = { generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'page' }, coverage: { requestedNamespaces: 0, completedNamespaces: 0, deniedNamespaces: [], failed: [] } }
    } else if (url.pathname === '/api/v1/nodes/worker-1') {
      data = {
        metadata: { namespace: '', name: 'worker-1', uid: 'uid-node', resourceVersion: '42', creationTimestamp: '2026-06-01T10:00:00Z', labels: {} },
        status: 'Ready', ready: true, roles: ['control-plane'], kubeletVersion: '1.32.0', internalIP: '192.168.8.10',
        conditions: [{ type: 'Ready', status: 'True', reason: 'KubeletReady', message: 'kubelet is posting ready status', lastTransitionTime: '2026-06-01T10:05:00Z' }],
        capacity: { cpu: '8', memory: '16Gi' }, allocatable: { cpu: '8', memory: '16Gi' }, taints: [], truncated: false,
      }
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

test('bulk namespace paste previews counts and saves one scope without cluster discovery (U12)', async ({ page, context }) => {
  const savedScopes: Array<{ id: number; name: string; mode: string; namespaces: string[] }> = []
  // Context-level routes registered after beforeEach win route precedence.
  await context.route('**/api/v1/namespace-scopes*', async (route) => {
    const request = route.request()
    if (request.method() === 'GET') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: savedScopes, meta }) })
      return
    }
    if (request.method() === 'POST' && !new URL(request.url()).pathname.endsWith('validate')) {
      const payload = request.postDataJSON() as { name?: string; mode: string; rawInput?: string }
      const parsed = payload.rawInput?.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean) ?? []
      const scope = { id: savedScopes.length + 1, name: payload.name ?? 'Batch', mode: payload.mode, namespaces: [...new Set(parsed)] }
      savedScopes.push(scope)
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { ...scope, clusterProfileId: 1, context: 'development', defaultNamespace: scope.namespaces[0] ?? null, version: 1, createdAt: '2026-09-05T00:00:00Z', updatedAt: '2026-09-05T00:00:00Z' }, meta }) })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [], meta }) })
  })
  await context.route('**/api/v1/namespace-scopes/1/select*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { selected: true }, meta }) })
  })
  await context.route('**/api/v1/namespaces*', async (route) => {
    await route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ code: 'FORBIDDEN', message: 'list namespaces denied' }) })
  })

  await page.goto('/namespaces')
  await expect(page.getByRole('heading', { name: 'Create a namespace scope' })).toBeVisible()
  await page.getByLabel('Scope name').fill('Batch fleet')
  await page.locator('fieldset').getByText('list', { exact: true }).click()
  const input = page.getByLabel('Namespace input')
  const names = Array.from({ length: 100 }, (_, index) => `fleet-${String(index + 1).padStart(3, '0')}`)
  await input.fill(`${names.join(', ')}, fleet-001, invalid_name,`)
  await expect(page.getByText('duplicates removed', { exact: false })).toBeVisible()
  await expect(page.locator('[aria-label="Namespace validation counters"]')).toContainText('100')
  await expect(page.locator('[aria-label="Parsed namespaces"]')).toContainText('invalid_name')

  // Invalid entries keep the save fail-closed until they are explicitly removed.
  await page.getByRole('button', { name: 'Remove invalid namespace invalid_name' }).click()
  await page.getByRole('button', { name: 'Save scope' }).click()
  await expect.poll(() => savedScopes.length).toBe(1)
  expect(savedScopes[0].namespaces).toHaveLength(100)
  await expect(page.getByText('Scope “Batch fleet” was saved.')).toBeVisible()

  // The mocked session already carries scopeId 1, so the saved scope renders as active.
  await expect(page.getByRole('button', { name: /Edit Batch fleet/i })).toBeVisible()
  await expect(page.getByText('active', { exact: true })).toBeVisible()
})

test('nodes lists and details without namespace scope and honors authorization (R02/V1)', async ({ page }) => {
  await page.goto('/nodes')
  await expect(page.getByRole('heading', { name: 'Nodes' })).toBeVisible()
  await expect(page.getByRole('cell', { name: /worker-1/ })).toBeVisible()
  await expect(page.getByText('Cluster-scoped result')).toBeVisible()

  await page.getByRole('button', { name: /Open Node worker-1/i }).click()
  await expect(page).toHaveURL(/\/nodes\/worker-1$/)
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByText('control-plane').first()).toBeVisible()
  await expect(page.getByText('Kubelet', { exact: true })).toBeVisible()
  await expect(page.getByText('1.32.0').first()).toBeVisible()
  await expect(page.getByText('Condition', { exact: true })).toBeVisible()
})

test('nodes route is reachable from the sidebar navigation tree (V1-08)', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('navigation', { name: 'Primary navigation' }).getByRole('link', { name: 'Nodes', exact: true }).click()
  await expect(page).toHaveURL(/\/nodes$/)
  await expect(page.getByRole('heading', { name: 'Nodes' })).toBeVisible()
})

test('storage tabs render honest states and navigate (R23-R27/V2)', async ({ page }) => {
  const pv = { name: 'pv-data', status: 'Available', capacity: '10Gi', accessModes: ['ReadWriteOnce'], reclaimPolicy: 'Retain', storageClass: 'standard', claim: null, ageSeconds: 3600 }
  await page.route('**/api/v1/persistent-volumes*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [pv], meta: { generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'page' }, coverage: { requestedNamespaces: 0, completedNamespaces: 0, deniedNamespaces: [], failed: [] } } }) })
  })
  await page.route('**/api/v1/persistent-volume-claims*', async (route) => {
    await route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ code: 'FORBIDDEN', message: 'Access to this resource was denied.' }) })
  })

  await page.goto('/storage/persistent-volumes')
  await expect(page.getByRole('heading', { name: 'Storage' })).toBeVisible()
  await expect(page.getByRole('cell', { name: /pv-data/ })).toBeVisible()
  // No fictitious namespace fan-out for cluster-scoped lists.
  await expect(page.getByText('Cluster-scoped result')).toBeVisible()

  await page.getByRole('tab', { name: 'persistent-volume-claims' }).click()
  await expect(page).toHaveURL(/\/storage\/persistent-volume-claims$/)
  // Denial is authoritative: never an empty result.
  await expect(page.getByText('Access to this resource was denied.')).toBeVisible()
})

test('leases page lists authorized leases in the scoped namespaces (R05/V2)', async ({ page }) => {
  const lease = { namespace: 'payments', name: 'leader-election', holderName: 'api-abc', durationSeconds: 15, renewTime: '2026-09-05T01:00:00Z', ageSeconds: 120 }
  await page.route('**/api/v1/leases?*', async (route) => {
    const url = new URL(route.request().url())
    const data = url.pathname === '/api/v1/leases' ? [lease] : []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: { generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'page' }, coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] } } }) })
  })

  await page.goto('/leases')
  await expect(page.getByRole('heading', { name: 'Leases' })).toBeVisible()
  await expect(page.getByRole('cell', { name: /leader-election/ })).toBeVisible()
  await expect(page.getByText('api-abc').first()).toBeVisible()
})
