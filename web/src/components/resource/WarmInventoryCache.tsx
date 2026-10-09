import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from 'react'
import { useQueryClient, type QueryKey } from '@tanstack/react-query'
import { useAutoRefresh } from './AutoRefreshProvider'
import { autoRefreshInterval, resourceRefreshInterval } from './autoRefresh'

const maximumWarmInventories = 5
const WarmInventoryContext = createContext<((key: QueryKey) => void) | null>(null)
export function useWarmInventoryCache() { return useContext(WarmInventoryContext) }

/** Keep only visited list pages warm. No discovery, detail, YAML or Secret-value reads. */
export function WarmInventoryCache({ children }: { children: ReactNode }) {
  const client = useQueryClient()
  const { enabled } = useAutoRefresh()
  const recent = useRef<QueryKey[]>([])
  const register = useCallback((key: QueryKey) => {
    const hash = JSON.stringify(key)
    recent.current = [...recent.current.filter((item) => JSON.stringify(item) !== hash), key]
    while (recent.current.length > maximumWarmInventories) {
      const oldest = recent.current.shift()!
      const query = client.getQueryCache().find({ queryKey: oldest, exact: true })
      if (query && query.getObserversCount() === 0) {
        void client.cancelQueries({ queryKey: oldest, exact: true })
        client.removeQueries({ queryKey: oldest, exact: true })
      }
    }
  }, [client])
  useEffect(() => {
    if (!enabled) return
    let stopped = false
    let busy = false
    async function refresh() {
      if (busy || document.visibilityState === 'hidden') return
      busy = true
      try {
        // Sequential refresh prevents four background lists competing with
        // the foreground list. React Query deduplicates concurrent requests.
        for (const key of [...recent.current].reverse()) {
          if (stopped) break
          const query = client.getQueryCache().find({ queryKey: key, exact: true })
          if (!query || query.getObserversCount() > 0 || query.state.data === undefined || query.state.fetchStatus !== 'idle'
            || Date.now() - query.state.dataUpdatedAt < autoRefreshInterval || resourceRefreshInterval(query.state.error) === false) continue
          await client.refetchQueries({ queryKey: key, exact: true, type: 'inactive' }, { cancelRefetch: false })
        }
      } finally { busy = false }
    }
    const timer = setInterval(() => void refresh(), autoRefreshInterval)
    return () => { stopped = true; clearInterval(timer) }
  }, [client, enabled])
  const value = useMemo(() => register, [register])
  return <WarmInventoryContext.Provider value={value}>{children}</WarmInventoryContext.Provider>
}
