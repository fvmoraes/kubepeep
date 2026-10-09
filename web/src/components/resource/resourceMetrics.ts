import { useAutoRefreshQueryOptions } from './AutoRefreshProvider'
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getDashboardMetrics, getHPAs, getWorkloads } from '../../api/client'
import type { CollectionResult, HorizontalPodAutoscaler, HPAResourceTarget, Pod, PodMetric, ResourceBudget, Workload } from '../../api/types'

// One shared observation per generation; a stale health badge cannot block it.
export function useResourceMetrics(generation?: string) {
  const autoRefresh = useAutoRefreshQueryOptions()
  return useQuery({ queryKey: ['resource-metrics', generation], queryFn: ({ signal }) => getDashboardMetrics(signal, generation), enabled: Boolean(generation), staleTime: 10_000, ...autoRefresh })
}

async function boundedCatalog<T>(fetchPage: (cursor?: string) => Promise<CollectionResult<T>>) {
  const items: T[] = []
  let cursor: string | undefined
  for (let page = 0; page < 10; page++) {
    const response = await fetchPage(cursor)
    items.push(...response.items)
    cursor = response.page.next ?? undefined
    if (!cursor) return items
  }
  return items
}

export function useResourceHPAs(generation?: string, namespace?: string) {
  const hpas = useQuery({ queryKey: ['resource-hpas', generation, namespace ?? ''], queryFn: ({ signal }) => boundedCatalog((cursor) => getHPAs({ limit: 100, namespaces: namespace ? [namespace] : undefined, continueToken: cursor }, signal, generation)), enabled: Boolean(generation), staleTime: 30_000, retry: false })
  const replicas = useQuery({ queryKey: ['resource-hpa-owners', generation, namespace ?? ''], queryFn: ({ signal }) => boundedCatalog((cursor) => getWorkloads({ limit: 100, kinds: ['replicasets'], namespaces: namespace ? [namespace] : undefined, continueToken: cursor }, signal, generation)), enabled: Boolean(generation && hpas.data?.some((hpa) => hpa.targetKind === 'Deployment')), staleTime: 30_000, retry: false })
  return useMemo(() => ({ hpas: hpas.data ?? [], replicas: replicas.data ?? [] }), [hpas.data, replicas.data])
}

export function podHPA(pod: Pod, catalog: { hpas: HorizontalPodAutoscaler[]; replicas: Workload[] }) {
  let owner = pod.owner
  if (owner?.kind === 'ReplicaSet') owner = catalog.replicas.find((item) => item.namespace === pod.namespace && item.name === owner?.name)?.owner ?? owner
  return catalog.hpas.find((hpa) => hpa.namespace === pod.namespace && hpa.targetKind === owner?.kind && hpa.targetName === owner?.name)
}

export function resourceUsage(current: number | undefined, budget: ResourceBudget | undefined, resource: 'cpu' | 'memory', target?: HPAResourceTarget) {
  const request = resource === 'cpu' ? budget?.cpuRequestMillicores : budget?.memoryRequestBytes
  const limit = resource === 'cpu' ? budget?.cpuLimitMillicores : budget?.memoryLimitBytes
  const total = target?.utilization ? request : target?.averageValue ?? limit ?? request
  const basis = target?.utilization ? 'request · HPA' : target?.averageValue ? 'HPA target' : limit ? 'limit' : 'request'
  const percent = current !== undefined && Number.isFinite(current) && current >= 0 && total && total > 0 ? current / total * 100 : null
  const threshold = target?.utilization ?? (target?.averageValue ? 100 : 80)
  const tone = percent === null ? 'unknown' : percent > 90 ? 'danger' : percent >= Math.min(threshold, 90) ? 'warning' : 'healthy'
  return { total, percent, threshold, tone, basis }
}

export function podResourceUsage(pod: Pick<Pod, 'resources' | 'containerResources'>, sample: PodMetric | undefined, resource: 'cpu' | 'memory', hpa?: HorizontalPodAutoscaler) {
  const targets = hpa?.resourceTargets?.filter((target) => target.resource === resource) ?? []
  const target = targets.find((item) => !item.container) ?? targets[0]
  const metric = target?.container ? sample?.containers.find((container) => container.name === target.container) : sample
  return { resource, target, current: resource === 'cpu' ? metric?.cpuMillicores : metric?.memoryBytes, budget: target?.container ? pod.containerResources?.[target.container] : pod.resources }
}
