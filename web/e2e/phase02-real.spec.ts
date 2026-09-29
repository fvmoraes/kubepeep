import { expect, test } from '@playwright/test'

test.skip(!process.env.KUBEPEEP_PHASE02_ORIGIN, 'Requires the opt-in isolated Phase 2 Kind probe')

test('reuses one Pod watch and a fresh page across Pods to Deployments navigation', async ({ page }) => {
  const podResponses: number[] = []
  page.on('response', (response) => {
    if (new URL(response.url()).pathname === '/api/v1/pods') podResponses.push(response.status())
  })

  await page.goto('/pods')
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table).toHaveAttribute('aria-rowcount', '101', { timeout: 30_000 })
  const firstPod = table.getByRole('button', { name: /^Open Pod / }).first()
  await expect(firstPod).toBeVisible()
  const liveUpdates = page.getByRole('region', { name: 'Resource live updates' })
  await expect(liveUpdates).toContainText(/✓ (\d+)\/\1 namespaces/, { timeout: 30_000 })
  const liveStatusBeforeNavigation = await liveUpdates.innerText()
  const metricsBeforeNavigation = await page.request.get('/metrics')
  const metricsBeforeText = await metricsBeforeNavigation.text()
  const watchBeforeMatch = metricsBeforeText.match(/^kubepeep_watch_active\{[^}]*resource="pods"[^}]*\}\s+([0-9.]+)/m)
  const activePodWatchesBeforeNavigation = Number(watchBeforeMatch?.[1] ?? Number.NaN)
  const responsesBeforeNavigation = podResponses.length

  await page.getByRole('link', { name: 'Deployments', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Workloads', exact: true })).toBeVisible()
  await page.evaluate(() => {
    const state = { startedAt: 0, visibleAt: 0 }
    ;(window as Window & { __phase02Navigation?: typeof state }).__phase02Navigation = state
    const link = document.querySelector<HTMLAnchorElement>('a[href="/pods"]')
    link?.addEventListener('click', () => { state.startedAt = performance.now() }, { once: true })
    new MutationObserver(() => {
      if (state.startedAt && !state.visibleAt && location.pathname === '/pods' && document.querySelector('table button[aria-label^="Open Pod"]')) {
        state.visibleAt = performance.now()
      }
    }).observe(document.body, { subtree: true, childList: true })
  })
  await page.getByRole('link', { name: 'Pods', exact: true }).click()
  await expect(firstPod).toBeVisible()
  const returnMs = await page.evaluate(() => {
    const state = (window as Window & { __phase02Navigation?: { startedAt: number; visibleAt: number } }).__phase02Navigation
    return (state?.visibleAt ?? 0) - (state?.startedAt ?? 0)
  })

  await page.waitForTimeout(200)
  const metricsResponse = await page.request.get('/metrics')
  const metrics = await metricsResponse.text()
  const watchLines = metrics.split('\n').filter((line) => line.includes('kubepeep_watch_active'))
  const watchMatches = metrics.match(/^kubepeep_watch_active\{[^}]*resource="pods"[^}]*\}\s+([0-9.]+)/m)
  const activePodWatches = Number(watchMatches?.[1] ?? Number.NaN)
  const report = {
    returnMs,
    podHTTPResponsesBeforeNavigation: responsesBeforeNavigation,
    podHTTPResponsesAfterReturn: podResponses.length,
    activePodWatches,
    activePodWatchesBeforeNavigation,
    metricsStatus: metricsResponse.status(),
    watchLines,
    liveStatusBeforeNavigation,
    domRows: await table.getByRole('row').count(),
  }
  console.log(`Phase 02 real Kind navigation: ${JSON.stringify(report)}`)

  expect(returnMs).toBeLessThan(100)
  expect(podResponses.length).toBe(responsesBeforeNavigation)
  expect(activePodWatchesBeforeNavigation).toBe(1)
  expect(activePodWatches).toBe(1)
})

test('starts infrastructure tier after summary and marks it unrelated', async ({ page }) => {
  let summaryFinishedAt = 0
  const infrastructure: Array<{ path: string; priority: string; startedAt: number }> = []
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (path === '/api/v1/nodes' || path === '/api/v1/persistent-volume-claims') {
      infrastructure.push({ path, priority: request.headers()['x-kubepeep-list-priority'] ?? '', startedAt: Date.now() })
    }
  })
  page.on('response', (response) => {
    if (new URL(response.url()).pathname === '/api/v1/dashboard/summary') summaryFinishedAt = Date.now()
  })

  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Cluster overview' })).toBeVisible({ timeout: 30_000 })
  await expect.poll(() => infrastructure.length, { timeout: 20_000 }).toBe(2)
  const report = { summaryFinishedAt, infrastructure }
  console.log(`Phase 02 real Kind tiers: ${JSON.stringify(report)}`)

  expect(summaryFinishedAt).toBeGreaterThan(0)
  expect(infrastructure.map((item) => item.path).sort()).toEqual(['/api/v1/nodes', '/api/v1/persistent-volume-claims'])
  expect(infrastructure.every((item) => item.priority === 'unrelated' && item.startedAt >= summaryFinishedAt)).toBe(true)
})
