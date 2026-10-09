import { act, renderHook } from '@testing-library/react'
import { expect, test } from 'vitest'
import { useSelectionBoundState } from './useSelectionBoundState'

test('a pending confirmation cannot reappear after switching selection and returning', () => {
 const {result,rerender}=renderHook(({generation})=>useSelectionBoundState<string|null>([generation],null),{initialProps:{generation:'one'}})
 act(()=>result.current[1]('delete'))
 expect(result.current[0]).toBe('delete')
 rerender({generation:'two'})
 expect(result.current[0]).toBeNull()
 rerender({generation:'one'})
 expect(result.current[0]).toBeNull()
})
