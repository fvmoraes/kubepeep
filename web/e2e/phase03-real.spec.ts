import { expect, test } from '@playwright/test'

test.skip(!process.env.KUBEPEEP_PHASE03_ORIGIN, 'Requires the opt-in isolated Kind browser probe')

test('loads the 50-namespace Kind Pod scope with real authorization and paints a bounded first page', async ({ page }) => {
  if (process.env.KUBEPEEP_PHASE03_NO_STREAM) {
    await page.route('**/api/v1/stream?*', (route) => route.abort())
  }
  if (process.env.KUBEPEEP_PHASE03_DELAY_HTTP) {
    await page.route('**/api/v1/pods?*', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 3_000))
      await route.continue()
    })
  }
  const requests = new Map<object, number>()
  let clickWallTime = 0
  const network: Array<{ path: string; ms: number; status?: number }> = []
  page.on('request', (request) => {
    requests.set(request, Date.now())
    if (clickWallTime && new URL(request.url()).pathname.startsWith('/api/v1/')) {
      console.log(`Phase 03 request ${new URL(request.url()).pathname} started ${Date.now() - clickWallTime}ms after click`)
    }
  })
  page.on('response', (response) => {
    if (new URL(response.url()).pathname === '/api/v1/stream') {
      console.log(`Phase 03 stream headers arrived ${Date.now() - clickWallTime}ms after click`)
    }
  })
  page.on('requestfinished', async (request) => {
    const started = requests.get(request)
    if (started === undefined) return
    const path = new URL(request.url()).pathname
    if (path.startsWith('/api/v1/')) network.push({ path, ms: Date.now() - started, status: (await request.response())?.status() })
  })
  await page.goto('/')
  const dwellMs = Number(process.env.KUBEPEEP_PHASE03_NAV_DWELL_MS ?? 1_000)
  if (dwellMs > 0) await page.waitForTimeout(dwellMs)
  await page.getByRole('button', { name: 'Workloads', exact: true }).click()
  const podsLink = page.getByRole('link', { name: 'Pods', exact: true })
  await expect(podsLink).toBeVisible()
  await page.evaluate(() => {
    const markers = { firstPreviewAt: 0, firstPodRowAt: 0 }
    ;(window as Window & { __phase03Markers?: typeof markers }).__phase03Markers = markers
    new MutationObserver(() => {
      const live = document.querySelector('[aria-label="Resource live updates"]')?.textContent ?? ''
      if (!markers.firstPreviewAt && /[1-9]\d* snapshot items received/.test(live)) markers.firstPreviewAt = performance.now()
      if (!markers.firstPodRowAt && document.querySelector('table button[aria-label^="Open Pod"]')) markers.firstPodRowAt = performance.now()
    }).observe(document.body, { subtree: true, childList: true, characterData: true })
  })
  const openedAt = await page.evaluate(() => performance.now())
  clickWallTime = Date.now()
  await podsLink.click()
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table.getByRole('button', { name: /Open Pod pod-000000 in kp-bench-/ }).first()).toBeVisible({ timeout: 20_000 })
  const visibleAfterClickMs = await page.evaluate((start) => performance.now() - start, openedAt)
  await expect(table).toHaveAttribute('aria-rowcount', '101', { timeout: 20_000 })
  const fullPageAfterClickMs = await page.evaluate((start) => performance.now() - start, openedAt)
  const samples = await page.evaluate(() => window.__KUBEPEEP_UX_METRICS__?.snapshot() ?? [])
  const markers = await page.evaluate((start) => {
    const value = (window as Window & { __phase03Markers?: { firstPreviewAt: number; firstPodRowAt: number } }).__phase03Markers
    return { firstPreviewMs: value?.firstPreviewAt ? value.firstPreviewAt - start : null, firstPodRowMs: value?.firstPodRowAt ? value.firstPodRowAt - start : null }
  }, openedAt)
  const firstRow = samples.find((sample) => sample.name === 'time_to_first_row' && sample.view === 'pods')
  const firstVisibleRow = samples.find((sample) => sample.name === 'time_to_first_visible_row' && sample.view === 'pods')
  const pageComplete = samples.find((sample) => sample.name === 'time_to_page_complete' && sample.view === 'pods')
  console.log(`Phase 03 real Kind 50x10: ${JSON.stringify({ dwellMs, firstRowMs: firstRow?.value, firstVisibleRowMs: firstVisibleRow?.value, pageCompleteMs: pageComplete?.value, visibleAfterClickMs, fullPageAfterClickMs, ...markers, domRows: await table.getByRole('row').count(), liveStatus: await page.getByRole('region', { name: 'Resource live updates' }).innerText(), network })}`)
  expect(await table.getByRole('row').count()).toBeLessThan(110)
  expect(markers.firstPodRowMs).not.toBeNull()
  expect(firstVisibleRow?.value).toBeDefined()
  expect(network.some((item) => item.path === '/api/v1/pods' && item.status === 200)).toBe(true)
  expect(network.filter((item) => item.path === '/api/v1/preferences').every((item) => item.status === 200)).toBe(true)
  if (dwellMs >= 1_000 && !process.env.KUBEPEEP_PHASE03_DELAY_HTTP) {
    expect(markers.firstPodRowMs).toBeLessThan(500)
    expect(firstVisibleRow!.value).toBeLessThan(500)
    expect(fullPageAfterClickMs).toBeLessThan(1_500)
  }
})

