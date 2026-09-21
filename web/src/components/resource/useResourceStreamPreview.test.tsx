import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { useResourceStreamPreview } from './useResourceStreamPreview'

type Item = { namespace: string; name: string }
const isItem = (value: unknown): value is Item => Boolean(value && typeof value === 'object' && typeof (value as Item).namespace === 'string' && typeof (value as Item).name === 'string')
const itemKey = (item: Item) => `${item.namespace}/${item.name}`

describe('stream previews', () => {
  it('bounds rows, filters namespaces, and fences generation changes', () => {
    const { result, rerender } = renderHook(({ generation }) => useResourceStreamPreview<Item>({ identity: ['pods', generation], topic: 'pods', namespace: 'payments', isItem, itemKey }), { initialProps: { generation: 'gen_1' } })
    act(() => result.current.onProgress({ topic: 'pods', snapshotId: 'snapshot', items: [
      ...Array.from({ length: 510 }, (_, index) => ({ namespace: 'payments', name: `pod-${index}` })),
      { namespace: 'private', name: 'excluded' },
      { namespace: 'payments', name: 'pod-0' },
    ], completedNamespaces: 1, requestedNamespaces: 2 }))
    expect(result.current.preview?.items).toHaveLength(500)
    expect(result.current.preview?.items.some((item) => item.namespace === 'private')).toBe(false)
    expect(result.current.preview).toMatchObject({ completed: 1, requested: 2 })
    rerender({ generation: 'gen_2' })
    expect(result.current.preview).toBeNull()
    act(() => result.current.onProgress({ topic: 'pods', snapshotId: 'next', items: [{ namespace: 'payments', name: 'new' }], completedNamespaces: 2, requestedNamespaces: 2 }))
    expect(result.current.preview?.items).toEqual([{ namespace: 'payments', name: 'new' }])
    act(() => result.current.onReset())
    expect(result.current.preview).toBeNull()
  })
})
