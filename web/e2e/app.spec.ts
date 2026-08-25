import { expect, test } from '@playwright/test'

test('serves the application shell and preserves History API navigation', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByLabel('Primary navigation')).toBeVisible()
  await expect(page.getByText('kubePeep', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'The local API returned an error' })).toBeVisible()

  await page.getByRole('link', { name: 'Workloads' }).click()
  await expect(page).toHaveURL(/\/workloads$/)
  await expect(page.getByRole('heading', { name: 'Workloads' })).toBeVisible()

  await page.reload()
  await expect(page.getByRole('heading', { name: 'Workloads' })).toBeVisible()
})

test('keeps the dashboard useful with partial data and an explicit bounded log scan', async ({ page }) => {
  const block = (value: unknown, overrides: Record<string, unknown> = {}) => ({
    value, complete: true, truncated: false,
    coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
    errors: [], ...overrides,
  })
  const counters = {
    namespaces: { state: 'available', value: 1 }, podsTotal: { state: 'available', value: 3 },
    podsHealthy: { state: 'available', value: 2 }, podsProblematic: { state: 'available', value: 1 },
    workloadsDegraded: { state: 'available', value: 1 }, restarts: { state: 'available', value: 12 },
    warningEvents: { state: 'available', value: 2 }, possibleLogMatches: { state: 'notCollected', value: null },
  }
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, {
      status: name === 'application' || name === 'cluster' ? 'healthy' : 'unknown', code: 'TEST', message: 'ready', checkedAt: null,
    }])),
    selection: {
      clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Restricted',
      scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'allowed', namespaceCount: 1, generation: 'gen_e2e',
    },
  }

  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const path = `${url.pathname}${url.search}`
    let data: unknown
    if (path === '/api/v1/status') data = status
    else if (path === '/api/v1/cluster/profiles') data = []
    else if (path === '/api/v1/session') data = { csrfToken: 'csrf_e2e', origin: 'http://127.0.0.1:4173', generation: 'gen_e2e', expiresAt: '2026-08-10T13:00:00Z' }
    else if (path === '/api/v1/dashboard/summary') data = block(counters, { coverage: null })
    else if (path === '/api/v1/dashboard/problems') data = block([{
      namespace: 'allowed', pod: 'restart-pod', owner: { kind: 'Deployment', name: 'api' }, container: 'api', containerType: 'regular',
      status: 'Running', reason: 'CrashLoopBackOff', message: 'back-off restarting failed container', source: 'containerWaiting', severity: 'critical', ageSeconds: 180,
    }])
    else if (path === '/api/v1/dashboard/restarts?limit=10') data = block([{
      namespace: 'allowed', pod: 'restart-pod', owner: { kind: 'Deployment', name: 'api' }, container: 'api', containerType: 'regular',
      restarts: 12, severity: 'critical', status: 'CrashLoopBackOff', lastReason: 'Error', ageSeconds: 180,
    }])
    else if (path === '/api/v1/dashboard/events') data = block([{
      timestamp: '2026-08-10T12:00:00Z', namespace: 'allowed', objectKind: 'Pod', objectName: 'restart-pod',
      reason: 'BackOff', message: 'Back-off restarting failed container', count: 2, source: 'kubelet', type: 'Warning',
    }])
    else if (path === '/api/v1/metrics') data = block({ collectedAt: '', windowSeconds: 0, pods: [], topCPU: [], topMemory: [] }, {
      complete: false, errors: [{ code: 'FEATURE_UNAVAILABLE', message: 'The optional feature is unavailable.' }],
    })
    else if (path === '/api/v1/dashboard/log-scan') data = block([{
      namespace: 'allowed', pod: 'restart-pod', container: 'api', workload: { kind: 'Deployment', name: 'api' },
      timestamp: '2026-08-10T12:00:00Z', excerpt: 'token=[REDACTED]', reasonCode: 'ERROR_KEYWORD', redacted: true, truncated: false,
    }])
    else data = []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: { generation: 'gen_e2e', collectedAt: '2026-08-10T12:00:00Z' } }) })
  })

  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Cluster overview' })).toBeVisible()
  await expect(page.getByText('CrashLoopBackOff', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Metrics API is not available. The rest of the dashboard is unaffected.')).toBeVisible()
  await expect(page.getByText('Scan has not been run')).toBeVisible()

  await page.getByRole('button', { name: 'Scan logs now' }).click()
  await expect(page.getByText('token=[REDACTED]')).toBeVisible()
  await expect(page.getByText('sensitive value redacted')).toBeVisible()
})

