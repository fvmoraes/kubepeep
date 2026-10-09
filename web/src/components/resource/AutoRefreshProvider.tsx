import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'
import { resourceRefreshInterval } from './autoRefresh'

const AutoRefreshContext = createContext({ enabled: true, toggle: () => {} })

export function AutoRefreshProvider({ children }: { children: ReactNode }) {
  const [enabled, setEnabled] = useState(true)
  const value = useMemo(() => ({ enabled, toggle: () => setEnabled((current) => !current) }), [enabled])
  return <AutoRefreshContext.Provider value={value}>{children}</AutoRefreshContext.Provider>
}

export function useAutoRefresh() { return useContext(AutoRefreshContext) }

export function useAutoRefreshQueryOptions() {
  const { enabled } = useAutoRefresh()
  return {
    refetchInterval: enabled ? (query: { state: { error: Error | null } }) => resourceRefreshInterval(query.state.error) : false as const,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: enabled,
    refetchOnReconnect: enabled,
  }
}

export function AutoRefreshToggle() {
  const { enabled, toggle } = useAutoRefresh()
  return <button type="button" className={`control inline-flex items-center gap-2 px-2 ${enabled ? 'text-kp-green' : 'text-kp-overlay-text'}`} aria-label="Automatic refresh every 10 seconds" aria-pressed={enabled} title={enabled ? 'Disable automatic refresh' : 'Enable automatic refresh every 10 seconds'} onClick={toggle}>
    <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-current" />Auto · {enabled ? '10s' : 'off'}
  </button>
}
