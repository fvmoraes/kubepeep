import { describe, expect, it } from 'vitest'
import { podHPA, podResourceUsage, resourceUsage } from './resourceMetrics'
import type { HorizontalPodAutoscaler, Pod, PodMetric, Workload } from '../../api/types'

const budget = { cpuRequestMillicores: 100, cpuLimitMillicores: 200, memoryRequestBytes: 1024, memoryLimitBytes: 2048 }

describe('resource bars', () => {
  it.each([[79, 'healthy'], [80, 'warning'], [90, 'warning'], [91, 'danger'], [120, 'danger']] as const)('classifies %d%% without losing over-capacity usage', (percent, tone) => {
    expect(resourceUsage(percent * 2, budget, 'cpu')).toMatchObject({ percent, tone, total: 200 })
  })
  it('uses HPA utilization against requests instead of limits', () => {
    expect(resourceUsage(65, budget, 'cpu', { resource: 'cpu', utilization: 60, averageValue: null })).toMatchObject({ total: 100, percent: 65, threshold: 60, tone: 'warning' })
  })
  it('keeps missing budgets and absent samples distinct from zero', () => {
    expect(resourceUsage(0, budget, 'cpu')).toMatchObject({ percent: 0, tone: 'healthy' })
    expect(resourceUsage(undefined, budget, 'cpu')).toMatchObject({ percent: null, tone: 'unknown' })
    expect(resourceUsage(10, undefined, 'cpu')).toMatchObject({ percent: null, tone: 'unknown' })
  })
  it('matches container HPA to that container usage and budget', () => {
    const target = { resource: 'cpu', container: 'api', utilization: 60, averageValue: null }
    const hpa = { resourceTargets: [target] } as HorizontalPodAutoscaler
    const pod = { resources: budget, containerResources: { api: { ...budget, cpuRequestMillicores: 50 } } }
    const sample = { cpuMillicores: 1000, containers: [{ name: 'api', cpuMillicores: 35, memoryBytes: 100 }] } as PodMetric
    const usage = podResourceUsage(pod, sample, 'cpu', hpa)
    expect(resourceUsage(usage.current, usage.budget, usage.resource, usage.target)).toMatchObject({ percent: 70, total: 50, tone: 'warning' })
  })
  it('resolves Deployment HPA through the real ReplicaSet owner', () => {
    const hpa = { namespace: 'ns', targetKind: 'Deployment', targetName: 'api' } as HorizontalPodAutoscaler
    const pod = { namespace: 'ns', owner: { kind: 'ReplicaSet', name: 'unrelated-hash' } } as Pod
    const replica = { namespace: 'ns', name: 'unrelated-hash', owner: { kind: 'Deployment', name: 'api' } } as Workload
    expect(podHPA(pod, { hpas: [hpa], replicas: [replica] })).toBe(hpa)
    expect(podHPA(pod, { hpas: [{ ...hpa, namespace: 'other' }], replicas: [replica] })).toBeUndefined()
  })
})
