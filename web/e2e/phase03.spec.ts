import { expect, test } from '@playwright/test'

test('12k synthetic Pods keep the table DOM bounded while scrolling', async ({ page }) => {
  const generation = 'gen_phase03'
  const pods = Array.from({ length: 12_000 }, (_, index) => ({
    namespace: 'default', name: `pod-${String(index).padStart(5, '0')}`, status: 'Running',
    ready: { current: 1, desired: 1 }, restarts: 0, node: 'worker-1', ip: null,
    owner: null, ageSeconds: 60, problematic: false,
  }))
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: name === 'metrics' ? 'unknown' : 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Default', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'default', namespaceCount: 1, generation },
  }
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const data = url.pathname === '/api/v1/status' ? status : url.pathname === '/api/v1/pods' ? pods : []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: {
      generation, collectedAt: '2026-09-21T17:00:00Z',
      page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' },
      coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
    } }) })
  })

  await page.goto('/pods')
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table).toHaveAttribute('aria-rowcount', '12001')
  await expect(page.getByText('pod-00000')).toBeVisible()
  expect(await table.getByRole('row').count()).toBeLessThan(60)
  await expect.poll(async () => (await page.evaluate(() => window.__KUBEPEEP_UX_METRICS__?.snapshot() ?? [])).some((sample) => sample.name === 'time_to_first_row' && sample.view === 'pods')).toBe(true)
  const timings = await page.evaluate(() => window.__KUBEPEEP_UX_METRICS__?.snapshot().filter((sample) => sample.name === 'time_to_shell_ready' || sample.view === 'pods' && (sample.name === 'time_to_first_row' || sample.name === 'time_to_page_complete')) ?? [])
  console.log(`Phase 03 synthetic 12k timing: ${JSON.stringify(timings)}`)
  // This deliberately violates the API's 100-item page limit to stress the
  // virtualizer. Timing budgets are measured on real bounded pages.
  expect(timings.find((sample) => sample.name === 'time_to_shell_ready')?.value).toBeGreaterThanOrEqual(0)
  expect(timings.find((sample) => sample.name === 'time_to_first_row')?.value).toBeGreaterThanOrEqual(0)
  expect(timings.find((sample) => sample.name === 'time_to_page_complete')?.value).toBeGreaterThanOrEqual(0)

  const viewport = table.locator('..')
  await viewport.evaluate((element) => { element.scrollTop = element.scrollHeight * 0.75; element.dispatchEvent(new Event('scroll')) })
  await expect.poll(async () => table.locator('tbody tr[data-index]').first().getAttribute('data-index')).not.toBe('0')
  expect(await table.getByRole('row').count()).toBeLessThan(60)
  const frameRate = await viewport.evaluate(async (element) => {
    element.scrollTop = 0
    const timestamps: number[] = []
    await new Promise<void>((resolve) => {
      const started = performance.now()
      const frame = (timestamp: number) => {
        timestamps.push(timestamp)
        element.scrollTop = (element.scrollHeight - element.clientHeight) * Math.min(1, (timestamp - started) / 1_500)
        if (timestamp - started < 1_500) requestAnimationFrame(frame)
        else resolve()
      }
      requestAnimationFrame(frame)
    })
    const intervals = timestamps.slice(1).map((timestamp, index) => timestamp - timestamps[index]).sort((left, right) => left - right)
    const renderedRows = element.querySelectorAll('tbody tr[data-index]')
    return { fps: (timestamps.length - 1) * 1_000 / (timestamps.at(-1)! - timestamps[0]), p95FrameMs: intervals[Math.floor(intervals.length * 0.95)] ?? 0, renderedRows: renderedRows.length, lastIndex: Number(renderedRows.item(renderedRows.length - 1)?.getAttribute('data-index')) }
  })
  console.log(`Phase 03 synthetic 12k continuous scroll: ${JSON.stringify(frameRate)}`)
  expect(frameRate.renderedRows).toBeLessThan(60)
  await expect.poll(async () => {
    await viewport.evaluate((element) => { element.scrollTop = element.scrollHeight })
    return Number(await table.locator('tbody tr[data-index]').last().getAttribute('data-index'))
  }).toBeGreaterThanOrEqual(11_990)
})

