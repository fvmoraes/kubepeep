import { expect, test, type Page } from '@playwright/test'

const generation = 'gen_detail_tabs'
const metadata = { namespace: 'payments', name: 'api', uid: 'uid-api', resourceVersion: '17', creationTimestamp: '2026-10-01T00:00:00Z', labels: {} }
const budget = { cpuRequestMillicores: 500, cpuLimitMillicores: 1000, memoryRequestBytes: 134217728, memoryLimitBytes: 268435456 }
const container = { name: 'api', image: 'example/api:1', ports: [], resources: budget, environment: [], envFrom: [] }
const pod = { ...metadata, name: 'api-pod', ready: { current: 1, desired: 1 }, restarts: 0, status: 'Running', node: 'worker-1', ageSeconds: 60, resources: budget, owner: { kind: 'Deployment', name: 'api' }, secrets: [], configMaps: [] }
const related = [{ kind: 'Pod', namespace: 'payments', name: 'api-pod' }, { kind: 'ReplicaSet', namespace: 'payments', name: 'api-rs' }, { kind: 'Job', namespace: 'payments', name: 'backup-job' }]
const conditions = [{ type: 'Available', status: 'True', reason: 'MinimumReplicasAvailable', message: 'Ready', lastTransitionTime: '2026-10-01T00:00:00Z' }]
const service = { ...metadata, type: 'ClusterIP', clusterIPs: ['10.0.0.25'], ports: [{ name: 'http', protocol: 'TCP', port: 8080, targetPort: { value: 8080, isInt: true } }], selector: { app: 'api' }, ageSeconds: 60 }
const ingress = { ...metadata, className: 'nginx', hosts: ['api.example.test'], tlsHosts: ['api.example.test'], paths: [{ host: 'api.example.test', path: '/v1', pathType: 'Prefix', backend: { serviceName: 'api', servicePort: { value: 8080 } } }], ageSeconds: 60 }
const errors = new WeakMap<Page, string[]>()
const mutations = new WeakMap<Page, string[]>()

