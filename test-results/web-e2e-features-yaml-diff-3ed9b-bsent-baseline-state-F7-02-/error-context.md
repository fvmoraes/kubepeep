# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: web/e2e/features.spec.ts >> yaml diff renders added and removed lines and the absent baseline state (F7-02)
- Location: web/e2e/features.spec.ts:131:1

# Error details

```
Error: page.goto: Protocol error (Page.navigate): Cannot navigate to invalid URL
Call log:
  - navigating to "/pods", waiting until "load"

```

# Test source

```ts
  32  |     logs: { version: 1 as const, items: [] },
  33  |   },
  34  |   favorites: { version: 1 as const, items: [] as Array<{ id: string; kind: string; namespace: string; name: string }> },
  35  | }
  36  | 
  37  | const block = (value: unknown) => ({
  38  |   value, complete: true, truncated: false,
  39  |   coverage: { requestedNamespaces: 1, completedNamespaces: 1, deniedNamespaces: [], failed: [] },
  40  |   errors: [],
  41  | })
  42  | 
  43  | const meta = { generation }
  44  | 
  45  | test.beforeEach(async ({ context }) => {
  46  |   preferences = {
  47  |     version: 1,
  48  |     ui: { language: 'en' },
  49  |     logs: { wrap: false, timestamps: true, tailLines: 200 },
  50  |     dashboard: { logScanWindow: '15m', sectionOrder: ['summary'], hiddenSections: [] },
  51  |     filters: {
  52  |       workloads: { version: 1, items: [] },
  53  |       pods: { version: 1, items: [] },
  54  |       events: { version: 1, items: [] },
  55  |       logs: { version: 1, items: [] },
  56  |     },
  57  |     favorites: { version: 1, items: [] },
  58  |   }
  59  |   await context.route('**/api/v1/**', async (route) => {
  60  |     const request = route.request()
  61  |     const url = new URL(request.url())
  62  |     const path = `${url.pathname}${url.search}`
  63  |     let data: unknown = []
  64  |     let responseMeta: Record<string, unknown> = meta
  65  |     if (url.pathname === '/api/v1/status') data = statusPayload()
  66  |     else if (url.pathname === '/api/v1/cluster/profiles') data = []
  67  |     else if (url.pathname === '/api/v1/session') data = { csrfToken: 'csrf_features', origin: 'http://127.0.0.1:4173', generation, expiresAt: '2026-09-02T18:00:00Z' }
  68  |     else if (url.pathname === '/api/v1/preferences' && request.method() === 'PUT') {
  69  |       preferences = request.postDataJSON() as typeof preferences
  70  |       data = preferences
  71  |     } else if (url.pathname === '/api/v1/preferences') data = preferences
  72  |     else if (url.pathname === '/api/v1/pods' && url.searchParams.has('limit')) {
  73  |       data = [pod]
  74  |       responseMeta = { generation, page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' }, coverage: null }
  75  |     } else if (url.pathname === '/api/v1/pods/payments/api-abc/yaml') {
  76  |       // Served as raw YAML text, not the JSON envelope.
  77  |       await route.fulfill({ status: 200, contentType: 'application/yaml; charset=utf-8', body: 'apiVersion: v1\nkind: Pod\nmetadata:\n  name: api-abc\n  namespace: payments\n  annotations:\n    example: "1"\nspec:\n  replicas: 1\n' })
  78  |       return
  79  |     } else if (url.pathname === '/api/v1/resources/pods/payments/api-abc/yaml-diff') {
  80  |       data = {
  81  |         absent: false, truncated: false,
  82  |         lines: [
  83  |           { kind: 'same', text: 'metadata:' },
  84  |           { kind: 'removed', text: '  example: "1"' },
  85  |           { kind: 'added', text: '  example: "2"' },
  86  |         ],
  87  |       }
  88  |     } else if (url.pathname === '/api/v1/resources/deployments/payments/api/yaml-diff') {
  89  |       data = { absent: true, truncated: false, lines: [] }
  90  |     }
  91  |     await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: responseMeta }) })
  92  |   })
  93  | })
  94  | 
  95  | test('global search navigates to resources already loaded in the session (F7-04)', async ({ page }) => {
  96  |   await page.goto('/pods')
  97  |   await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()
  98  |   await page.waitForTimeout(300) // allow the bounded pod page cache to populate
  99  | 
  100 |   await page.keyboard.press('Control+k')
  101 |   const input = page.getByRole('combobox', { name: 'Search application pages' })
  102 |   await input.fill('api-abc')
  103 |   const option = page.getByRole('option', { name: /api-abc/ })
  104 |   await expect(option).toBeVisible()
  105 |   await option.click()
  106 |   await expect(page).toHaveURL(/\/pods\/payments\/api-abc$/)
  107 | })
  108 | 
  109 | test('favorite star persists through the allowlisted preferences PUT (F7-01)', async ({ page }) => {
  110 |   await page.goto('/pods')
  111 |   await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()
  112 |   await page.waitForTimeout(300)
  113 | 
  114 |   await page.getByRole('link', { name: /Open Pod api-abc in payments/i }).click()
  115 |   await expect(page).toHaveURL(/\/pods\/payments\/api-abc$/)
  116 |   const star = page.getByRole('button', { name: 'Add api-abc to favorites' })
  117 |   await expect(star).toBeDisabled() // preferences still loading
  118 |   await expect(star).toBeEnabled()
  119 |   await star.click()
  120 | 
  121 |   await expect.poll(() => preferences.favorites?.items.length).toBe(1)
  122 |   expect(preferences.favorites.items[0]).toMatchObject({ kind: 'pod', namespace: 'payments', name: 'api-abc' })
  123 |   await expect(page.getByRole('button', { name: 'Remove api-abc from favorites' })).toBeVisible()
  124 | 
  125 |   await page.keyboard.press('Control+k')
  126 |   const input = page.getByRole('combobox', { name: 'Search application pages' })
  127 |   await input.fill('favorite')
  128 |   await expect(page.getByRole('option', { name: /deployment · payments|pod · payments/i }).first()).toBeVisible()
  129 | })
  130 | 
  131 | test('yaml diff renders added and removed lines and the absent baseline state (F7-02)', async ({ page }) => {
> 132 |   await page.goto('/pods')
      |              ^ Error: page.goto: Protocol error (Page.navigate): Cannot navigate to invalid URL
  133 |   await expect(page.getByRole('heading', { name: 'Pods' })).toBeVisible()
  134 |   await page.waitForTimeout(300)
  135 | 
  136 |   await page.getByRole('link', { name: /Open Pod api-abc in payments/i }).click()
  137 |   await page.getByRole('button', { name: 'Load authorized YAML' }).click()
  138 |   await expect(page.getByLabel('YAML document')).toBeVisible()
  139 | 
  140 |   await page.getByRole('button', { name: 'Diff vs last-applied' }).click()
  141 |   const diff = page.getByLabel('YAML diff against last-applied')
  142 |   await expect(diff).toBeVisible()
  143 |   await expect(diff).toContainText('-   example: "1"')
  144 |   await expect(diff).toContainText('+   example: "2"')
  145 | 
  146 |   await page.goto('/workloads')
  147 |   await page.waitForTimeout(300)
  148 | })
  149 | 
```