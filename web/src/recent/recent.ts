// In-memory recent-targets store (F5/V5-12). Nothing here persists: the store
// lives for the current session, never writes browser storage and never records
// Secrets. Persistence and migration are the F6 contract.

export interface RecentTarget {
  path: string
  kind: string
  namespace: string | null
  name: string
  recordedAt: number
}

export const recentLimit = 20
export const recentExpirationMs = 30 * 24 * 60 * 60 * 1000

let entries: RecentTarget[] = []
const listeners = new Set<() => void>()

function emit() {
  for (const listener of listeners) listener()
}

// Recorded detail kinds. Secrets are deliberately absent: visiting a Secret
// never becomes history.
const recordablePrefixes = [
  { prefix: '/pods/', kind: 'Pod' },
  { prefix: '/workloads/deployments/', kind: 'Deployment' },
  { prefix: '/workloads/statefulsets/', kind: 'StatefulSet' },
  { prefix: '/workloads/daemonsets/', kind: 'DaemonSet' },
  { prefix: '/workloads/jobs/', kind: 'Job' },
  { prefix: '/workloads/cronjobs/', kind: 'CronJob' },
  { prefix: '/workloads/replicasets/', kind: 'ReplicaSet' },
  { prefix: '/leases/', kind: 'Lease' },
  { prefix: '/storage/persistent-volume-claims/', kind: 'PersistentVolumeClaim' },
  { prefix: '/service-accounts/', kind: 'ServiceAccount' },
  { prefix: '/network/endpoints/', kind: 'Endpoints' },
  { prefix: '/network/network-policies/', kind: 'NetworkPolicy' },
  { prefix: '/access/roles/', kind: 'Role' },
  { prefix: '/access/role-bindings/', kind: 'RoleBinding' },
]

function kindForPath(path: string): { kind: string; namespace: string | null; name: string } | null {
  for (const candidate of recordablePrefixes) {
    if (path.startsWith(candidate.prefix)) {
      const segments = path.slice(candidate.prefix.length).split('/').filter(Boolean).map(decodeURIComponent)
      if (segments.length === 2) {
        return { kind: candidate.kind, namespace: segments[0], name: segments[1] }
      }
      if (segments.length === 1) {
        return { kind: candidate.kind, namespace: null, name: segments[0] }
      }
      return null
    }
  }
  return null
}

// recordPath registers a completed navigation to an eligible detail target.
export function recordPath(path: string) {
  const parsed = kindForPath(path)
  if (!parsed) return
  entries = [
    { path, kind: parsed.kind, namespace: parsed.namespace, name: parsed.name, recordedAt: Date.now() },
    ...entries.filter((entry) => entry.path !== path),
  ].slice(0, recentLimit)
  emit()
}

export function recentTargets(): RecentTarget[] {
  const cutoff = Date.now() - recentExpirationMs
  entries = entries.filter((entry) => entry.recordedAt >= cutoff)
  return [...entries]
}

export function clearRecentTargets() {
  entries = []
  emit()
}

export function subscribeRecentTargets(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
