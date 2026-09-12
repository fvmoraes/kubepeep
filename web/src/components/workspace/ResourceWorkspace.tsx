import { useEffect } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, X } from 'lucide-react'

import {
  getClusterRole,
  getClusterRoleBinding,
  getConfigMap,
  getCSIDriver,
  getCSINode,
  getCustomResourceDefinition,
  getConfigMapYAML,
  getEndpointsItem,
  getEndpointSlice,
  getEndpointSliceYAML,
  getEvents,
  getHPA,
  getIngress,
  getIngressClass,
  getIngressYAML,
  getLease,
  getLeaseYAML,
  getLimitRange,
  getMutatingWebhookConfiguration,
  getNamespaceObject,
  getNetworkPolicy,
  getNode,
  getNodeYAML,
  getPod,
  getPodYAML,
  getPersistentVolume,
  getPersistentVolumeClaim,
  getPersistentVolumeClaimYAML,
  getPersistentVolumeClaims,
  getPersistentVolumeYAML,
  getPriorityClass,
  getRole,
  getRoleBinding,
  getRuntimeClass,
  getSecret,
  getService,
  getServiceAccount,
  getServiceYAML,
  getStatus,
  getStorageClass,
  getStorageClassYAML,
  getValidatingWebhookConfiguration,
  getVolumeAttachment,
  getWorkload,
  getWorkloadYAML,
  getResourceQuota,
  getPDB,
} from '../../api/client'
import type {
  PodDetail,
  ResourceRef,
  WorkloadDetail,
  EventResource,
  PersistentVolumeClaim,
} from '../../api/types'
import { Badge, Button, StatusBadge } from '../ui'
import { FavoriteButton } from '../FavoriteButton'
import { PodActions, WorkloadActions } from '../ResourceActions'
import { YamlViewer } from '../YamlViewer'
import { TableLink } from '../resource/TableLink'
import { Facts } from '../resource/Facts'
import { errorMessage } from '../resource/errors'
import { dateTime } from '../resource/format'
import { eventBadgeVariant, statusBadgeVariant } from '../resource/status'
import { ResourceTabStrip, type ResourceTab } from '../resource/ResourceTabStrip'
import { resourceDetailPath, resourceKindLabel, workloadKindPath } from '../../navigation/paths'
import { useResourceWorkspace, type WorkspaceEntry } from './ResourceWorkspaceProvider'

const yamlCollections = new Set([
  'pods', 'services', 'ingresses', 'endpoint-slices', 'configmaps',
  'leases', 'persistent-volume-claims', 'persistent-volumes', 'storage-classes', 'nodes',
])

const kindToTarget: Record<string, { collection: string; kind?: string }> = {
  Pod: { collection: 'pods' },
  Deployment: { collection: 'workloads', kind: 'Deployment' },
  StatefulSet: { collection: 'workloads', kind: 'StatefulSet' },
  DaemonSet: { collection: 'workloads', kind: 'DaemonSet' },
  Job: { collection: 'workloads', kind: 'Job' },
  CronJob: { collection: 'workloads', kind: 'CronJob' },
  ReplicaSet: { collection: 'workloads', kind: 'ReplicaSet' },
  Service: { collection: 'services' },
  Ingress: { collection: 'ingresses' },
  EndpointSlice: { collection: 'endpoint-slices' },
  Endpoints: { collection: 'endpoints' },
  ConfigMap: { collection: 'configmaps' },
  Secret: { collection: 'secrets' },
  PersistentVolumeClaim: { collection: 'persistent-volume-claims' },
  PersistentVolume: { collection: 'persistent-volumes' },
  StorageClass: { collection: 'storage-classes' },
  Node: { collection: 'nodes' },
  ServiceAccount: { collection: 'service-accounts' },
  Role: { collection: 'roles' },
  RoleBinding: { collection: 'role-bindings' },
  ClusterRole: { collection: 'cluster-roles' },
  ClusterRoleBinding: { collection: 'cluster-role-bindings' },
}

function refToWorkspaceRef(ref: ResourceRef): { collection: string; kind: string | null; namespace: string | null; name: string } | null {
  if (!ref.name) return null
  const mapped = kindToTarget[ref.kind]
  if (mapped) {
    const target = { collection: mapped.collection, kind: mapped.kind ?? ref.kind, namespace: ref.namespace ?? null, name: ref.name }
    return resourceDetailPath(target) ? target : null
  }
  return null
}

