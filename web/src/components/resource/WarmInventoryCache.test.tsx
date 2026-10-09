import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { AutoRefreshProvider, AutoRefreshToggle } from './AutoRefreshProvider'
import { WarmInventoryCache } from './WarmInventoryCache'
import { useInfiniteCollection } from './useInfiniteCollection'

afterEach(() => { cleanup(); vi.useRealTimers() })

it('keeps five visited inventories warm, reuses rows, and stops with auto refresh disabled', async () => {
  vi.useFakeTimers()
  const counts = new Map<string, number>()
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  function List({ name }: { name: string }) {
    const collection = useInfiniteCollection({ identity: ['resources', name, 'gen'], filters: {}, enabled: true, fetchPage: async () => {
      const value = (counts.get(name) ?? 0) + 1; counts.set(name, value)
      return { items: [{ name, value }], page: { limit: 100, next: '', complete: true, truncated: false, filterScope: 'collection' as const }, coverage: null }
    } })
    return <p>{collection.items.map((item) => `${item.name}:${item.value}`).join()}</p>
  }
  function Tree({ name }: { name: string }) { return <QueryClientProvider client={client}><AutoRefreshProvider><WarmInventoryCache><AutoRefreshToggle /><List key={name} name={name} /></WarmInventoryCache></AutoRefreshProvider></QueryClientProvider> }
  const view = render(<Tree name="pods" />)
  const settle = async () => { await act(async () => { await vi.advanceTimersByTimeAsync(10) }) }
  await settle()
  expect(screen.getByText('pods:1')).toBeVisible()
  view.rerender(<Tree name="workloads" />); await settle()
  await act(async () => { await vi.advanceTimersByTimeAsync(10_100) })
  expect(counts.get('pods')).toBe(2)
  view.rerender(<Tree name="pods" />)
  expect(screen.getByText('pods:2')).toBeVisible()
  expect(counts.get('pods')).toBe(2)
  for (const name of ['services', 'ingresses', 'configmaps', 'secrets']) { view.rerender(<Tree name={name} />); await settle() }
  const inventories = client.getQueryCache().findAll({ queryKey: ['resources'] })
  expect(inventories).toHaveLength(5)
  expect(inventories.some((query) => query.queryKey[1] === 'workloads')).toBe(false)
  fireEvent.click(screen.getByRole('button', { name: 'Automatic refresh every 10 seconds' }))
  const before = [...counts]
  await act(async () => { await vi.advanceTimersByTimeAsync(30_000) })
  expect([...counts]).toEqual(before)
  client.clear()
})