test('prefetches only the next authorized page after 75% scroll', async ({ page }) => {
  const generation = 'gen_prefetch'
  const requests: Array<{ search: string; priority: string | null }> = []
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: name === 'metrics' ? 'unknown' : 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Default', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'default', namespaceCount: 1, generation },
  }
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname === '/api/v1/pods') requests.push({ search: url.search, priority: route.request().headers()['x-kubepeep-list-priority'] ?? null })
    const second = url.searchParams.has('continue')
    const pods = Array.from({ length: 100 }, (_, index) => ({
      namespace: 'default', name: `pod-${String(index + (second ? 100 : 0)).padStart(5, '0')}`, status: 'Running',
      ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false,
    }))
    const data = url.pathname === '/api/v1/status' ? status : url.pathname === '/api/v1/pods' ? pods : []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: {
      generation,
      page: { limit: 100, next: url.pathname === '/api/v1/pods' && !second ? 'next' : '', complete: second, truncated: false, filterScope: 'page' },
      coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
    } }) })
  })

  await page.goto('/pods')
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table).toHaveAttribute('aria-rowcount', '101')
  expect(requests).toHaveLength(1)
  await table.locator('..').evaluate((element) => {
    element.scrollTop = (element.scrollHeight - element.clientHeight) * 0.8
    element.dispatchEvent(new Event('scroll'))
  })
  await expect(table).toHaveAttribute('aria-rowcount', '201')
  expect(requests).toHaveLength(2)
  expect(requests[0].priority).toBeNull()
  expect(requests[1]).toEqual(expect.objectContaining({ priority: 'likely-next', search: expect.stringContaining('continue=next') }))
})

test('retains at most five loaded Pod pages after a sixth cursor page', async ({ page }) => {
  const generation = 'gen_five_pages'
  const requested: number[] = []
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: name === 'metrics' ? 'unknown' : 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Default', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'default', namespaceCount: 1, generation },
  }
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const index = Number(url.searchParams.get('continue')?.replace('page-', '') ?? 0)
    if (url.pathname === '/api/v1/pods') requested.push(index)
    const data = url.pathname === '/api/v1/status' ? status : url.pathname === '/api/v1/pods'
      ? Array.from({ length: 100 }, (_, offset) => ({ namespace: 'default', name: `pod-${String(index * 100 + offset).padStart(3, '0')}`, status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false })) : []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: {
      generation, page: { limit: 100, next: index < 5 ? `page-${index + 1}` : '', complete: index === 5, truncated: index < 5, filterScope: 'page' },
      coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
    } }) })
  })
  await page.goto('/pods')
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table).toHaveAttribute('aria-rowcount', '101')
  for (const [index, count] of [201, 301, 401, 501, 501].entries()) {
    await table.locator('..').evaluate((element) => {
      element.scrollTop = element.scrollHeight
      element.dispatchEvent(new Event('scroll'))
    })
    await expect.poll(() => requested.length).toBeGreaterThanOrEqual(index + 2)
    await expect.poll(async () => Number(await table.getAttribute('aria-rowcount'))).toBeGreaterThanOrEqual(count)
  }
  expect([...new Set(requested)].sort()).toEqual([0, 1, 2, 3, 4, 5])
  expect(requested.length).toBeLessThanOrEqual(6)
  await expect(table).toHaveAttribute('aria-rowcount', '501')
  expect(await table.getByRole('row').count()).toBeLessThan(60)
})

test('deferred prefetch leaves the visible next-page action available', async ({ page }) => {
  const generation = 'gen_deferred'
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: name === 'metrics' ? 'unknown' : 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Default', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'default', namespaceCount: 1, generation },
  }
  let deferred = 0
  let visibleNext = 0
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const next = url.pathname === '/api/v1/pods' && url.searchParams.has('continue')
    const speculative = route.request().headers()['x-kubepeep-list-priority'] === 'likely-next'
    if (next && speculative) {
      deferred += 1
      await route.fulfill({ status: 429, contentType: 'application/json', body: JSON.stringify({ code: 'PREFETCH_DEFERRED', message: 'Visible work has priority.' }) })
      return
    }
    if (next) visibleNext += 1
    const data = url.pathname === '/api/v1/status' ? status : url.pathname === '/api/v1/pods'
      ? Array.from({ length: 100 }, (_, index) => ({ namespace: 'default', name: `pod-${index + (next ? 100 : 0)}`, status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, ageSeconds: 60, problematic: false })) : []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: {
      generation, page: { limit: 100, next: next ? '' : 'next', complete: next, truncated: false, filterScope: 'page' },
      coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
    } }) })
  })
  await page.goto('/pods')
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table).toHaveAttribute('aria-rowcount', '101')
  await table.locator('..').evaluate((element) => {
    element.scrollTop = (element.scrollHeight - element.clientHeight) * 0.8
    element.dispatchEvent(new Event('scroll'))
  })
  await expect.poll(() => deferred).toBe(1)
  await expect(page.getByText('The next page could not be loaded.')).toHaveCount(0)
  await page.getByRole('button', { name: 'Load next page' }).click()
  await expect(table).toHaveAttribute('aria-rowcount', '201')
  expect(visibleNext).toBe(1)
})

