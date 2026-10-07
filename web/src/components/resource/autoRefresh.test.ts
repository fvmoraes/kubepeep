import { QueryClient } from '@tanstack/react-query'
import { expect, it } from 'vitest'
import { APIError } from '../../api/client'
import { inventoryRefreshInterval, resourceRefreshInterval } from './autoRefresh'

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

it.each([
  [503, 'AUTHORIZATION_UNAVAILABLE', 15_000],
  [504, 'UPSTREAM_TIMEOUT', 15_000],
  [401, 'AUTHENTICATION_UNAVAILABLE', false],
  [403, 'FORBIDDEN', false],
  [409, 'GENERATION_CHANGED', false],
] as const)('uses a bounded recovery interval for %s/%s', (status, code, interval) => {
  const error = new APIError(status, { code, message: 'Resource request failed.' })
  expect(resourceRefreshInterval(error)).toBe(interval)
  for (const data of [undefined, { pages: [{ items: [] }], pageParams: [''] }]) {
    expect(inventoryRefreshInterval({ queryKey: ['resources', 'pods'], state: { error, data } })).toBe(interval)
  }
  expect(inventoryRefreshInterval({ queryKey: ['resources', 'pod-detail'], state: { error, data: undefined } })).toBe(false)
  expect(inventoryRefreshInterval({ queryKey: ['resources', 'secrets'], state: { error, data: { entries: [] } } })).toBe(false)
})
