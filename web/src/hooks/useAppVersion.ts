import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'

import { getStatus } from '../api/client'
import { desktopPlatform } from '../api/desktop'

const fallbackVersion = 'dev'

/**
 * Application version from a single shared source: the desktop bridge
 * (ldflags buildinfo) with the local status endpoint as fallback.
 * Never hardcodes a release number in the UI.
 */
export function useAppVersion(): string {
  const [desktopVersion, setDesktopVersion] = useState<string | null>(null)
  useEffect(() => {
    let active = true
    void desktopPlatform().then((info) => {
      if (active && info?.version) setDesktopVersion(info.version)
    })
    return () => {
      active = false
    }
  }, [])
  const status = useQuery({
    queryKey: ['local-status'],
    queryFn: ({ signal }) => getStatus(signal),
    staleTime: 15_000,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const remote = typeof status.data?.version === 'string' && status.data.version !== '' ? status.data.version : null
  return desktopVersion ?? remote ?? fallbackVersion
}
