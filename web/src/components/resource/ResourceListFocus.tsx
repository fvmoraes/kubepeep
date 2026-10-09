import { createContext, useContext } from 'react'

export const ResourceListFocus = createContext<{ name: string; clear: () => void } | null>(null)
export function useResourceListFocus() { return useContext(ResourceListFocus) }
