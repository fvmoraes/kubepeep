import { QuantityUsage, QuotaUsage, WorkloadProgress } from '../resource/QuantityUsage'
import { helmDriver, isHelmCollection } from '../../navigation/helm'
import { getHelmRelease } from '../../api/client'
import type { HelmRelease } from '../../api/types'
import { useAutoRefreshQueryOptions } from '../resource/AutoRefreshProvider'
import { LoadingState } from '../ui/LoadingState'
import { DataEntries, SecretData, WorkloadEnvironment } from './WorkloadData'
import { relatedResources } from './relatedResources'
import { PodMetrics, WorkloadMetrics } from './WorkloadMetrics'
import { lazy, Suspense, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, X } from 'lucide-react'

import {
  createIdempotencyKey,
  createServicePortForward,
  getClusterRole,
  getClusterRoleBinding,
  getConfigMap,
  getCSIDriver,
  getCSINode,
  getCustomResourceDefinition,
  getEndpointsItem,
  getEndpointSlice,
  getEvents,
  getPermissions,
  getHPA,
  getIngress,
  getIngressClass,
  getInvestigation,
  getLease,
  getLimitRange,
  getMutatingWebhookConfiguration,
  getNamespaceObject,
  getNetworkPolicy,
  getNode,
  getPod,
  getPersistentVolume,
  getPersistentVolumeClaim,
  getPersistentVolumeClaims,
  getPriorityClass,
  getRole,
  getRoleBinding,
  getRuntimeClass,
  getSecret,
  getService,
  getServiceAccount,
  getStatus,
  getStorageClass,
  getValidatingWebhookConfiguration,
  getVolumeAttachment,
  getWorkload,
  getResourceQuota,
  getPDB,
} from '../../api/client'
import type {
  PodDetail,
  ResourceRef,
  WorkloadDetail,
  EventResource,
  PersistentVolumeClaim,
  IngressDetail,
  ServiceDetail,
  SelectionSummary,
  Investigation,
  ResourceQuota, HorizontalPodAutoscaler, PodDisruptionBudget,
} from '../../api/types'
import { Badge, Button, Input, Select, StatusBadge } from '../ui'
import { csrfForGeneration } from '../../actions/csrf'
import { useToast } from '../ui/Toast'
import { FavoriteButton } from '../FavoriteButton'
import { TableLink } from '../resource/TableLink'
import { Facts } from '../resource/Facts'
import { errorMessage } from '../resource/errors'
import { dateTime } from '../resource/format'
import { eventBadgeVariant, statusBadgeVariant } from '../resource/status'
import { ResourceTabStrip, type ResourceTab } from '../resource/ResourceTabStrip'
import { resourceRefForKind, resourceKindLabel, workloadKindPath } from '../../navigation/paths'
import { useResourceWorkspace, type WorkspaceEntry } from './ResourceWorkspaceProvider'

const PodLogsPanel = lazy(() => import('./PodLogsPanel').then((module) => ({ default: module.PodLogsPanel })))
const WorkloadLogsPanel = lazy(() => import('./WorkloadLogsPanel').then((module) => ({ default: module.WorkloadLogsPanel })))
const ResourceYamlEditor = lazy(() => import('./ResourceYamlEditor').then((module) => ({ default: module.ResourceYamlEditor })))
const PodActions = lazy(() => import('../ResourceActions').then((module) => ({ default: module.PodActions })))
const WorkloadActions = lazy(() => import('../ResourceActions').then((module) => ({ default: module.WorkloadActions })))
const HelmDocumentEditor = lazy(() => import('./HelmDetails').then((module) => ({ default: module.HelmDocumentEditor })))
const HelmHistory = lazy(() => import('./HelmDetails').then((module) => ({ default: module.HelmHistory })))

const refToWorkspaceRef = resourceRefForKind