test.beforeEach(async ({ page }) => {
  const failures: string[] = [], writes: string[] = []
  errors.set(page, failures)
  mutations.set(page, writes)
  page.on('pageerror', error => failures.push(error.message))
  page.on('console', message => {
    // HTTP failures are asserted as visible errors in the explicit failure test.
    if (message.type() === 'error' && !message.text().startsWith('Failed to load resource:')) failures.push(message.text())
  })
  await page.route('**/api/v1/**', async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname
    if (!['GET', 'HEAD'].includes(request.method())) writes.push(path)
    let data: unknown = []
    const pageMeta = { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }
    if (path === '/api/v1/status') data = { version: 'test', components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map(key => [key, { status: 'healthy' }])), selection: { clusterProfileId: 1, context: 'development', cluster: 'dev', scopeId: 1, scopeMode: 'list', scopeName: 'Payments', scopeSource: 'saved', defaultNamespace: 'payments', namespaceCount: 1, generation } }
    else if (path === '/api/v1/preferences') data = { version: 1, ui: { language: 'en' }, logs: { wrap: false, timestamps: true, tailLines: 200 }, dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] }, filters: Object.fromEntries(['pods', 'workloads', 'events', 'logs'].map(name => [name, { version: 1, items: [] }])) }
    else if (path === '/api/v1/session') data = { csrfToken: 'csrf_tabs', generation }
    else if (path === '/api/v1/namespace-scopes/1') data = { namespaces: ['payments'], defaultNamespace: 'payments' }
    else if (path === '/api/v1/stream') { await route.fulfill({ contentType: 'text/event-stream', body: `event: meta\ndata: {"generation":"${generation}"}\n\n` }); return }
    else if (path === '/api/v1/pods') data = [pod]
    else if (path === '/api/v1/pods/payments/api-pod') data = { metadata: { ...metadata, name: pod.name }, summary: pod, conditions, containers: [{ spec: container, type: 'regular', ready: true, restartCount: 0, state: 'running' }], initContainers: [], ephemeralContainers: [], relatedEvents: [] }
    else if (path === '/api/v1/workloads' || path.startsWith('/api/v1/workloads/')) {
      const kind = ({ deployments: 'Deployment', replicasets: 'ReplicaSet', statefulsets: 'StatefulSet', cronjobs: 'CronJob' } as Record<string, string>)[path.split('/')[4] ?? url.searchParams.get('kind') ?? 'deployments'] ?? 'Deployment'
      const value = { ...metadata, metadata, kind, ready: 1, desired: 1, available: 1, updated: 1, status: 'Healthy', conditions, containers: [container], resources: budget, related, secrets: [], configMaps: [], ageSeconds: 60 }
      data = path === '/api/v1/workloads' ? [value] : value
    }
    else if (path.startsWith('/api/v1/investigation/')) data = { generation, ownerChain: [{ kind: 'Deployment', namespace: 'payments', name: 'api', status: 'Healthy' }], pods: [related[0]], services: [{ kind: 'Service', namespace: 'payments', name: 'api' }], endpointSlices: [], configMaps: [], pvcs: [], events: [], coverage: [{ topic: 'Services', complete: false }] }
    else if (path === '/api/v1/metrics') data = { complete: true, truncated: false, errors: [], coverage: null, value: { collectedAt: '2026-10-12T12:00:00Z', windowSeconds: 30, pods: [{ namespace: 'payments', pod: 'api-pod', cpuMillicores: 250, memoryBytes: 67108864, containers: [{ name: 'api', cpuMillicores: 250, memoryBytes: 67108864 }] }], topCPU: [], topMemory: [] } }
    else if (path === '/api/v1/events') data = [
      { namespace: 'payments', name: 'pod-event', objectKind: 'Pod', objectName: 'api-pod', type: 'Normal', reason: 'Started', count: 1, message: 'Pod audit event', timestamp: '2026-10-12T12:00:00Z' },
      { namespace: 'payments', name: 'deployment-event', objectKind: 'Deployment', objectName: 'api', type: 'Normal', reason: 'ScalingReplicaSet', count: 1, message: 'Deployment audit event', timestamp: '2026-10-12T12:00:00Z' },
      { namespace: 'payments', name: 'unrelated-event', objectKind: 'Pod', objectName: 'unrelated', type: 'Warning', reason: 'Failed', count: 1, message: 'Unrelated event must stay hidden', timestamp: '2026-10-12T12:00:00Z' },
    ]
    else if (path === '/api/v1/services') data = [service]
    else if (path === '/api/v1/services/payments/api') data = { metadata, summary: service }
    else if (path === '/api/v1/endpoints/payments/api') data = { ...metadata, readyCount: 2, notReadyCount: 1, ports: ['http:8080/TCP'], subsets: [], truncated: false }
    else if (path === '/api/v1/ingresses') data = [ingress]
    else if (path === '/api/v1/ingresses/payments/api') data = { metadata, summary: ingress, defaultBackend: null }
    else if (path === '/api/v1/persistent-volume-claims') data = [{ ...metadata, name: 'data-api-0', status: 'Bound', volumeName: 'disk-0', ageSeconds: 60 }, { ...metadata, name: 'data-unrelated-0', status: 'Bound', volumeName: 'disk-1', ageSeconds: 60 }]
    else if (path === '/api/v1/permissions') data = { generation, complete: true, truncated: false, errors: [], decisions: url.searchParams.getAll('capability').map(capabilityId => ({ capabilityId, namespace: 'payments', resourceName: 'api', decision: 'denied' })) }
    await route.fulfill({ json: { data, meta: { generation, page: pageMeta, coverage: null } } })
  })
})

test.afterEach(async ({ page }) => {
  expect(errors.get(page), 'No uncaught exceptions or React rendering errors').toEqual([])
  expect(mutations.get(page), 'Read-only detail navigation must not mutate resources').toEqual([])
})

for (const kind of ['Pod', 'Deployment']) {
  test(`${kind} Metrics, Investigation and Events render content without losing object identity`, async ({ page }) => {
    await page.goto(kind === 'Pod' ? '/pods/payments/api-pod' : '/workloads/deployments/payments/api')
    const panel = page.locator('.workspace-panel')
    await expect(panel.getByRole('tabpanel')).toContainText('uid-api')
    await panel.getByRole('tab', { name: 'Metrics', exact: true }).click()
    const metrics = panel.getByRole('region', { name: kind === 'Pod' ? 'Pod resource usage' : 'Workload resource usage' })
    await expect(metrics.getByRole('meter', { name: 'CPU', exact: true }).first()).toHaveAttribute('aria-valuetext', '250 m / 1000 m · 25.0%')
    await expect(metrics.getByRole('meter', { name: 'Memory', exact: true }).first()).toHaveAttribute('aria-valuetext', '64 MiB / 256 MiB · 25.0%')
    await panel.getByRole('tab', { name: 'Investigation', exact: true }).click()
    await expect(panel.getByRole('status')).toContainText('Partial local coverage: Services')
    await expect(panel.getByRole('button', { name: 'Service · api payments' })).toBeVisible()
    await panel.getByRole('tab', { name: 'Events', exact: true }).click()
    await expect(panel.getByRole('tabpanel')).toContainText(`${kind} audit event`)
    await expect(panel.getByRole('tabpanel')).not.toContainText('Unrelated event must stay hidden')
    if (kind === 'Pod') {
      await panel.getByRole('tab', { name: 'Containers', exact: true }).click()
      await expect(panel.getByRole('row').filter({ hasText: 'example/api:1' })).toContainText('running')
    } else {
      for (const [tab, target] of [['Pods', 'Open Pod api-pod'], ['ReplicaSets', 'Open ReplicaSet api-rs']]) {
        await panel.getByRole('tab', { name: tab, exact: true }).click()
        await expect(panel.getByRole('button', { name: target, exact: true })).toBeVisible()
      }
      await panel.getByRole('tab', { name: 'Rollout', exact: true }).click()
      await expect(panel.getByRole('tabpanel')).toContainText('never restarted by KubePeep')
      await expect(panel.getByRole('row').filter({ hasText: 'Available' })).toContainText('True')
    }
    await panel.getByRole('tab', { name: 'Overview', exact: true }).click()
    await expect(panel.getByRole('tabpanel')).toContainText('uid-api')
    await expect(panel.getByRole('alert')).toHaveCount(0)
  })
}

