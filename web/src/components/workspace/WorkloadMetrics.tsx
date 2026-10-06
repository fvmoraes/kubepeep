import { LoadingState } from '../ui/LoadingState'
import type { PodDetail, WorkloadDetail } from '../../api/types'
import { ResourceUsage } from '../resource/ResourceUsage'
import { podHPA, podResourceUsage, useResourceHPAs, useResourceMetrics } from '../resource/resourceMetrics'
import { errorMessage } from '../resource/errors'

export function PodMetrics({ detail, generation }: { detail: PodDetail; generation?: string }) {
  const metrics = useResourceMetrics(generation)
  const catalog = useResourceHPAs(generation, detail.metadata.namespace)
  const hpa = podHPA(detail.summary, catalog)
  if (metrics.isPending) return <LoadingState label="Loading Pod metrics…" />
  if (metrics.isError) return <p role="alert" className="text-content text-kp-yellow">{errorMessage(metrics.error)}</p>
  const pod = metrics.data.block.value.pods.find((item) => item.namespace === detail.metadata.namespace && item.pod === detail.metadata.name)
  const containers = [...detail.containers, ...detail.initContainers, ...detail.ephemeralContainers]
  return <section className="grid min-w-0 gap-4" aria-label="Pod resource usage">
    {!pod ? <p role="status" className="text-content text-kp-yellow">No current sample. Check Metrics Server and permission to read metrics.k8s.io Pods.</p> : null}
    <div className="grid gap-4 sm:grid-cols-2">{(['cpu', 'memory'] as const).map((resource) => <div key={resource}><h3 className="mb-2 text-heading">{resource === 'cpu' ? 'CPU' : 'Memory'}</h3><ResourceUsage {...podResourceUsage(detail.summary, pod, resource, hpa)} /></div>)}</div>
    <p className="m-0 text-content text-kp-overlay-text">{hpa ? `HPA: ${hpa.name}. Utilization targets use resource requests.` : 'Warning threshold: 80%. Uses limits when set, otherwise requests.'} Red above 90%. Missing budgets have no percentage.</p>
    {pod ? <div className="overflow-x-auto"><table className="w-full text-left text-content"><thead><tr><th className="p-2">Container</th><th className="p-2">CPU</th><th className="p-2">Memory</th></tr></thead><tbody>{pod.containers.map((sample) => <tr key={sample.name} className="border-t border-kp-divider"><th scope="row" className="p-2 font-bold">{sample.name}</th>{(['cpu', 'memory'] as const).map((resource) => <td key={resource} className="p-2"><ResourceUsage resource={resource} current={resource === 'cpu' ? sample.cpuMillicores : sample.memoryBytes} budget={containers.find((container) => container.spec.name === sample.name)?.spec.resources} target={hpa?.resourceTargets?.find((target) => target.resource === resource && (!target.container || target.container === sample.name))} /></td>)}</tr>)}</tbody></table></div> : null}
  </section>
}

export function WorkloadMetrics({ detail, generation }: { detail: WorkloadDetail; generation?: string }) {
  const metrics = useResourceMetrics(generation)
  const catalog = useResourceHPAs(generation, detail.metadata.namespace)
  const hpa = catalog.hpas.find((item) => item.namespace === detail.metadata.namespace && item.targetKind === detail.kind && item.targetName === detail.metadata.name)
  const pods = detail.related.filter((ref) => ref.kind === 'Pod' && ref.namespace === detail.metadata.namespace)
  const names = new Set(pods.map((ref) => ref.name))
  const samples = metrics.data?.block.value.pods.filter((sample) => sample.namespace === detail.metadata.namespace && names.has(sample.pod)) ?? []
  const complete = samples.length > 0 && samples.length === pods.length && pods.length >= (detail.ready ?? 0)
  const total = (value: number | null) => value === null ? null : value * samples.length
  const budget = detail.resources ? { cpuRequestMillicores: total(detail.resources.cpuRequestMillicores), cpuLimitMillicores: total(detail.resources.cpuLimitMillicores), memoryRequestBytes: total(detail.resources.memoryRequestBytes), memoryLimitBytes: total(detail.resources.memoryLimitBytes) } : undefined
  return <section className="grid gap-4" aria-label="Workload resource usage">
    <p role="status" className="m-0 text-content text-kp-overlay-text">{metrics.isPending ? 'Loading metrics…' : `${samples.length} of ${pods.length} related Pods sampled.`} Totals use the current template requests and limits across sampled replicas.</p>
    {metrics.isError ? <p role="alert" className="text-content text-kp-yellow">{errorMessage(metrics.error)}</p> : null}
    <div className="grid gap-4 sm:grid-cols-2">{(['cpu', 'memory'] as const).map((resource) => {
      const targets = hpa?.resourceTargets?.filter((item) => item.resource === resource) ?? []
      const target = targets.find((item) => !item.container) ?? targets[0]
      const containerBudget = target?.container ? detail.containers.find((item) => item.name === target.container)?.resources : undefined
      const selectedBudget = target?.container ? containerBudget && { cpuRequestMillicores: total(containerBudget.cpuRequestMillicores), cpuLimitMillicores: total(containerBudget.cpuLimitMillicores), memoryRequestBytes: total(containerBudget.memoryRequestBytes), memoryLimitBytes: total(containerBudget.memoryLimitBytes) } : budget
      const observations = samples.map((sample) => target?.container ? sample.containers.find((item) => item.name === target.container) : sample)
      const current = complete && observations.every((sample) => sample !== undefined) ? observations.reduce((sum, sample) => sum + (resource === 'cpu' ? sample!.cpuMillicores : sample!.memoryBytes), 0) : undefined
      return <div key={resource}><h3 className="mb-2 text-heading">{resource === 'cpu' ? 'CPU' : 'Memory'}</h3><ResourceUsage resource={resource} current={current} budget={selectedBudget} target={target?.averageValue ? { ...target, averageValue: target.averageValue * samples.length } : target} /></div>
    })}</div>
  </section>
}