export function tabsFor(entry: WorkspaceEntry): ResourceTab[] {
  const tabs: ResourceTab[] = [{ id: 'overview', label: 'Overview' }]
  if (entry.collection === 'pods') {
    tabs.push({ id: 'logs', label: 'Logs' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'actions', label: 'Actions' })
    return tabs
  }
  if (entry.collection === 'workloads') {
    switch (entry.kind) {
      case 'Deployment':
        tabs.push({ id: 'pods', label: 'Pods' }, { id: 'replicasets', label: 'ReplicaSets' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'rollout', label: 'Rollout' }, { id: 'actions', label: 'Actions' })
        break
      case 'StatefulSet':
        tabs.push({ id: 'pods', label: 'Pods' }, { id: 'pvcs', label: 'PVCs' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'actions', label: 'Actions' })
        break
      case 'DaemonSet':
        tabs.push({ id: 'pods', label: 'Pods' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'actions', label: 'Actions' })
        break
      case 'Job':
        tabs.push({ id: 'pods', label: 'Pods' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'actions', label: 'Actions' })
        break
      case 'CronJob':
        tabs.push({ id: 'jobs', label: 'Jobs' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'actions', label: 'Actions' })
        break
      case 'ReplicaSet':
        tabs.push({ id: 'pods', label: 'Pods' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'actions', label: 'Actions' })
        break
    }
    return tabs
  }
  if (entry.collection === 'services') {
    tabs.push({ id: 'endpoints', label: 'Endpoints' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' })
    return tabs
  }
  if (entry.collection === 'ingresses') {
    tabs.push({ id: 'rules', label: 'Rules' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' })
    return tabs
  }
  if (entry.collection === 'configmaps') {
    tabs.push({ id: 'data', label: 'Data' }, { id: 'yaml', label: 'YAML' })
    return tabs
  }
  if (entry.collection === 'nodes') {
    tabs.push({ id: 'conditions', label: 'Conditions' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' })
    return tabs
  }
  if (yamlCollections.has(entry.collection)) tabs.push({ id: 'yaml', label: 'YAML' })
  if (entry.namespace) tabs.push({ id: 'events', label: 'Events' })
  return tabs
}

type WorkspaceDetail =
  | { type: 'pod'; data: PodDetail }
  | { type: 'workload'; data: WorkloadDetail }
  | { type: 'other'; data: unknown; label: string }

async function fetchDetail(entry: WorkspaceEntry, signal: AbortSignal, generation: string | undefined): Promise<WorkspaceDetail> {
  const ns = entry.namespace
  const name = entry.name
  switch (entry.collection) {
    case 'pods':
      return { type: 'pod', data: await getPod(ns!, name, signal, generation) }
    case 'workloads':
      return { type: 'workload', data: await getWorkload(workloadKindPath(entry.kind!)!, ns!, name, signal, generation) }
    case 'services':
      return { type: 'other', data: await getService(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'ingresses':
      return { type: 'other', data: await getIngress(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'endpoint-slices':
      return { type: 'other', data: await getEndpointSlice(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'endpoints':
      return { type: 'other', data: await getEndpointsItem(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'network-policies':
      return { type: 'other', data: await getNetworkPolicy(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'configmaps':
      return { type: 'other', data: await getConfigMap(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'secrets':
      return { type: 'other', data: await getSecret(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'nodes':
      return { type: 'other', data: await getNode(name, signal, generation), label: resourceKindLabel(entry) }
    case 'leases':
      return { type: 'other', data: await getLease(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'persistent-volumes':
      return { type: 'other', data: await getPersistentVolume(name, signal, generation), label: resourceKindLabel(entry) }
    case 'persistent-volume-claims':
      return { type: 'other', data: await getPersistentVolumeClaim(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'storage-classes':
      return { type: 'other', data: await getStorageClass(name, signal, generation), label: resourceKindLabel(entry) }
    case 'csi-drivers':
      return { type: 'other', data: await getCSIDriver(name, signal, generation), label: resourceKindLabel(entry) }
    case 'csi-nodes':
      return { type: 'other', data: await getCSINode(name, signal, generation), label: resourceKindLabel(entry) }
    case 'volume-attachments':
      return { type: 'other', data: await getVolumeAttachment(name, signal, generation), label: resourceKindLabel(entry) }
    case 'namespaces':
      return { type: 'other', data: await getNamespaceObject(name, signal, generation), label: resourceKindLabel(entry) }
    case 'service-accounts':
      return { type: 'other', data: await getServiceAccount(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'resource-quotas':
      return { type: 'other', data: await getResourceQuota(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'limit-ranges':
      return { type: 'other', data: await getLimitRange(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'hpas':
      return { type: 'other', data: await getHPA(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'pdbs':
      return { type: 'other', data: await getPDB(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'roles':
      return { type: 'other', data: await getRole(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'role-bindings':
      return { type: 'other', data: await getRoleBinding(ns!, name, signal, generation), label: resourceKindLabel(entry) }
    case 'cluster-roles':
      return { type: 'other', data: await getClusterRole(name, signal, generation), label: resourceKindLabel(entry) }
    case 'cluster-role-bindings':
      return { type: 'other', data: await getClusterRoleBinding(name, signal, generation), label: resourceKindLabel(entry) }
    case 'customresourcedefinitions':
      return { type: 'other', data: await getCustomResourceDefinition(name, signal, generation), label: resourceKindLabel(entry) }
    case 'priority-classes':
      return { type: 'other', data: await getPriorityClass(name, signal, generation), label: resourceKindLabel(entry) }
    case 'runtime-classes':
      return { type: 'other', data: await getRuntimeClass(name, signal, generation), label: resourceKindLabel(entry) }
    case 'mutating-webhook-configurations':
      return { type: 'other', data: await getMutatingWebhookConfiguration(name, signal, generation), label: resourceKindLabel(entry) }
    case 'validating-webhook-configurations':
      return { type: 'other', data: await getValidatingWebhookConfiguration(name, signal, generation), label: resourceKindLabel(entry) }
    case 'ingress-classes':
      return { type: 'other', data: await getIngressClass(name, signal, generation), label: resourceKindLabel(entry) }
    default:
      throw new Error(`No detail view for ${entry.collection}.`)
  }
}

async function fetchYAML(entry: WorkspaceEntry, signal: AbortSignal): Promise<string> {
  const ns = entry.namespace
  const name = entry.name
  switch (entry.collection) {
    case 'pods':
      return getPodYAML(ns!, name, signal)
    case 'workloads':
      return getWorkloadYAML(workloadKindPath(entry.kind!)!, ns!, name, signal)
    case 'services':
      return getServiceYAML(ns!, name, signal)
    case 'ingresses':
      return getIngressYAML(ns!, name, signal)
    case 'endpoint-slices':
      return getEndpointSliceYAML(ns!, name, signal)
    case 'configmaps':
      return getConfigMapYAML(ns!, name, signal)
    case 'nodes':
      return getNodeYAML(name, signal)
    case 'leases':
      return getLeaseYAML(ns!, name, signal)
    case 'persistent-volumes':
      return getPersistentVolumeYAML(name, signal)
    case 'persistent-volume-claims':
      return getPersistentVolumeClaimYAML(ns!, name, signal)
    case 'storage-classes':
      return getStorageClassYAML(name, signal)
    default:
      throw new Error('YAML is not offered for this resource type.')
  }
}

function yamlDiffTarget(entry: WorkspaceEntry): { collection: string; namespace: string; name: string } | undefined {
  if (!entry.namespace) return undefined
  const collection = entry.collection === 'workloads' ? workloadKindPath(entry.kind!) : entry.collection
  return collection ? { collection, namespace: entry.namespace, name: entry.name } : undefined
}

function RelatedRefList({ refs, onOpen, emptyNote }: { refs: ResourceRef[]; onOpen: (ref: ResourceRef) => void; emptyNote: string }) {
  if (refs.length === 0) return <p className="m-0 text-sm text-kp-overlay-text" role="note">{emptyNote}</p>
  return (
    <ul className="m-0 grid list-none gap-1 p-0">
      {refs.map((ref) => (
        <li key={`${ref.kind}/${ref.namespace ?? ''}/${ref.name}`}>
          <TableLink aria-label={`Open ${ref.kind} ${ref.name}`} onClick={() => onOpen(ref)} primary={ref.name} secondary={ref.kind} disabledReason={refToWorkspaceRef(ref) ? undefined : 'Detail navigation is unavailable for this reference.'} />
        </li>
      ))}
    </ul>
  )
}

function ConditionsTable({ conditions }: { conditions: Array<{ type: string; status: string; reason: string | null; message: string | null; lastTransitionTime: string | null }> }) {
  if (conditions.length === 0) return <p className="m-0 text-sm text-kp-overlay-text" role="note">No conditions reported.</p>
  return (
    <div className="overflow-x-auto rounded-lg border border-kp-overlay-0">
      <table className="w-full border-collapse text-left text-sm">
        <thead><tr className="border-b border-kp-overlay-0 text-2xs uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-medium">Condition</th><th className="px-2.5 py-1.5 font-medium">Status</th><th className="px-2.5 py-1.5 font-medium">Since</th></tr></thead>
        <tbody>
          {conditions.map((condition) => (
            <tr key={condition.type} className="border-b border-kp-overlay-0/50 last:border-0">
              <td className="px-2.5 py-1.5 text-kp-text">{condition.type}</td>
              <td className="px-2.5 py-1.5"><StatusBadge variant={statusBadgeVariant(condition.status === 'True' ? 'Healthy' : condition.status === 'False' ? 'Degraded' : 'Unknown')}>{condition.status}</StatusBadge></td>
              <td className="px-2.5 py-1.5 text-kp-subtext">{condition.lastTransitionTime ? dateTime(condition.lastTransitionTime) : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function WorkspaceEvents({ entry, generation }: { entry: WorkspaceEntry; generation: string | undefined }) {
  const list = useQuery({
    queryKey: ['workspace-events', generation, entry.collection, entry.kind, entry.namespace, entry.name],
    queryFn: ({ signal }) => getEvents({ limit: 100, namespaces: entry.namespace ? [entry.namespace] : undefined, objectKind: entry.kind ?? undefined, search: entry.name, sort: 'timestamp', order: 'desc' }, signal, generation),
    enabled: Boolean(generation),
  })
  const events: EventResource[] = (list.data?.items ?? []).filter((item) => item.objectName === entry.name && (entry.kind ? item.objectKind === entry.kind : true))
  if (list.isPending) return <p className="text-sm text-kp-overlay-text" role="status">Loading events…</p>
  if (list.isError) return <p className="text-sm text-kp-red" role="alert">{errorMessage(list.error)}</p>
  if (events.length === 0) return <p className="m-0 text-sm text-kp-overlay-text" role="note">No Kubernetes events reference this resource in the current scope.</p>
  return (
    <div className="overflow-x-auto rounded-lg border border-kp-overlay-0 bg-kp-surface-0">
      <table className="w-full border-collapse text-left text-sm">
        <thead><tr className="border-b border-kp-overlay-0 text-2xs uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-medium">Time</th><th className="px-2.5 py-1.5 font-medium">Type</th><th className="px-2.5 py-1.5 font-medium">Reason</th><th className="px-2.5 py-1.5 font-medium">Message</th></tr></thead>
        <tbody>
          {events.map((event, index) => (
            <tr key={`${event.timestamp ?? index}/${event.reason}`} className="border-b border-kp-overlay-0/50 last:border-0">
              <td className="px-2.5 py-1.5 whitespace-nowrap text-kp-subtext">{event.timestamp ? dateTime(event.timestamp) : '—'}</td>
              <td className="px-2.5 py-1.5"><Badge variant={eventBadgeVariant(event.type)}>{event.type}</Badge></td>
              <td className="px-2.5 py-1.5 text-kp-text">{event.reason}</td>
              <td className="px-2.5 py-1.5 break-words text-kp-subtext">{event.message}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function WorkloadRelatedTab({ detail, kind, onOpen }: { detail: WorkloadDetail; kind: string; onOpen: (ref: ResourceRef) => void }) {
  const refs = detail.related.filter((ref) => ref.kind === kind)
  return <RelatedRefList refs={refs} onOpen={onOpen} emptyNote={`No ${kind} objects are related to this ${detail.kind} in the current scope.`} />
}

function StatefulSetPVCs({ entry, generation, onOpen }: { entry: WorkspaceEntry; generation: string | undefined; onOpen: (ref: ResourceRef) => void }) {
  const list = useQuery({
    queryKey: ['workspace-pvcs', generation, entry.namespace, entry.name],
    queryFn: ({ signal }) => getPersistentVolumeClaims({ namespaces: [entry.namespace!], limit: 100 }, signal, generation),
    enabled: Boolean(generation && entry.namespace),
  })
  const claims: PersistentVolumeClaim[] = (list.data?.items ?? []).filter((claim) => claim.name.includes(`-${entry.name}-`))
  if (list.isPending) return <p className="text-sm text-kp-overlay-text" role="status">Loading claims…</p>
  if (list.isError) return <p className="text-sm text-kp-red" role="alert">{errorMessage(list.error)}</p>
  if (claims.length === 0) return <p className="m-0 text-sm text-kp-overlay-text" role="note">No PersistentVolumeClaims match this StatefulSet's volume claim templates.</p>
  return (
    <ul className="m-0 grid list-none gap-1 p-0">
      {claims.map((claim) => (
        <li key={claim.name}>
          <TableLink
            aria-label={`Open PersistentVolumeClaim ${claim.name}`}
            onClick={() => onOpen({ kind: 'PersistentVolumeClaim', namespace: claim.namespace, name: claim.name })}
            primary={claim.name}
            secondary={`${claim.status} · ${claim.volumeName || 'no volume'}`}
          />
        </li>
      ))}
    </ul>
  )
}

function ServiceEndpoints({ entry, generation }: { entry: WorkspaceEntry; generation: string | undefined }) {
  const detail = useQuery({
    queryKey: ['workspace-service-endpoints', generation, entry.namespace, entry.name],
    queryFn: ({ signal }) => getEndpointsItem(entry.namespace!, entry.name, signal, generation),
    enabled: Boolean(generation && entry.namespace),
  })
  if (detail.isPending) return <p className="text-sm text-kp-overlay-text" role="status">Loading endpoints…</p>
  if (detail.isError) return <p className="text-sm text-kp-red" role="alert">{errorMessage(detail.error)}</p>
  const data = detail.data
  return (
    <Facts facts={[
      { label: 'Ready addresses', value: String(data.readyCount) },
      { label: 'Not ready', value: String(data.notReadyCount) },
      { label: 'Ports', value: data.ports.join(', ') || 'none' },
      { label: 'Truncated', value: data.truncated ? 'yes — address list exceeds the bounded window' : 'no' },
    ]} />
  )
}

function IngressRules({ paths, defaultBackend }: { paths: Array<{ host: string; path: string; pathType: string; backend: { serviceName: string; servicePort: { value: number | string } } }>; defaultBackend: { serviceName: string } | null }) {
  if (paths.length === 0 && !defaultBackend) return <p className="m-0 text-sm text-kp-overlay-text" role="note">No routing rules are declared.</p>
  return (
    <div className="overflow-x-auto rounded-lg border border-kp-overlay-0 bg-kp-surface-0">
      <table className="w-full border-collapse text-left text-sm">
        <thead><tr className="border-b border-kp-overlay-0 text-2xs uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-medium">Host</th><th className="px-2.5 py-1.5 font-medium">Path</th><th className="px-2.5 py-1.5 font-medium">Type</th><th className="px-2.5 py-1.5 font-medium">Backend</th></tr></thead>
        <tbody>
          {paths.map((path, index) => (
            <tr key={`${path.host}/${path.path}/${index}`} className="border-b border-kp-overlay-0/50 last:border-0">
              <td className="px-2.5 py-1.5 text-kp-text">{path.host}</td>
              <td className="px-2.5 py-1.5 mono text-kp-subtext">{path.path || '/'}</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">{path.pathType}</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">{path.backend.serviceName}:{path.backend.servicePort.value}</td>
            </tr>
          ))}
          {defaultBackend ? (
            <tr>
              <td className="px-2.5 py-1.5 text-kp-overlay-text">default</td>
              <td className="px-2.5 py-1.5 mono text-kp-subtext">/</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">defaultBackend</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">{defaultBackend.serviceName}</td>
            </tr>
          ) : null}
        </tbody>
      </table>
    </div>
  )
}

function ConfigMapData({ entries }: { entries: Array<{ key: string; encoding: string; value: string; truncated: boolean }> }) {
  if (entries.length === 0) return <p className="m-0 text-sm text-kp-overlay-text" role="note">This ConfigMap has no entries.</p>
  return (
    <div className="grid gap-2">
      {entries.map((entry) => (
        <details key={entry.key} className="rounded-lg border border-kp-overlay-0 bg-kp-surface-1">
          <summary className="cursor-pointer px-2.5 py-2 text-sm text-kp-sky">{entry.key} · {entry.encoding}{entry.truncated ? ' · truncated' : ''}</summary>
          <pre className="max-h-[320px] overflow-auto border-t border-kp-overlay-0 px-2.5 py-2 text-xs leading-relaxed text-kp-subtext break-words whitespace-pre-wrap">{entry.value}</pre>
        </details>
      ))}
    </div>
  )
}

function PodLogsLink({ detail }: { detail: PodDetail }) {
  const container = detail.containers[0]?.spec.name ?? ''
  const query = new URLSearchParams({ namespace: detail.metadata.namespace, pod: detail.metadata.name, container })
  return (
    <p className="m-0 text-sm text-kp-overlay-text" role="note">
      Open the dedicated log viewer with follow support:
      <a className="ml-1 text-kp-sky underline-offset-2 hover:underline" href={`/logs?${query.toString()}`}>Open Pod logs</a>
    </p>
  )
}

function overviewFacts(entry: WorkspaceEntry, detail: WorkspaceDetail): Array<{ label: string; value: string }> {
  if (detail.type === 'pod') {
    const data = detail.data
    return [
      { label: 'Status', value: data.summary.status },
      { label: 'Ready', value: `${data.summary.ready.current}/${data.summary.ready.desired}` },
      { label: 'Restarts', value: String(data.summary.restarts) },
      { label: 'Node', value: data.summary.node ?? '—' },
      { label: 'IP', value: data.summary.ip ?? '—' },
      { label: 'Owner', value: data.summary.owner ? `${data.summary.owner.kind}/${data.summary.owner.name}` : 'standalone' },
      { label: 'UID', value: data.metadata.uid },
    ]
  }
  if (detail.type === 'workload') {
    const data = detail.data
    return [
      { label: 'Status', value: data.status },
      { label: 'Ready', value: `${data.ready ?? '—'} / ${data.desired ?? '—'}` },
      { label: 'Available', value: String(data.available ?? '—') },
      { label: 'Updated', value: String(data.updated ?? '—') },
      { label: 'Containers', value: data.containers.map((value) => value.name).join(', ') || 'none' },
      { label: 'Images', value: data.containers.map((value) => value.image).join(', ') || 'none' },
      { label: 'Resource version', value: data.metadata.resourceVersion },
      { label: 'UID', value: data.metadata.uid },
    ]
  }
  const data = detail.data as Record<string, unknown>
  const facts: Array<{ label: string; value: string }> = []
  const metadata = data.metadata as { resourceVersion?: string; creationTimestamp?: string; uid?: string } | undefined
  if (metadata?.creationTimestamp) facts.push({ label: 'Created', value: dateTime(metadata.creationTimestamp) })
  if (metadata?.uid) facts.push({ label: 'UID', value: metadata.uid })
  if (metadata?.resourceVersion) facts.push({ label: 'Resource version', value: metadata.resourceVersion })
  const simple = (key: string, label: string) => {
    const value = data[key]
    if (value === undefined || value === null || value === '' || typeof value === 'object') return
    facts.push({ label, value: String(value) })
  }
  simple('status', 'Status')
  simple('type', 'Type')
  simple('holderName', 'Holder')
  simple('provisioner', 'Provisioner')
  simple('controller', 'Controller')
  simple('readyCount', 'Ready addresses')
  simple('notReadyCount', 'Not ready')
  simple('podSelector', 'Pod selector')
  simple('ruleCount', 'Rules')
  simple('roleRefKind', 'Role kind')
  simple('roleRefName', 'Role name')
  simple('targetKind', 'Target kind')
  simple('targetName', 'Target name')
  simple('minReplicas', 'Min replicas')
  simple('maxReplicas', 'Max replicas')
  simple('currentReplicas', 'Current replicas')
  simple('desiredReplicas', 'Desired replicas')
  simple('value', 'Priority')
  simple('handler', 'Handler')
  simple('nodeName', 'Node')
  simple('attacher', 'Attacher')
  simple('volumeName', 'Volume')
  simple('volumeMode', 'Volume mode')
  simple('reclaimPolicy', 'Reclaim policy')
  simple('storageClass', 'Storage class')
  simple('sessionAffinity', 'Session affinity')
  simple('externalTrafficPolicy', 'External traffic policy')
  simple('className', 'Class')
  simple('addressType', 'Address type')
  simple('phase', 'Phase')
  simple('attachRequired', 'Attach required')
  simple('kubeletVersion', 'Kubelet')
  simple('internalIP', 'Internal IP')
  simple('kind', 'Kind')
  simple('group', 'Group')
  simple('scope', 'Scope')
  simple('webhookCount', 'Webhooks')
  return facts
}

function secretNotice(label: string): string | null {
  if (label === 'Secret') {
    return 'Secret values, annotations, managed fields and YAML are intentionally unavailable.'
  }
  return null
}

export function ResourceWorkspaceOverlay() {
  const workspace = useResourceWorkspace()
  const entry = workspace.active
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  const selection = status.data?.selection ?? null
  const generation = selection?.generation

  const detail = useQuery({
    queryKey: ['workspace-detail', generation, entry?.collection, entry?.kind, entry?.namespace, entry?.name],
    queryFn: ({ signal }) => fetchDetail(entry as WorkspaceEntry, signal, generation),
    enabled: Boolean(workspace.open && entry && generation),
  })
  const yaml = useMutation({
    mutationFn: () => fetchYAML(entry as WorkspaceEntry, new AbortController().signal),
  })
  const entryKey = entry ? `${entry.collection}|${entry.kind ?? ''}|${entry.namespace ?? ''}|${entry.name}` : ''
  useEffect(() => { yaml.reset() }, [entryKey]) // eslint-disable-line react-hooks/exhaustive-deps -- reset cached YAML when the target changes

  if (!workspace.open || !entry) return null
  const activeEntry: WorkspaceEntry = entry

  const kindLabel = resourceKindLabel(activeEntry)
  const favoriteKind = activeEntry.collection === 'pods' ? 'pod' as const
    : activeEntry.collection === 'workloads' ? (({ Deployment: 'deployment', StatefulSet: 'statefulset', DaemonSet: 'daemonset', Job: 'job', CronJob: 'cronjob' } as const)[activeEntry.kind ?? ''] ?? null)
    : null
  const tabs = tabsFor(activeEntry)
  const activeTab = tabs.some((tab) => tab.id === activeEntry.tab) ? activeEntry.tab : 'overview'

  function openRef(ref: ResourceRef) {
    const target = refToWorkspaceRef(ref)
    if (target) workspace.openResource(target)
  }

  function detailFallback() {
    if (!selection) return <p className="text-sm text-kp-overlay-text" role="note">Select a Kubernetes context to inspect resources.</p>
    if (detail.isPending) return <p className="text-sm text-kp-overlay-text" role="status">Loading authorized detail…</p>
    if (detail.isError) return <p className="text-sm text-kp-red" role="alert">{errorMessage(detail.error)}</p>
    return null
  }

  function renderTab(tab: string) {
    if (tab === 'overview') {
      if (detail.isPending || detail.isError || !detail.data) return detailFallback()
      const data = detail.data
      const related = data.type === 'workload' ? (data.data.related ?? []) : []
      const owner = data.type === 'pod' && data.data.summary.owner
        ? [{ kind: data.data.summary.owner.kind, name: data.data.summary.owner.name, namespace: data.data.metadata.namespace } as ResourceRef]
        : []
      // Older backends encode an empty related-event slice as null.
      const podRefs = data.type === 'pod' ? (data.data.relatedEvents ?? []) : []
      return (
        <div className="grid content-start gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(260px,0.7fr)]">
          <div className="grid content-start gap-3 min-w-0">
            <Facts facts={overviewFacts(activeEntry, data)} />
            {data.type === 'workload' && data.data.conditions.length > 0 ? <ConditionsTable conditions={data.data.conditions} /> : null}
            {data.type === 'pod' ? <ConditionsTable conditions={data.data.conditions} /> : null}
            {data.type === 'other' ? <p className="m-0 text-xs text-kp-overlay-text" role="note">{secretNotice(data.label) ?? ''}</p> : null}
          </div>
          <div className="grid content-start gap-2 min-w-0">
            <h3 className="m-0 text-sm text-kp-text">Related resources</h3>
            {data.type === 'workload' ? <RelatedRefList refs={related} onOpen={openRef} emptyNote="No related objects are visible in the current scope." /> : null}
            {data.type === 'pod' ? <RelatedRefList refs={owner} onOpen={openRef} emptyNote="This Pod has no controller owner." /> : null}
            {data.type === 'pod' && podRefs.length > 0 ? (
              <>
                <h3 className="m-0 mt-2 text-sm text-kp-text">Related events</h3>
                <RelatedRefList refs={podRefs} onOpen={openRef} emptyNote="" />
              </>
            ) : null}
            {data.type === 'other' ? <p className="m-0 text-sm text-kp-overlay-text" role="note">Relationship navigation is available for workloads, Pods and Services.</p> : null}
          </div>
        </div>
      )
    }
    if (tab === 'yaml') {
      return (
        <YamlViewer
          value={yaml.data}
          pending={yaml.isPending}
          error={yaml.error}
          onLoad={() => yaml.mutate(undefined)}
          diffTarget={yamlDiffTarget(activeEntry)}
        />
      )
    }
    if (tab === 'events') return <WorkspaceEvents entry={activeEntry} generation={generation} />
    if (tab === 'logs') {
      if (detail.data?.type === 'pod') return <PodLogsLink detail={detail.data.data} />
      return detailFallback()
    }
    if (tab === 'actions') {
      if (!detail.data || detail.isPending || detail.isError || !selection) return detailFallback()
      if (detail.data.type === 'pod') return <PodActions detail={detail.data.data} selection={selection} />
      if (detail.data.type === 'workload') return <WorkloadActions detail={detail.data.data} selection={selection} />
      return null
    }
    if (tab === 'pods' || tab === 'replicasets' || tab === 'jobs') {
      if (detail.data?.type === 'workload') {
        const kind = tab === 'pods' ? 'Pod' : tab === 'replicasets' ? 'ReplicaSet' : 'Job'
        return <WorkloadRelatedTab detail={detail.data.data} kind={kind} onOpen={openRef} />
      }
      return detailFallback()
    }
    if (tab === 'pvcs') {
      return <StatefulSetPVCs entry={activeEntry} generation={generation} onOpen={openRef} />
    }
    if (tab === 'endpoints') return <ServiceEndpoints entry={activeEntry} generation={generation} />
    if (tab === 'rules') {
      if (detail.data?.type === 'other') {
        const data = detail.data.data as { summary?: { paths?: Array<{ host: string; path: string; pathType: string; backend: { serviceName: string; servicePort: { value: number | string } } }> }; defaultBackend?: { serviceName: string } | null }
        return <IngressRules paths={data.summary?.paths ?? []} defaultBackend={data.defaultBackend ?? null} />
      }
      return detailFallback()
    }
    if (tab === 'data') {
      if (detail.data?.type === 'other') {
        const data = detail.data.data as { entries?: Array<{ key: string; encoding: string; value: string; truncated: boolean }> }
        return <ConfigMapData entries={data.entries ?? []} />
      }
      return detailFallback()
    }
    if (tab === 'rollout') {
      if (detail.data?.type === 'workload') {
        const data = detail.data.data
        return (
          <div className="grid content-start gap-3">
            <Facts facts={[
              { label: 'Status', value: data.status },
              { label: 'Ready', value: `${data.ready ?? '—'} / ${data.desired ?? '—'}` },
              { label: 'Available', value: String(data.available ?? '—') },
              { label: 'Updated', value: String(data.updated ?? '—') },
              { label: 'Last restart stamp', value: data.restartAt ? dateTime(data.restartAt) : 'never restarted by KubePeep' },
            ]} />
            <ConditionsTable conditions={data.conditions} />
            <p className="m-0 text-xs text-kp-overlay-text" role="note">Progressing/Available conditions come from the controller; rollout completion is observed, not promised.</p>
          </div>
        )
      }
      return detailFallback()
    }
    if (tab === 'conditions') {
      if (detail.data?.type === 'other') {
        const data = detail.data.data as { conditions?: Array<{ type: string; status: string; reason: string | null; message: string | null; lastTransitionTime: string | null }> }
        return <ConditionsTable conditions={data.conditions ?? []} />
      }
      return detailFallback()
    }
    return null
  }

  return (
    <>
      <div className="workspace-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) workspace.close() }} aria-hidden="true" />
      <div className="workspace-panel" role="dialog" aria-modal="true" aria-label={`${kindLabel} ${activeEntry.name}`}>
        <header className="workspace-header">
          <Button variant="icon" className="workspace-nav-btn" onClick={workspace.back} disabled={!workspace.canBack} disabledReason="There is no previous resource in this workspace history." aria-label="Go to previous resource">
            <ChevronLeft size={16} aria-hidden="true" />
          </Button>
          <Button variant="icon" className="workspace-nav-btn" onClick={workspace.forward} disabled={!workspace.canForward} disabledReason="There is no next resource in this workspace history." aria-label="Go to next resource">
            <ChevronRight size={16} aria-hidden="true" />
          </Button>
          <Badge variant="accent">{kindLabel}</Badge>
          <strong className="mono min-w-0 truncate text-sm text-kp-text">{activeEntry.name}</strong>
          {activeEntry.namespace ? <span className="shrink-0 text-xs text-kp-overlay-text">ns: {activeEntry.namespace}</span> : <span className="shrink-0 text-xs text-kp-overlay-text">cluster-scoped</span>}
          <span className="flex-1" />
          {favoriteKind ? <FavoriteButton kind={favoriteKind} namespace={activeEntry.namespace ?? undefined} name={activeEntry.name} generation={generation} label={kindLabel} /> : null}
          <button type="button" onClick={workspace.close} className="h-7 w-7 shrink-0 grid place-items-center rounded-md text-kp-overlay-text hover:text-kp-text hover:bg-kp-surface-3" aria-label="Close resource workspace">
            <X size={16} aria-hidden="true" />
          </button>
        </header>
        <ResourceTabStrip tabs={tabs} active={activeTab} onChange={workspace.setTab} ariaLabel={`${kindLabel} workspace tabs`} panelId="workspace-tabpanel" />
        <div className="workspace-body" id="workspace-tabpanel" role="tabpanel">
          {renderTab(activeTab)}
        </div>
      </div>
    </>
  )
}