test('StatefulSet PVCs preserve scoped related names and CronJob Jobs use explicit references', async ({ page }) => {
  await page.goto('/workloads/statefulsets/payments/api')
  const panel = page.locator('.workspace-panel')
  await panel.getByRole('tab', { name: 'PVCs', exact: true }).click()
  await expect(panel.getByRole('button', { name: 'Open PersistentVolumeClaim data-api-0', exact: true })).toBeVisible()
  await expect(panel.getByRole('tabpanel')).not.toContainText('data-unrelated-0')
  await page.goto('/workloads/cronjobs/payments/api')
  await panel.getByRole('tab', { name: 'Jobs', exact: true }).click()
  await expect(panel.getByRole('button', { name: 'Open Job backup-job', exact: true })).toBeVisible()
  await expect(panel.getByRole('tabpanel')).not.toContainText('api-pod')
})

test('Service Endpoints load bounded counts and denied actions remain disabled', async ({ page }) => {
  await page.goto('/network/services/payments/api')
  const panel = page.locator('.workspace-panel')
  await panel.getByRole('tab', { name: 'Endpoints', exact: true }).click()
  await expect(panel.getByRole('tabpanel')).toContainText('http:8080/TCP')
  await expect(panel.getByText('Ready addresses', { exact: true }).locator('..')).toContainText('2')
  await panel.getByRole('tab', { name: 'Actions', exact: true }).click()
  await expect(panel.getByRole('button', { name: 'Start port-forward' })).toBeDisabled()
  await expect(panel.getByRole('alert')).toHaveCount(0)
})

test('Ingress Rules, Backends and Actions retain routes and navigate to the exact Service', async ({ page }) => {
  await page.goto('/network/ingresses/payments/api')
  const panel = page.locator('.workspace-panel')
  await panel.getByRole('tab', { name: 'Rules', exact: true }).click()
  const rule = panel.getByRole('row').filter({ hasText: 'api.example.test' })
  await expect(rule).toContainText('/v1')
  await expect(rule).toContainText('api:8080')
  await panel.getByRole('tab', { name: 'Actions', exact: true }).click()
  await expect(panel.getByRole('link', { name: 'Open URL' })).toHaveAttribute('href', 'https://api.example.test')
  await expect(panel.getByRole('button', { name: 'Copy Host' })).toBeEnabled()
  await panel.getByRole('tab', { name: 'Backends', exact: true }).click()
  await panel.getByRole('button', { name: 'Open Service api', exact: true }).click()
  await expect(page).toHaveURL(url => url.pathname === '/network/services/payments/api' && url.searchParams.get('focus') === 'api' && url.searchParams.get('namespace') === 'payments')
  await expect(panel.getByRole('tabpanel')).toContainText('10.0.0.25')
})

test('a denied detail tab shows its error while other tabs remain usable', async ({ page }) => {
  await page.route('**/api/v1/endpoints/payments/api', route => route.fulfill({ status: 403, json: { code: 'FORBIDDEN', message: 'Endpoints are not authorized for this identity.' } }))
  await page.goto('/network/services/payments/api')
  const panel = page.locator('.workspace-panel')
  await panel.getByRole('tab', { name: 'Endpoints', exact: true }).click()
  await expect(panel.getByRole('alert')).toContainText('Endpoints are not authorized for this identity.')
  await panel.getByRole('tab', { name: 'Overview', exact: true }).click()
  await expect(panel.getByRole('tabpanel')).toContainText('10.0.0.25')
  await expect(panel.getByRole('alert')).toHaveCount(0)
})