test('loads all five real Pod pages through the native cursor with bounded DOM and heap', async ({ page }) => {
  test.skip(Boolean(process.env.KUBEPEEP_PHASE03_ONLY_FIRST), 'First-paint calibration')
  const pageResponses: Array<{ continued: boolean; status: number; count?: number; next?: boolean; complete?: boolean }> = []
  page.on('response', async (response) => {
    const url = new URL(response.url())
    if (url.pathname === '/api/v1/pods') {
      const payload = await response.json().catch(() => null) as { data?: unknown[]; meta?: { page?: { next?: string; complete?: boolean } } } | null
      pageResponses.push({ continued: url.searchParams.has('continue'), status: response.status(), count: payload?.data?.length, next: Boolean(payload?.meta?.page?.next), complete: payload?.meta?.page?.complete })
    }
  })
  await page.goto('/pods')
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table).toHaveAttribute('aria-rowcount', '101', { timeout: 20_000 })
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('Performance.enable')
  const usedHeap = async () => {
    const metrics = await cdp.send('Performance.getMetrics')
    return metrics.metrics.find((item) => item.name === 'JSHeapUsedSize')?.value ?? 0
  }
  const initialHeapBytes = await usedHeap()
  const viewport = table.locator('..')
  for (const expected of [201, 301, 401, 501]) {
    await viewport.evaluate((element) => {
      element.scrollTop = element.scrollHeight
      element.dispatchEvent(new Event('scroll'))
    })
    try {
      await expect.poll(async () => Number(await table.getAttribute('aria-rowcount')), { timeout: 8_000 }).toBeGreaterThanOrEqual(expected)
    } catch (error) {
      console.log(`Phase 03 real page stall: ${JSON.stringify({ expected, pageResponses, visibleText: (await page.locator('main').innerText()).slice(-1200) })}`)
      throw error
    }
    expect(await table.getByRole('row').count()).toBeLessThan(60)
  }
  await viewport.evaluate((element) => { element.scrollTop = element.scrollHeight })
  await expect(table).toHaveAttribute('aria-rowcount', '501')
  await expect(page.getByRole('button', { name: 'Load next page' })).toBeDisabled()
  expect(pageResponses.at(-1)).toEqual(expect.objectContaining({ next: false, complete: true }))
  await expect(table.getByRole('button', { name: 'Open Pod pod-000009 in kp-bench-0049' })).toBeVisible()
  const heapGrowthBytes = await usedHeap() - initialHeapBytes
  console.log(`Phase 03 real Kind 50x10 cursor: ${JSON.stringify({ pages: 5, rows: 500, domRows: await table.getByRole('row').count(), initialHeapBytes, heapGrowthBytes })}`)
  expect(heapGrowthBytes).toBeLessThan(30 * 1024 * 1024)
})