export function tabsFor(entry: WorkspaceEntry): ResourceTab[] {
  if (parseDynamicCollection(entry.collection)) return [{ id: 'overview', label: 'Overview' }, { id: 'yaml', label: 'YAML' }]
  if (isHelmCollection(entry.collection)) return [{id:'overview',label:'Overview'},{id:'values',label:'Values'},{id:'manifest',label:'Manifest'},{id:'history',label:'History'}]
  const tabs: ResourceTab[] = [{ id: 'overview', label: 'Overview' }]
  if (entry.collection === 'pods') {
		tabs.push({ id: 'investigation', label: 'Investigation' }, { id: 'logs', label: 'Logs' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'metrics', label: 'Metrics' }, { id: 'containers', label: 'Containers' }, { id: 'data', label: 'Data / Env' }, { id: 'actions', label: 'Actions' })
    return tabs
  }
  if (entry.collection === 'workloads') {
		tabs.push({ id: 'investigation', label: 'Investigation' }, { id: 'logs', label: 'Logs' }, { id: 'metrics', label: 'Metrics' }, { id: 'data', label: 'Data / Env' })
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
    tabs.push({ id: 'endpoints', label: 'Endpoints' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'actions', label: 'Actions' })
    return tabs
  }
  if (entry.collection === 'ingresses') {
    tabs.push({ id: 'rules', label: 'Rules' }, { id: 'backends', label: 'Backends' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' }, { id: 'actions', label: 'Actions' })
    return tabs
  }
  if (entry.collection === 'secrets') { tabs.push({ id: 'data', label: 'Data' }, { id: 'yaml', label: 'YAML' }); return tabs }
  if (entry.collection === 'configmaps') {
    tabs.push({ id: 'data', label: 'Data' }, { id: 'yaml', label: 'YAML' })
    return tabs
  }
  if (entry.collection === 'nodes') {
    tabs.push({ id: 'conditions', label: 'Conditions' }, { id: 'yaml', label: 'YAML' }, { id: 'events', label: 'Events' })
    return tabs
  }
  tabs.push({ id: 'yaml', label: 'YAML' })
  if (entry.namespace) tabs.push({ id: 'events', label: 'Events' })
  return tabs
}

function InvestigationView({ value, onOpen }: { value: Investigation; onOpen: (ref: ResourceRef) => void }) {
	const groups: Array<[string, Investigation[keyof Pick<Investigation, 'ownerChain' | 'pods' | 'services' | 'endpointSlices' | 'configMaps' | 'pvcs' | 'events'>]]> = [
		['Owner chain', value.ownerChain], ['Pods', value.pods], ['Services', value.services], ['EndpointSlices', value.endpointSlices], ['ConfigMaps', value.configMaps], ['PersistentVolumeClaims', value.pvcs], ['Events', value.events],
	]
	const incomplete = value.coverage.filter((item) => !item.complete)
	return <div className="grid gap-3">
		{incomplete.length > 0 ? <p className="rounded-md border border-kp-yellow-border bg-kp-yellow-bg px-3 py-2 text-content text-kp-yellow" role="status">Partial local coverage: {incomplete.map((item) => item.topic).join(', ')}. Absence below does not prove absence in the cluster.</p> : null}
		<div className="grid gap-3 sm:grid-cols-2">
			{groups.map(([label, resources]) => <section key={label} className="rounded-lg border border-kp-overlay-0 bg-kp-surface-1 p-3"><h3 className="text-heading text-kp-text">{label}</h3>{resources.length === 0 ? <p className="mt-1 text-content text-kp-overlay-text">No item in the loaded local index.</p> : <ul className="mt-2 grid list-none gap-1 p-0">{resources.map((resource) => {
				const navigable = resource.kind !== 'Event'
				const unavailableReason = navigable ? undefined : 'Event navigation is unavailable; inspect the event in the Events page.'
				return <li key={`${resource.kind}/${resource.namespace ?? ''}/${resource.name}`}><button type="button" disabled={!navigable} title={unavailableReason} className="control-row w-full px-2 py-1.5 text-left text-content text-kp-subtext enabled:hover:bg-kp-surface-2 enabled:hover:text-kp-text disabled:cursor-default" onClick={() => onOpen({ apiGroup: resource.apiGroup, kind: resource.kind, namespace: resource.namespace, name: resource.name })}><strong className="block text-kp-text">{resource.kind} · {resource.name}</strong><span>{resource.namespace ?? 'cluster'}{resource.status ? ` · ${resource.status}` : ''}</span>{unavailableReason ? <span className="mt-1 block text-kp-overlay-text">{unavailableReason}</span> : null}</button></li>
			})}</ul>}</section>)}
		</div>
	</div>
}

type WorkspaceDetail =
  | { type: 'pod'; data: PodDetail }
  | { type: 'workload'; data: WorkloadDetail }
  | { type: 'other'; data: unknown; label: string }

async function fetchDetail(entry: WorkspaceEntry, signal: AbortSignal, generation: string | undefined): Promise<WorkspaceDetail> {
 const dynamic = parseDynamicCollection(entry.collection)
 if (dynamic) return { type: 'other', data: await getDynamicResource(dynamic, entry.namespace, entry.name, signal, generation), label: entry.kind || dynamic.resource }
 if (isHelmCollection(entry.collection)) return {type:'other',data:await getHelmRelease(helmDriver(entry.collection),entry.namespace!,entry.name,signal,generation),label:'HelmRelease'}
 if (isGatewayCollection(entry.collection)) return { type: 'other', data: await getGatewayResource(entry.collection,entry.namespace,entry.name,signal,generation),label:resourceKindLabel(entry) }
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


function RelatedRefList({ refs, onOpen, emptyNote }: { refs: ResourceRef[]; onOpen: (ref: ResourceRef) => void; emptyNote: string }) {
  if (refs.length === 0) return <p className="m-0 text-content text-kp-overlay-text" role="note">{emptyNote}</p>
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
  if (conditions.length === 0) return <p className="m-0 text-content text-kp-overlay-text" role="note">No conditions reported.</p>
  return (
    <div className="overflow-x-auto rounded-lg border border-kp-overlay-0">
      <table className="w-full border-collapse text-left text-content">
        <thead><tr className="border-b border-kp-overlay-0 text-column uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-bold">Condition</th><th className="px-2.5 py-1.5 font-bold">Status</th><th className="px-2.5 py-1.5 font-bold">Since</th></tr></thead>
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
  if (list.isPending) return <LoadingState label="Loading events…" />
  if (list.isError) return <p className="text-content text-kp-red" role="alert">{errorMessage(list.error)}</p>
  if (events.length === 0) return <p className="m-0 text-content text-kp-overlay-text" role="note">No Kubernetes events reference this resource in the current scope.</p>
  return (
    <div className="overflow-x-auto rounded-lg border border-kp-overlay-0 bg-kp-surface-0">
      <table className="w-full border-collapse text-left text-content">
        <thead><tr className="border-b border-kp-overlay-0 text-column uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-bold">Time</th><th className="px-2.5 py-1.5 font-bold">Type</th><th className="px-2.5 py-1.5 font-bold">Reason</th><th className="px-2.5 py-1.5 font-bold">Message</th></tr></thead>
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
  if (list.isPending) return <LoadingState label="Loading claims…" />
  if (list.isError) return <p className="text-content text-kp-red" role="alert">{errorMessage(list.error)}</p>
  if (claims.length === 0) return <p className="m-0 text-content text-kp-overlay-text" role="note">No PersistentVolumeClaims match this StatefulSet's volume claim templates.</p>
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
  if (detail.isPending) return <LoadingState label="Loading endpoints…" />
  if (detail.isError) return <div><p className="text-content text-kp-red" role="alert">{errorMessage(detail.error)}</p><Button variant="secondary" disabled={detail.isFetching} onClick={() => void detail.refetch()}>Retry loading detail</Button></div>
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
  if (paths.length === 0 && !defaultBackend) return <p className="m-0 text-content text-kp-overlay-text" role="note">No routing rules are declared.</p>
  return (
    <div className="overflow-x-auto rounded-lg border border-kp-overlay-0 bg-kp-surface-0">
      <table className="w-full border-collapse text-left text-content">
        <thead><tr className="border-b border-kp-overlay-0 text-column uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-bold">Host</th><th className="px-2.5 py-1.5 font-bold">Path</th><th className="px-2.5 py-1.5 font-bold">Type</th><th className="px-2.5 py-1.5 font-bold">Backend</th></tr></thead>
        <tbody>
          {paths.map((path, index) => (
            <tr key={`${path.host}/${path.path}/${index}`} className="border-b border-kp-overlay-0/50 last:border-0">
              <td className="px-2.5 py-1.5 text-kp-text">{path.host}</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">{path.path || '/'}</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">{path.pathType}</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">{path.backend.serviceName}:{path.backend.servicePort.value}</td>
            </tr>
          ))}
          {defaultBackend ? (
            <tr>
              <td className="px-2.5 py-1.5 text-kp-overlay-text">default</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">/</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">defaultBackend</td>
              <td className="px-2.5 py-1.5 text-kp-subtext">{defaultBackend.serviceName}</td>
            </tr>
          ) : null}
        </tbody>
      </table>
    </div>
  )
}


function PodContainers({ detail }: { detail: PodDetail }) {
  const containers = [...detail.initContainers, ...detail.containers, ...detail.ephemeralContainers]
  if (containers.length === 0) return <p className="m-0 text-content text-kp-overlay-text" role="note">No containers are declared.</p>
  return (
    <div className="overflow-x-auto rounded-lg border border-kp-overlay-0 bg-kp-surface-0">
      <table className="w-full border-collapse text-left text-content">
        <thead><tr className="border-b border-kp-overlay-0 text-column uppercase tracking-wider text-kp-overlay-text"><th className="px-2.5 py-1.5 font-bold">Name</th><th className="px-2.5 py-1.5 font-bold">Type</th><th className="px-2.5 py-1.5 font-bold">Image</th><th className="px-2.5 py-1.5 font-bold">State</th><th className="px-2.5 py-1.5 font-bold">Ready</th><th className="px-2.5 py-1.5 font-bold">Restarts</th></tr></thead>
        <tbody>{containers.map((container) => <tr key={`${container.type}/${container.spec.name}`} className="border-b border-kp-overlay-0/50 last:border-0"><td className="px-2.5 py-1.5 text-kp-text">{container.spec.name}</td><td className="px-2.5 py-1.5 text-kp-subtext">{container.type}</td><td className="max-w-[28rem] truncate px-2.5 py-1.5 text-kp-subtext" title={container.spec.image}>{container.spec.image}</td><td className="px-2.5 py-1.5"><StatusBadge variant={statusBadgeVariant(container.state)}>{container.reason ?? container.state}</StatusBadge></td><td className="px-2.5 py-1.5 text-kp-subtext">{container.ready === null ? '—' : container.ready ? 'yes' : 'no'}</td><td className="px-2.5 py-1.5 tabular-nums text-kp-subtext">{container.restartCount}</td></tr>)}</tbody>
      </table>
    </div>
  )
}

function IngressBackends({ detail, onOpen }: { detail: IngressDetail; onOpen: (ref: ResourceRef) => void }) {
  const names = Array.from(new Set([...detail.summary.paths.map((path) => path.backend.serviceName), ...(detail.defaultBackend ? [detail.defaultBackend.serviceName] : [])]))
  return <RelatedRefList refs={names.map((name) => ({ kind: 'Service', namespace: detail.metadata.namespace, name }))} onOpen={onOpen} emptyNote="No Service backends are declared." />
}

function CopyAction({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false)
  const [failed, setFailed] = useState(false)
  return <span className="inline-flex items-center gap-2"><Button variant="secondary" disabled={!value} onClick={async () => {
    try {
      await navigator.clipboard.writeText(value)
      setFailed(false)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1_500)
    } catch {
      setCopied(false)
      setFailed(true)
    }
  }}>{copied ? 'Copied' : label}</Button>{failed ? <span className="text-content text-kp-red" role="alert">Copy failed</span> : null}</span>
}

function NetworkActions({ entry, detail, selection, setTab }: { entry: WorkspaceEntry; detail: ServiceDetail | IngressDetail; selection: SelectionSummary; setTab: (tab: string) => void }) {
  if (entry.collection === 'services') {
    return <ServiceActions entry={entry} detail={detail as ServiceDetail} selection={selection} setTab={setTab} />
  }
  const ingress = detail as IngressDetail
  const host = ingress.summary.hosts[0] ?? ''
  const url = host ? `${ingress.summary.tlsHosts.includes(host) ? 'https' : 'http'}://${host}` : ''
  return <div className="flex flex-wrap items-center gap-2">{url ? <a className="control inline-flex items-center justify-center border border-kp-blue-border bg-kp-blue-bg px-2 text-content font-normal text-kp-sky transition-colors duration-100 hover:border-kp-sky hover:bg-kp-surface-3 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-kp-mauve" href={url} target="_blank" rel="noreferrer">Open URL</a> : <Button disabled disabledReason="This Ingress does not declare a host.">Open URL</Button>}<CopyAction label="Copy Host" value={host} /></div>
}

function ServiceActions({ entry, detail, selection, setTab }: { entry: WorkspaceEntry; detail: ServiceDetail; selection: SelectionSummary; setTab: (tab: string) => void }) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const tcpPorts = detail.summary.ports.filter((port) => port.protocol === 'TCP')
  const [servicePort, setServicePort] = useState(tcpPorts[0]?.port ?? 0)
  const [localPort, setLocalPort] = useState('')
  const prerequisites = useQuery({
    queryKey: ['service-port-forward-permissions', selection.generation, entry.namespace, entry.name],
    queryFn: async ({ signal }) => {
      const [service, pods] = await Promise.all([
        getPermissions({ namespaces: [entry.namespace!], capabilityIds: ['services.get'], resourceNames: [entry.name] }, signal, selection.generation),
        getPermissions({ namespaces: [entry.namespace!], capabilityIds: ['pods.list', 'pods.portforward.create'] }, signal, selection.generation),
      ])
      return [...service.decisions, ...pods.decisions]
    },
    staleTime: 15_000,
  })
  const permissionAllowed = (capabilityId: string) => prerequisites.data?.some((item) => item.capabilityId === capabilityId && item.decision === 'allowed') === true
  const local = localPort === '' ? null : Number(localPort)
  const localValid = local === null || (Number.isInteger(local) && local >= 1024 && local <= 65_535)
  const hasSelector = Object.keys(detail.summary.selector ?? {}).length > 0
  const canStart = hasSelector && servicePort > 0 && localValid && ['services.get', 'pods.list', 'pods.portforward.create'].every(permissionAllowed)
  const portForward = useMutation({
    mutationFn: async () => {
      const csrfToken = await csrfForGeneration(selection.generation)
      return createServicePortForward(entry.namespace!, entry.name, {
        remotePort: servicePort,
        localPort: local,
        confirmed: true,
        action: 'portForward',
        consequenceCode: 'EXPOSE_SERVICE_PORT_LOCALLY',
        target: { clusterProfileId: selection.clusterProfileId, context: selection.context, namespace: entry.namespace!, kind: 'Service', name: entry.name },
        expectedGeneration: selection.generation,
      }, csrfToken, createIdempotencyKey())
    },
    onSuccess: (session) => {
      toast.success('Service port-forward active', `127.0.0.1:${session.localPort} → ${entry.namespace}/${entry.name}:${servicePort} through Pod ${session.pod}.`)
      void queryClient.invalidateQueries({ queryKey: ['port-forwards'] })
    },
    onError: (error) => {
      toast.error('Service port-forward failed', errorMessage(error))
      void queryClient.invalidateQueries({ queryKey: ['service-port-forward-permissions', selection.generation] })
    },
  })
  const clusterIP = detail.summary.clusterIPs.find((value) => value && value !== 'None') ?? ''
  const disabledReason = !hasSelector ? 'This Service has no selector, so KubePeep cannot resolve a backing Pod.'
    : prerequisites.isPending ? 'Checking Service and Pod permissions with Kubernetes.'
      : prerequisites.isError ? 'The permission check failed; the action remains disabled.'
        : !canStart ? 'Kubernetes denied one or more permissions required to resolve and forward this Service.' : undefined
  return <div className="grid gap-3">
    <div className="flex flex-wrap gap-2"><Button onClick={() => setTab('endpoints')}>View Endpoints</Button><CopyAction label="Copy ClusterIP" value={clusterIP} /></div>
    <div className="flex flex-wrap items-end gap-2 border-t border-kp-overlay-0 pt-3">
      <label className="grid w-40 gap-1"><span className="text-column uppercase tracking-wider text-kp-overlay-text">Service port</span><Select aria-label="Service port" value={servicePort} onChange={(event) => setServicePort(Number(event.target.value))}>{tcpPorts.length === 0 ? <option value={0}>No TCP ports</option> : tcpPorts.map((port) => <option key={`${port.name ?? ''}/${port.port}`} value={port.port}>{port.name ? `${port.name} · ` : ''}{port.port}</option>)}</Select></label>
      <label className="grid w-40 gap-1"><span className="text-column uppercase tracking-wider text-kp-overlay-text">Local port (optional)</span><Input aria-label="Service local port" aria-invalid={!localValid} inputMode="numeric" value={localPort} onChange={(event) => setLocalPort(event.target.value)} placeholder="automatic" /></label>
      <Button disabled={!canStart || portForward.isPending} disabledReason={disabledReason} onClick={() => portForward.mutate()}>{portForward.isPending ? 'Starting…' : 'Start port-forward'}</Button>
    </div>
    {!localValid ? <p className="m-0 text-content text-kp-red" role="alert">An explicit local port must be 1,024–65,535.</p> : null}
    <p className="m-0 text-content text-kp-overlay-text" role="note">KubePeep resolves one ready backing Pod, rechecks exact Pod port-forward permission, and binds only to 127.0.0.1.</p>
  </div>
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
  const raw = detail.data as Record<string, unknown>
  // Network DTOs put inventory fields in summary; configuration/RBAC DTOs
  // are flat. Both contracts must expose the selected object's actual data.
  const data = { ...(raw.summary as Record<string, unknown> | undefined), ...raw }
  const facts: Array<{ label: string; value: string }> = []
  const metadata = (data.metadata ?? data) as { resourceVersion?: string; creationTimestamp?: string; uid?: string }
  if (metadata?.creationTimestamp) facts.push({ label: 'Created', value: dateTime(metadata.creationTimestamp) })
  if (metadata?.uid) facts.push({ label: 'UID', value: metadata.uid })
  if (metadata?.resourceVersion) facts.push({ label: 'Resource version', value: metadata.resourceVersion })
  const simple = (key: string, label: string) => {
    const value = data[key]
    if (value === undefined || value === null || value === '') return
    facts.push({ label, value: overviewValue(value) })
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
  for (const [key, label] of [
    ['name', 'Name'], ['namespace', 'Namespace'], ['clusterIPs', 'Cluster IPs'], ['ports', 'Ports'],
    ['hosts', 'Hosts'], ['tlsHosts', 'TLS hosts'], ['loadBalancerAddresses', 'Load balancer addresses'],
    ['endpoints', 'Endpoints'], ['policyTypes', 'Policy types'], ['ruleSummary', 'Rules'],
    ['selector', 'Selector'], ['durationSeconds', 'Lease duration (seconds)'], ['renewTime', 'Last renewal'],
    ['capacity', 'Capacity'], ['allocatable', 'Allocatable'], ['accessModes', 'Access modes'], ['claim', 'Claim'],
    ['default', 'Default'], ['volumeBindingMode', 'Volume binding mode'], ['allowVolumeExpansion', 'Volume expansion'],
    ['attached', 'Attached'], ['persistentVolumeName', 'Persistent volume'], ['podInfoOnMount', 'Pod info on mount'],
    ['storageCapacity', 'Storage capacity'], ['fsGroupPolicy', 'FS group policy'], ['driverCount', 'Drivers'],
    ['drivers', 'Driver details'], ['hard', 'Hard limits'], ['used', 'Used'], ['items', 'Limits'],
    ['metricNames', 'Metrics'], ['resourceTargets', 'Resource targets'], ['minAvailable', 'Min available'],
    ['maxUnavailable', 'Max unavailable'], ['currentHealthy', 'Current healthy'], ['desiredHealthy', 'Desired healthy'],
    ['disruptionsAllowed', 'Disruptions allowed'], ['expectedPods', 'Expected Pods'], ['rules', 'Policy rules'],
    ['subjects', 'Subjects'], ['versions', 'Versions'], ['globalDefault', 'Global default'],
    ['preemptionPolicy', 'Preemption policy'], ['overhead', 'Overhead'], ['webhooks', 'Webhook details'],
    ['parameters', 'Parameters'], ['roles', 'Roles'], ['totalBytes', 'Data size (bytes)'],
    ['apiVersion', 'API version'], ['addresses', 'Addresses'], ['listeners', 'Listeners'],
    ['chart', 'Chart'], ['appVersion', 'App version'], ['revision', 'Revision'], ['driver', 'Storage'], ['updatedAt', 'Updated'],
  ]) simple(key, label)
  if (entry.collection === 'configmaps' && Array.isArray(data.entries)) facts.push({ label: 'Data keys', value: data.entries.map((item: { key: string }) => item.key).join(', ') || 'none' })
  return facts
}

function overviewValue(value: unknown): string {
  if (value === null || value === undefined) return '—'
  if (Array.isArray(value)) return value.map(overviewValue).join('; ') || 'none'
  if (typeof value === 'object') {
    const record = value as Record<string, unknown>
    if ('isInt' in record) return String(record.isInt ? record.int : record.string)
    return Object.entries(record).map(([key, item]) => `${key}: ${overviewValue(item)}`).join(', ') || 'none'
  }
  return String(value)
}

function secretNotice(label: string): string | null {
  if (label === 'Secret') {
    return 'Open Data to reveal authorized Secret values.'
  }
  return null
}

export function ResourceWorkspacePanel() {
  const workspace = useResourceWorkspace()
  const entry = workspace.active
  const autoRefresh = useAutoRefreshQueryOptions()
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
  const selection = status.data?.selection ?? null
  const generation = selection?.generation

  const detail = useQuery({
    queryKey: ['workspace-detail', generation, entry?.collection, entry?.kind, entry?.namespace, entry?.name],
    queryFn: ({ signal }) => fetchDetail(entry as WorkspaceEntry, signal, generation),
    enabled: Boolean(workspace.open && entry && generation),
    ...autoRefresh,
  })
  const investigation = useQuery({
    queryKey: ['investigation', generation, entry?.kind, entry?.namespace, entry?.name],
    queryFn: ({ signal }) => getInvestigation(entry?.kind ?? (entry?.collection === 'pods' ? 'Pod' : ''), entry!.namespace!, entry!.name, signal),
    enabled: Boolean(workspace.open && entry?.tab === 'investigation' && entry.namespace && generation && (entry.collection === 'pods' || entry.collection === 'workloads')),
    staleTime: 5_000,
  })
  const entryKey = entry ? `${generation ?? ''}|${entry.collection}|${entry.kind ?? ''}|${entry.namespace ?? ''}|${entry.name}` : ''

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
    if (target) workspace.openRelatedResource(target)
  }

  function detailFallback() {
    if (!selection) return <p className="text-content text-kp-overlay-text" role="note">Select a Kubernetes context to inspect resources.</p>
    if (detail.isPending) return <LoadingState label="Loading authorized detail…" />
    if (detail.isError) return <div><p className="text-content text-kp-red" role="alert">{errorMessage(detail.error)}</p><Button variant="secondary" disabled={detail.isFetching} onClick={() => void detail.refetch()}>Retry loading detail</Button></div>
    return null
  }

  function renderTab(tab: string) {
    if (parseDynamicCollection(activeEntry.collection)) {
      if (tab === 'yaml' && generation) return <Suspense fallback={<LoadingState label="Opening resource YAML…" />}><DynamicYAML key={entryKey} entry={activeEntry} generation={generation} /></Suspense>
      return detail.data?.type === 'other' ? <Suspense fallback={<LoadingState label="Opening resource detail…" />}><DynamicOverview value={detail.data.data as DynamicRow} /></Suspense> : detailFallback()
    }
    if (isHelmCollection(activeEntry.collection) && selection && activeEntry.namespace) {
      if (tab === 'values' || tab === 'manifest') return <Suspense fallback={<LoadingState label="Opening Helm document…" />}><HelmDocumentEditor key={`${entryKey}|${tab}`} driver={helmDriver(activeEntry.collection)} namespace={activeEntry.namespace} name={activeEntry.name} format={tab} selection={selection} /></Suspense>
      if (tab === 'history') return detail.data?.type === 'other' ? <Suspense fallback={<LoadingState label="Opening Helm history…" />}><HelmHistory key={entryKey} release={detail.data.data as HelmRelease} selection={selection} /></Suspense> : detailFallback()
    }
    if (tab === 'investigation') {
      if (investigation.isPending) return <p className="text-content text-kp-overlay-text" role="status">Reading the generation-scoped local relationship index…</p>
      if (investigation.isError) return <p className="text-content text-kp-red" role="alert">{errorMessage(investigation.error)}</p>
      if (investigation.data) return <InvestigationView value={investigation.data} onOpen={openRef} />
    }
    if (tab === 'data' && activeEntry.collection === 'secrets' && generation && activeEntry.namespace) return <SecretData key={entryKey} namespace={activeEntry.namespace} name={activeEntry.name} generation={generation} />
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
            {data.type === 'workload' ? <WorkloadProgress workload={data.data} /> : null}
            {data.type === 'other' && activeEntry.collection === 'hpas' ? <QuantityUsage label="HPA replica capacity" current={(data.data as HorizontalPodAutoscaler).currentReplicas} total={(data.data as HorizontalPodAutoscaler).maxReplicas} /> : null}
            {data.type === 'other' && activeEntry.collection === 'resource-quotas' ? <QuotaUsage quota={data.data as ResourceQuota} /> : null}
            {data.type === 'other' && activeEntry.collection === 'pdbs' ? <QuantityUsage label="PDB healthy pods" current={(data.data as PodDisruptionBudget).currentHealthy} total={(data.data as PodDisruptionBudget).desiredHealthy} completion failed={(data.data as PodDisruptionBudget).currentHealthy < (data.data as PodDisruptionBudget).desiredHealthy} /> : null}
            {data.type === 'workload' && data.data.conditions.length > 0 ? <ConditionsTable conditions={data.data.conditions} /> : null}
              {data.type === 'pod' ? <ConditionsTable conditions={data.data.conditions} /> : null}
              {data.type === 'other' && Array.isArray((data.data as { conditions?: unknown }).conditions) ? <ConditionsTable conditions={(data.data as { conditions: PodDetail['conditions'] }).conditions} /> : null}
            {data.type === 'other' ? <p className="m-0 text-content text-kp-overlay-text" role="note">{secretNotice(data.label) ?? ''}</p> : null}
          </div>
          <div className="grid content-start gap-2 min-w-0">
            {(data.type === 'pod' || data.type === 'workload') ? <>
              <h3 className="m-0 text-heading text-kp-text">Secrets</h3>
              <RelatedRefList refs={(data.type === 'pod' ? data.data.summary.secrets ?? [] : data.data.secrets ?? []).map((name) => ({ kind: 'Secret', name, namespace: activeEntry.namespace ?? undefined }))} onOpen={openRef} emptyNote="No Secret references declared." />
              <h3 className="m-0 mt-2 text-heading text-kp-text">ConfigMaps</h3>
              <RelatedRefList refs={(data.type === 'pod' ? data.data.summary.configMaps ?? [] : data.data.configMaps ?? []).map((name) => ({ kind: 'ConfigMap', name, namespace: activeEntry.namespace ?? undefined }))} onOpen={openRef} emptyNote="No ConfigMap references declared." />
            </> : null}
            <h3 className="m-0 mt-2 text-heading text-kp-text">Related resources</h3>
            {data.type === 'workload' ? <RelatedRefList refs={related} onOpen={openRef} emptyNote="No related objects are visible in the current scope." /> : null}
            {data.type === 'pod' ? <RelatedRefList refs={owner} onOpen={openRef} emptyNote="This Pod has no controller owner." /> : null}
            {data.type === 'pod' && podRefs.length > 0 ? (
              <>
                <h3 className="m-0 mt-2 text-heading text-kp-text">Related events</h3>
                <RelatedRefList refs={podRefs} onOpen={openRef} emptyNote="" />
              </>
            ) : null}
            {data.type === 'other' ? <RelatedRefList refs={relatedResources(activeEntry.collection, data.data, activeEntry.namespace)} onOpen={openRef} emptyNote="No explicit resource references are available in this detail." /> : null}
          </div>
        </div>
      )
    }
    if (tab === 'yaml' && selection) {
      const collection = activeEntry.collection === 'workloads' ? workloadKindPath(activeEntry.kind!) : activeEntry.collection
      if (!collection) return null
      return <Suspense fallback={<LoadingState label="Opening YAML editor…" />}><ResourceYamlEditor key={entryKey} collection={collection} namespace={activeEntry.namespace} name={activeEntry.name} selection={selection} /></Suspense>
    }
    if (tab === 'events') return <WorkspaceEvents entry={activeEntry} generation={generation} />
      if (tab === 'logs') {
        if (detail.data?.type === 'pod' && selection) return <Suspense fallback={<LoadingState label="Opening logs…" />}><PodLogsPanel key={entryKey} pods={[{ namespace: activeEntry.namespace!, name: activeEntry.name }]} selection={selection} /></Suspense>
          if (detail.data?.type === 'workload' && selection) return <Suspense fallback={<LoadingState label="Opening logs…" />}><WorkloadLogsPanel key={entryKey} detail={detail.data.data} selection={selection} /></Suspense>
        return detailFallback()
      }
      if (tab === 'metrics') {
        if (detail.data?.type === 'pod') return <PodMetrics detail={detail.data.data} generation={generation} />
        if (detail.data?.type === 'workload') return <WorkloadMetrics detail={detail.data.data} generation={generation} />
        return detailFallback()
      }
      if (tab === 'containers') {
        if (detail.data?.type === 'pod') return <PodContainers detail={detail.data.data} />
        return detailFallback()
      }
      if (tab === 'actions') {
        if (!detail.data || detail.isPending || detail.isError || !selection) return detailFallback()
          if (detail.data.type === 'pod') return <Suspense fallback={<LoadingState label="Opening Pod actions…" />}><PodActions detail={detail.data.data} selection={selection} /></Suspense>
          if (detail.data.type === 'workload') return <Suspense fallback={<LoadingState label="Opening workload actions…" />}><WorkloadActions detail={detail.data.data} selection={selection} /></Suspense>
          if (detail.data.type === 'other' && (activeEntry.collection === 'services' || activeEntry.collection === 'ingresses')) return <NetworkActions entry={activeEntry} detail={detail.data.data as ServiceDetail | IngressDetail} selection={selection} setTab={workspace.setTab} />
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
    if (tab === 'backends') {
      if (detail.data?.type === 'other' && activeEntry.collection === 'ingresses') return <IngressBackends detail={detail.data.data as IngressDetail} onOpen={openRef} />
      return detailFallback()
    }
    if (tab === 'data') {
      if (detail.data?.type === 'pod') return <WorkloadEnvironment containers={[...detail.data.data.containers, ...detail.data.data.initContainers, ...detail.data.data.ephemeralContainers].map((container) => container.spec)} namespace={activeEntry.namespace!} onOpen={openRef} />
      if (detail.data?.type === 'workload') return <WorkloadEnvironment containers={detail.data.data.containers} namespace={activeEntry.namespace!} onOpen={openRef} />
      if (detail.data?.type === 'other') {
        const data = detail.data.data as { entries?: Array<{ key: string; encoding: 'utf-8' | 'base64'; value: string; truncated: boolean }> }
        return <DataEntries entries={data.entries ?? []} />
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
            <p className="m-0 text-content text-kp-overlay-text" role="note">Progressing/Available conditions come from the controller; rollout completion is observed, not promised.</p>
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
      <div className="workspace-panel" role="region" aria-label={`${kindLabel} ${activeEntry.name}`}>
        <header className="workspace-header">
          <div className="workspace-history">
          <Button variant="icon" className="workspace-nav-btn" onClick={workspace.back} disabled={!workspace.canBack} disabledReason="There is no previous resource in this workspace history." aria-label="Go to previous resource">
            <ChevronLeft size={16} aria-hidden="true" />
          </Button>
          <Button variant="icon" className="workspace-nav-btn" onClick={workspace.forward} disabled={!workspace.canForward} disabledReason="There is no next resource in this workspace history." aria-label="Go to next resource">
            <ChevronRight size={16} aria-hidden="true" />
          </Button>
          </div>
          <div className="workspace-identity"><Badge variant="accent">{kindLabel}</Badge>
          <strong className="min-w-0 truncate text-heading text-kp-text">{activeEntry.name}</strong>
          {activeEntry.namespace ? <span className="shrink-0 text-content text-kp-overlay-text">ns: {activeEntry.namespace}</span> : <span className="shrink-0 text-content text-kp-overlay-text">cluster-scoped</span>}
          </div><div className="workspace-actions">
          {favoriteKind ? <FavoriteButton kind={favoriteKind} namespace={activeEntry.namespace ?? undefined} name={activeEntry.name} generation={generation} label={kindLabel} /> : null}
          <button type="button" onClick={workspace.close} className="control control-icon grid place-items-center text-kp-overlay-text hover:text-kp-text hover:bg-kp-surface-3" aria-label="Close resource workspace">
            <X size={16} aria-hidden="true" />
          </button></div>
        </header>
        <ResourceTabStrip tabs={tabs} active={activeTab} onChange={workspace.setTab} ariaLabel={`${kindLabel} workspace tabs`} panelId="workspace-tabpanel" />
        <div className="workspace-body">
          <div key={`${entryKey}|${activeTab}`} className={`workspace-tab-content${activeTab === 'logs' ? ' workspace-tab-content--logs' : ''}`} id="workspace-tabpanel" role="tabpanel" tabIndex={0}>
            {renderTab(activeTab)}
          </div>
        </div>
      </div>
    </>
  )
}
import { isGatewayCollection } from '../../navigation/gateway'
import { getGatewayResource } from '../../api/client'
import { parseDynamicCollection } from '../../navigation/dynamic'
import { getDynamicResource } from '../../api/client'
import type { DynamicRow } from '../../api/types'
const DynamicOverview = lazy(() => import('./DynamicDetails').then(module => ({ default: module.DynamicOverview })))
const DynamicYAML = lazy(() => import('./DynamicDetails').then(module => ({ default: module.DynamicYAML })))
