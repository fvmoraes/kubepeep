import { useCallback, useState } from 'react'

/** Cursors belong to one generation and namespace/filter binding. */
export function useGenerationCursor(generation: string | undefined, binding = ''): [string, (value: string) => void] {
  const key = JSON.stringify([generation, binding])
  const [state, setState] = useState({ key, value: '' })
  const changed = state.key !== key
  if (changed) setState({ key, value: '' })
  const setValue = useCallback((value: string) => setState({ key, value }), [key])
  return [changed ? '' : state.value, setValue]
}

export function useGenerationCursorMap<K extends string>(generation: string | undefined, empty: Record<K, string>, binding = ''): [Record<K, string>, (key: K, value: string) => void] {
  const key = JSON.stringify([generation, binding])
  const [state, setState] = useState(() => ({ key, values: { ...empty } }))
  const changed = state.key !== key
  if (changed) setState({ key, values: { ...empty } })
  const setValue = useCallback((name: K, value: string) => setState((current) => ({
    key,
    values: { ...(current.key === key ? current.values : empty), [name]: value },
  })), [empty, key])
  return [changed ? empty : state.values, setValue]
}
