import type { ResourceQuota, Workload } from '../../api/types'

/** Kubernetes decimal/binary quantities share a base unit within each quota key. */
export function quantityNumber(value?: string): number | undefined {
  if (!value) return undefined
  const match = value.match(/^([+-]?(?:\d+(?:\.\d*)?|\.\d+))([eE][+-]?\d+|[numkKMGTPE]|[KMGTPE]i)?$/)
  if (!match) return undefined
  const suffix = match[2] ?? ''
  const powers: Record<string, number> = { n: 1e-9, u: 1e-6, m: 1e-3, k: 1e3, K: 1e3, M: 1e6, G: 1e9, T: 1e12, P: 1e15, E: 1e18, Ki: 1024, Mi: 1024 ** 2, Gi: 1024 ** 3, Ti: 1024 ** 4, Pi: 1024 ** 5, Ei: 1024 ** 6 }
  const number = Number(match[1]) * (suffix ? powers[suffix] ?? 10 ** Number(suffix.slice(1)) : 1)
  return Number.isFinite(number) && number >= 0 ? number : undefined
}

export function QuantityUsage({ label, current, total, text, completion = false, failed = false }: { label: string; current?: number | null; total?: number | null; text?: string; completion?: boolean; failed?: boolean }) {
  const percent = current != null && total != null && Number.isFinite(current) && Number.isFinite(total) && current >= 0 && total > 0 ? current / total * 100 : null
  const tone = percent === null ? 'unknown' : failed ? 'danger' : completion ? percent >= 100 ? 'healthy' : 'warning' : percent > 90 ? 'danger' : percent >= 80 ? 'warning' : 'healthy'
  const caption = `${text ?? `${current ?? '—'} / ${total ?? '—'}`} · ${percent === null ? 'No ratio' : `${percent.toFixed(1)}%`}`
  return <div className="resource-usage" data-tone={tone} title={`${label}: ${caption}${completion ? '; green when complete' : '; yellow at 80%, red above 90%'}`}>
    <span className="resource-usage-value">{caption}</span>
    <div className="resource-usage-track" role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent === null ? undefined : Math.min(100, Math.round(percent))} aria-valuetext={caption}><span style={{ width: `${percent === null ? 0 : Math.min(100, percent)}%` }} /></div>
  </div>
}

export function QuotaUsage({ quota }: { quota: Pick<ResourceQuota, 'hard' | 'used'> }) {
  const entries = Object.entries(quota.hard ?? {})
  return entries.length ? <div className="grid gap-2">{entries.map(([key, hard]) => <div key={key}><small className="text-kp-overlay-text">{key}</small><QuantityUsage label={`Quota ${key}`} current={quantityNumber(quota.used?.[key])} total={quantityNumber(hard)} text={`${quota.used?.[key] ?? '—'} / ${hard}`} /></div>)}</div> : <span>No quota limits</span>
}

export function WorkloadProgress({ workload }: { workload: Pick<Workload, 'kind' | 'ready' | 'desired' | 'available' | 'status'> }) {
  return <QuantityUsage label={workload.kind === 'Job' ? 'Job completions' : 'Ready replicas'} current={workload.kind === 'Job' ? workload.available : workload.ready} total={workload.desired} completion failed={workload.status === 'Failed' || workload.status === 'Degraded'} />
}
