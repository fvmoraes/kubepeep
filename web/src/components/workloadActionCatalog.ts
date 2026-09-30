import type { WorkloadDetail } from '../api/types'

export type WorkloadActionID = 'restart' | 'scale' | 'delete' | 'suspend' | 'runNow'

export interface WorkloadActionDefinition {
  id: WorkloadActionID
  capabilityID: string
}

// Canonical UI/backend capability inventory. Tests and release documentation
// use the same closed catalog so a new kind or action cannot silently escape
// the RBAC matrix.
export const workloadActionCatalog: Record<WorkloadDetail['kind'], readonly WorkloadActionDefinition[]> = {
  Deployment: [
    { id: 'restart', capabilityID: 'deployments.restart' },
    { id: 'scale', capabilityID: 'deployments.scale' },
    { id: 'delete', capabilityID: 'deployments.delete' },
  ],
  StatefulSet: [
    { id: 'restart', capabilityID: 'statefulsets.restart' },
    { id: 'scale', capabilityID: 'statefulsets.scale' },
    { id: 'delete', capabilityID: 'statefulsets.delete' },
  ],
  DaemonSet: [
    { id: 'restart', capabilityID: 'daemonsets.restart' },
    { id: 'delete', capabilityID: 'daemonsets.delete' },
  ],
  Job: [{ id: 'delete', capabilityID: 'jobs.delete' }],
  CronJob: [
    { id: 'suspend', capabilityID: 'cronjobs.suspend' },
    { id: 'runNow', capabilityID: 'cronjobs.runnow' },
    { id: 'delete', capabilityID: 'cronjobs.delete' },
  ],
  ReplicaSet: [{ id: 'delete', capabilityID: 'replicasets.delete' }],
}
