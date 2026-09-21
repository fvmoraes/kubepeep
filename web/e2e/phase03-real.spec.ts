import { expect, test } from '@playwright/test'

test.skip(!process.env.KUBEPEEP_PHASE03_ORIGIN, 'Requires the opt-in isolated Kind browser probe')

test('loads the 50-namespace Kind Pod scope with real authorization and paints a bounded first page', async ({ page }) => {
  if (process.env.KUBEPEEP_PHASE03_NO_STREAM) {
    await page.route('**/api/v1/stream?*', (route) => route.abort())
  }
  const requests = new Map<object, number>()
  const network: Array<{ path: string; ms: number; status?: number }> = []
  page.on('request', (request) => { requests.set(request, Date.now()) })
  page.on('requestfinished', async (request) => {
    const started = requests.get(request)
    if (started === undefined) return
    const path = new URL(request.url()).pathname
    if (path.startsWith('/api/v1/')) network.push({ path, ms: Date.now() - started, status: (await request.response())?.status() })
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Workloads', exact: true }).click()
  const podsLink = page.getByRole('link', { name: 'Pods', exact: true })
  await expect(podsLink).toBeVisible()
  const openedAt = await page.evaluate(() => performance.now())
  await podsLink.click()
  const table = page.getByRole('table', { name: 'Authorized Pod pages' })
  await expect(table.getByRole('button', { name: /Open Pod pod-000000 in kp-bench-/ }).first()).toBeVisible({ timeout: 20_000 })
  const visibleAfterClickMs = await page.evaluate((start) => performance.now() - start, openedAt)
  await expect.poll(async () => (await page.evaluate(() => window.__KUBEPEEP_UX_METRICS__?.snapshot() ?? [])).some((sample) => sample.name === 'time_to_first_row' && sample.view === 'pods'), { timeout: 20_000 }).toBe(true)
  const samples = await page.evaluate(() => window.__KUBEPEEP_UX_METRICS__?.snapshot() ?? [])
  const firstRow = samples.find((sample) => sample.name === 'time_to_first_row' && sample.view === 'pods')
  const pageComplete = samples.find((sample) => sample.name === 'time_to_page_complete' && sample.view === 'pods')
  console.log(`Phase 03 real Kind 50x10: ${JSON.stringify({ firstRowMs: firstRow?.value, pageCompleteMs: pageComplete?.value, visibleAfterClickMs, domRows: await table.getByRole('row').count(), liveStatus: await page.getByRole('region', { name: 'Resource live updates' }).innerText(), network })}`)
  expect(await table.getByRole('row').count()).toBeLessThan(110)
  expect(firstRow?.value).toBeDefined()
  expect(pageComplete?.value).toBeDefined()
})