test('other resource families keep their own bounded cursor while scrolling', async ({ page }) => {
  const generation = 'gen_families'
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: name === 'metrics' ? 'unknown' : 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Default', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'default', namespaceCount: 1, generation },
  }
  const scenarios = [
    { route: '/workloads', api: '/api/v1/workloads', caption: 'Authorized workload pages', row: (index: number) => ({ kind: 'Deployment', namespace: 'default', name: `workload-${index}`, status: 'Healthy', ready: 1, desired: 1, ageSeconds: 60 }) },
    { route: '/events', api: '/api/v1/events', caption: 'Authorized event pages', row: (index: number) => ({ namespace: 'default', objectKind: 'Pod', objectName: `pod-${index}`, timestamp: '2026-09-21T12:00:00Z', type: 'Normal', reason: 'Started', count: 1, message: 'Started' }) },
    { route: '/network/services', api: '/api/v1/services', caption: 'Authorized services pages', row: (index: number) => ({ namespace: 'default', name: `service-${index}`, type: 'ClusterIP', clusterIPs: [] }) },
    { route: '/config/secrets', api: '/api/v1/secrets', caption: 'Authorized secrets metadata pages', row: (index: number) => ({ apiVersion: 'v1', kind: 'Secret', metadata: { namespace: 'default', name: `secret-${index}`, uid: `uid-${index}`, creationTimestamp: '2026-09-21T12:00:00Z' } }) },
    { route: '/nodes', api: '/api/v1/nodes', caption: 'Authorized node pages', row: (index: number) => ({ name: `node-${index}`, roles: [], status: 'Ready', kubeletVersion: 'v1', internalIP: null, ageSeconds: 60 }) },
  ]
  const requests: Array<{ path: string; cursor: string | null; priority: string | null }> = []
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const scenario = scenarios.find((item) => item.api === url.pathname)
    if (scenario) requests.push({ path: url.pathname, cursor: url.searchParams.get('continue'), priority: route.request().headers()['x-kubepeep-list-priority'] ?? null })
    const second = url.searchParams.has('continue')
    const data = url.pathname === '/api/v1/status' ? status : scenario
      ? Array.from({ length: 100 }, (_, index) => scenario.row(index + (second ? 100 : 0))) : []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: {
      generation, page: { limit: 100, next: scenario && !second ? `cursor-${scenario.api}` : '', complete: second, truncated: false, filterScope: 'page' },
      coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
    } }) })
  })

  for (const scenario of scenarios) {
    await page.goto(scenario.route)
    const table = page.getByRole('table', { name: scenario.caption })
    await expect(table).toHaveAttribute('aria-rowcount', '101')
    await table.locator('..').evaluate((element) => {
      element.scrollTop = (element.scrollHeight - element.clientHeight) * 0.8
      element.dispatchEvent(new Event('scroll'))
    })
    await expect(table).toHaveAttribute('aria-rowcount', '201')
    expect(requests.filter((request) => request.path === scenario.api).map((request) => [request.cursor, request.priority])).toEqual([[null, null], [`cursor-${scenario.api}`, 'likely-next']])
  }
})

test('measures variable-height Event rows without rendering the whole list', async ({ page }) => {
  const generation = 'gen_events'
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: name === 'metrics' ? 'unknown' : 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Default', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'default', namespaceCount: 1, generation },
  }
  const events = Array.from({ length: 1_000 }, (_, index) => ({
    namespace: 'default', objectKind: 'Pod', objectName: `pod-${index}`, reason: 'Started',
    timestamp: new Date(Date.UTC(2026, 8, 21, 12, 0, index)).toISOString(),
    message: index % 2 === 0 ? 'Short event' : 'Long event details '.repeat(30),
    count: 1, source: 'kubelet', type: 'Normal',
  }))
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const data = url.pathname === '/api/v1/status' ? status : url.pathname === '/api/v1/events' ? events : []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: {
      generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' },
      coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
    } }) })
  })
  await page.goto('/events')
  const table = page.getByRole('table', { name: 'Authorized event pages' })
  await expect(table).toHaveAttribute('aria-rowcount', '1001')
  const viewport = table.locator('..')
  await expect.poll(async () => {
    await viewport.evaluate((element) => { element.scrollTop = element.scrollHeight })
    return Number(await table.locator('tbody tr[data-index]').last().getAttribute('data-index'))
  }).toBeGreaterThanOrEqual(995)
  expect(await table.getByRole('row').count()).toBeLessThan(60)
})