test('filters Pods, persists allowlisted saved filters and builds the Logs catalog from exact capabilities', async ({ page }) => {
  const generation = 'gen_filters_e2e'
  const empty = { version: 1, items: [] as Array<{ id: string; name: string; query: Record<string, unknown> }> }
  let preferences = {
    version: 1,
    ui: { language: 'en' },
    logs: { wrap: false, timestamps: true, tailLines: 200 },
    dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] as string[] },
    filters: {
      workloads: empty,
      pods: { version: 1, items: [{ id: 'saved-worker', name: 'Worker failures', query: { namespace: ['payments'], status: ['Failed'], node: 'worker-9' } }] },
      events: empty,
      logs: empty,
    },
  }
  const podRequests: string[] = []
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Restricted', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 1, generation },
  }
  const pod = { namespace: 'payments', name: 'api-abc', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 2, node: 'worker-1', ip: '10.0.0.8', owner: { kind: 'Deployment', name: 'api' }, ageSeconds: 60, problematic: false }

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = `${url.pathname}${url.search}`
    let data: unknown = []
    let meta: Record<string, unknown> = { generation }
    if (url.pathname === '/api/v1/status') data = status
    else if (url.pathname === '/api/v1/cluster/profiles') data = []
    else if (url.pathname === '/api/v1/session') data = { csrfToken: 'csrf_filters', origin: 'http://127.0.0.1:4173', generation, expiresAt: '2026-08-17T18:00:00Z' }
    else if (url.pathname === '/api/v1/preferences' && request.method() === 'PUT') {
      preferences = request.postDataJSON() as typeof preferences
      data = preferences
    } else if (url.pathname === '/api/v1/preferences') data = preferences
    else if (url.pathname === '/api/v1/pods' && url.searchParams.has('limit')) {
      podRequests.push(path)
      data = [pod]
      meta = { generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }, coverage: null }
    } else if (url.pathname === '/api/v1/permissions') data = {
      generation, complete: true, truncated: false, errors: [], decisions: [{ capabilityId: 'pods.logs.get', namespace: 'payments', resourceName: 'api-abc', decision: 'allowed', apiGroup: '', resource: 'pods', subresource: 'log', verb: 'get', reasonCode: 'SAR_ALLOWED', expiresAt: null }],
    }
    else if (url.pathname === '/api/v1/pods/payments/api-abc') data = {
      metadata: { namespace: 'payments', name: 'api-abc', uid: 'uid-api', resourceVersion: '17', creationTimestamp: '2026-08-17T10:00:00Z', labels: {} },
      summary: pod, conditions: [], containers: [{ spec: { name: 'api', image: 'example/api:1', ports: [] }, type: 'regular', ready: true, restartCount: 2, state: 'running', reason: null }], initContainers: [], ephemeralContainers: [], relatedEvents: [],
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta }) })
  })

  await page.goto('/pods')
  await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()
  await page.getByLabel('Namespace').fill('payments')
  await page.getByLabel('Workload owner').fill('api')
  await page.getByLabel('Node').fill('worker-1')
  await page.getByLabel('Search this bounded page').fill('backend')
  await page.getByRole('button', { name: 'Apply filters' }).click()
  await expect.poll(() => podRequests.some((value) => {
    const query = new URL(value, 'http://127.0.0.1').searchParams
    return query.get('namespace') === 'payments' && query.get('workload') === 'api' && query.get('node') === 'worker-1' && query.get('search') === 'backend'
  })).toBe(true)

  await page.getByRole('combobox', { name: 'Saved filter', exact: true }).selectOption('saved-worker')
  await page.getByRole('button', { name: 'Apply saved filter' }).click()
  await expect(page.getByLabel('Node')).toHaveValue('worker-9')
  await expect(page.getByLabel('Status')).toHaveValue('Failed')
  await page.getByLabel('Save current filter as').fill('Saved from browser')
  await page.getByRole('button', { name: 'Save current filter' }).click()
  await expect(page.getByText('Current bounded filter saved.')).toBeVisible()
  expect(preferences.filters.pods.items.some((value) => value.name === 'Saved from browser')).toBe(true)

  await page.getByRole('link', { name: 'Logs', exact: true }).click()
  await expect(page.getByText('1 log-authorized Pod available in the complete bounded catalog.')).toBeVisible()
  await page.getByRole('combobox', { name: 'Namespace', exact: true }).selectOption('payments')
  await page.getByRole('combobox', { name: 'Pod', exact: true }).selectOption('api-abc')
  await page.getByRole('combobox', { name: 'Container', exact: true }).selectOption('api')
  await expect(page.getByRole('button', { name: 'Read logs' })).toBeEnabled()
})
