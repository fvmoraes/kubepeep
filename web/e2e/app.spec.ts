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
