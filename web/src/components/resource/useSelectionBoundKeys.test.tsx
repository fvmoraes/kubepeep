import { act, renderHook } from '@testing-library/react'
import { expect, it } from 'vitest'

import { useSelectionBoundKeys } from './useSelectionBoundKeys'

it('drops checked rows synchronously when the authorized selection changes', () => {
  const { result, rerender } = renderHook(({ generation }) => useSelectionBoundKeys(['profile', 'context', 'scope', generation]), { initialProps: { generation: 'old' } })
  act(() => result.current[1](new Set(['payments/api'])))
  expect(result.current[0].has('payments/api')).toBe(true)
  rerender({ generation: 'new' })
  expect(result.current[0].size).toBe(0)
  act(() => result.current[1](new Set(['payments/worker'])))
  expect([...result.current[0]]).toEqual(['payments/worker'])
  rerender({ generation: 'old' })
  expect(result.current[0].size).toBe(0)
})
