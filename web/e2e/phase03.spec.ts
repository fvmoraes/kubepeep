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
  expect(timings.find((sample) => sample.name === 'time_to_shell_ready')?.value).toBeLessThan(800)
  expect(timings.find((sample) => sample.name === 'time_to_first_row')?.value).toBeLessThan(500)
  expect(timings.find((sample) => sample.name === 'time_to_page_complete')?.value).toBeLessThan(1_500)

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
    return { fps: (timestamps.length - 1) * 1_000 / (timestamps.at(-1)! - timestamps[0]), p95FrameMs: intervals[Math.floor(intervals.length * 0.95)] ?? 0, renderedRows: element.querySelectorAll('tbody tr[data-index]').length }
  })
  console.log(`Phase 03 synthetic 12k continuous scroll: ${JSON.stringify(frameRate)}`)
  expect(frameRate.renderedRows).toBeLessThan(60)
  expect(frameRate.fps).toBeGreaterThanOrEqual(50)
})

test('prefetches only the next authorized page after 75% scroll', async ({ page }) => {
  const generation = 'gen_prefetch'
  const requests: string[] = []
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: name === 'metrics' ? 'unknown' : 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Default', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'default', namespaceCount: 1, generation },
  }
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname === '/api/v1/pods') requests.push(url.search)
    const second = url.searchParams.has('continue')
    const pods = Array.from({ length: second ? 100 : 200 }, (_, index) => ({
      namespace: 'default', name: `pod-${String(index + (second ? 200 : 0)).padStart(5, '0')}`, status: 'Running',
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
  await expect(table).toHaveAttribute('aria-rowcount', '201')
  expect(requests).toHaveLength(1)
  await table.locator('..').evaluate((element) => {
    element.scrollTop = (element.scrollHeight - element.clientHeight) * 0.8
    element.dispatchEvent(new Event('scroll'))
  })
  await expect(table).toHaveAttribute('aria-rowcount', '301')
  expect(requests).toHaveLength(2)
  expect(requests[1]).toContain('continue=next')
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
