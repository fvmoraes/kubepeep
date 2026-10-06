import { QueryClient } from '@tanstack/react-query'
import { expect, it } from 'vitest'
import { APIError } from '../../api/client'
import { inventoryRefreshInterval } from './autoRefresh'

it('polls bounded inventories but never denied queries or explicit data/detail reads', () => {
  const client = new QueryClient()
  const query = client.getQueryCache().build(client, { queryKey: ['resources', 'pods'] })
  expect(inventoryRefreshInterval(query)).toBe(15_000)
  query.setData({ page: { next: '' }, items: [] })
  expect(inventoryRefreshInterval(query)).toBe(15_000)
  query.setData({ pages: [{ items: [] }], pageParams: [''] })
  expect(inventoryRefreshInterval(query)).toBe(15_000)
  query.setState({ error: new APIError(403, { code: 'FORBIDDEN', message: 'Denied' }) })
  expect(inventoryRefreshInterval(query)).toBe(false)
  query.setState({ error: null })
  query.setData({ entries: [{ key: 'secret', value: 'value' }] })
  expect(inventoryRefreshInterval(query)).toBe(false)
  const detail = client.getQueryCache().build(client, { queryKey: ['resources', 'pod-detail'] })
  expect(inventoryRefreshInterval(detail)).toBe(false)
  client.clear()
})
