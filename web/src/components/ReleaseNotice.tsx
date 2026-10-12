import { useQuery } from '@tanstack/react-query'
import { getReleaseUpdate } from '../api/client'

const interval = 2 * 60 * 60 * 1000

export function ReleaseNotice() {
  const update = useQuery({
    queryKey: ['release-update'], queryFn: ({ signal }) => getReleaseUpdate(signal),
    staleTime: interval, refetchInterval: interval, refetchIntervalInBackground: true,
    refetchOnWindowFocus: false, refetchOnReconnect: false, retry: false,
  })
  if (!update.data?.available || !update.data.latestVersion) return null
  return <footer className="release-notice" aria-label="Application update">
    <a href="https://kubepeep.online/download" target="_blank" rel="noopener noreferrer" onClick={event => {
      const runtime = (window as Window & { runtime?: { BrowserOpenURL?: (url: string) => void } }).runtime
      if (runtime?.BrowserOpenURL) { event.preventDefault(); runtime.BrowserOpenURL('https://kubepeep.online/download') }
    }}>KubePeep {update.data.latestVersion} available · Download</a>
  </footer>
}
