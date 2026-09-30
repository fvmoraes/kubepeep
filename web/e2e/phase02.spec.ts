import { expect, test } from '@playwright/test'

test('returning to Pods keeps the fresh cache and manual HTTP revalidation preserves it', async ({ page }) => {
  const generation = 'gen_phase02'
  const status = {
    version: 'test', commit: 'test', buildDate: 'test', port: 2748,
    components: Object.fromEntries(['application', 'sqlite', 'kubeconfig', 'context', 'cluster', 'metrics'].map((name) => [name, { status: 'healthy', code: 'TEST', message: 'ready', checkedAt: null }])),
    selection: { clusterProfileId: 1, context: 'development', cluster: 'kind-kubepeep', scopeId: 1, scopeName: 'Default', scopeMode: 'list', scopeSource: 'saved', defaultNamespace: 'default', namespaceCount: 1, generation },
  }
  const pod = { namespace: 'default', name: 'cached-api', status: 'Running', ready: { current: 1, desired: 1 }, restarts: 0, node: 'node-1', ip: '10.0.0.1', owner: null, ageSeconds: 60, problematic: false }
  let podRequests = 0
  let releaseRevalidation: (() => void) | undefined

  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname === '/api/v1/pods' && url.searchParams.has('limit')) {
      podRequests += 1
      if (podRequests > 1) await new Promise<void>((resolve) => { releaseRevalidation = resolve })
    }
    const data = url.pathname === '/api/v1/status' ? status : url.pathname === '/api/v1/session'
      ? { csrfToken: 'csrf_phase02', origin: 'http://127.0.0.1:4173', generation, expiresAt: '2026-09-21T18:00:00Z' }
      : url.pathname === '/api/v1/pods' ? [pod] : []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: {
      generation, collectedAt: '2026-09-21T17:00:00Z',
      page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' },
      coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
    } }) })
  })

  await page.goto('/pods')
  await expect(page.getByText('cached-api')).toBeVisible()
  const navigation = page.getByRole('navigation', { name: 'Primary navigation' })
  await navigation.evaluate((nav) => {
    nav.querySelectorAll<HTMLButtonElement>('button[aria-expanded="false"]').forEach((button) => button.click())
  })
  await navigation.locator('a[href="/workloads/kind/deployments"]').click()
  await expect(page).toHaveURL(/\/workloads\/kind\/deployments$/)
  await navigation.locator('a[href="/pods"]').click()
  try {
    await expect(page.getByText('cached-api')).toBeVisible({ timeout: 1_000 })
    expect(podRequests).toBe(1)
    await page.getByRole('button', { name: 'Refresh now' }).click()
    await expect.poll(() => podRequests).toBe(2)
    await expect(page.getByText('cached-api')).toBeVisible({ timeout: 1_000 })
  } finally {
    releaseRevalidation?.()
  }
})
