import {
  Anchor,
  ArrowLeftRight,
  Boxes,
  Braces,
  CalendarClock,
  Container,
  Database,
  FileCog,
  FileText,
  Gauge,
  Globe,
  HardDrive,
  KeyRound,
  Layers,
  LayoutDashboard,
  Lock,
  Network,
  Package,
  Rocket,
  Ruler,
  ScrollText,
  Server,
  Settings,
  Ship,
  ShieldCheck,
  Split,
  Timer,
  TrendingUp,
  Users,
  Waypoints,
  type LucideIcon,
} from 'lucide-react'

export interface NavItem {
  id: string
  label: string
  /** Route path. Undefined = not implemented yet (rendered disabled). */
  path?: string
  icon: LucideIcon
  /** Short tooltip text for the compact sidebar. */
  tip?: string
  /** Extra search terms for the command palette. */
  keywords?: string[]
}

export interface NavGroup {
  id: string
  label: string
  items: NavItem[]
}

/**
 * Single source of truth for the Kubernetes navigation tree.
 * Items without a path are prepared for future backend support and render
 * as disabled entries with a "coming soon" tooltip.
 */
export const navGroups: NavGroup[] = [
  {
    id: 'cluster',
    label: 'Cluster',
    items: [
      { id: 'overview', label: 'Overview', path: '/', icon: LayoutDashboard, tip: 'Overview', keywords: ['dashboard', 'health'] },
      { id: 'nodes', label: 'Nodes', path: '/nodes', icon: Server, tip: 'Nodes' },
      { id: 'events', label: 'Events', path: '/events', icon: FileText, tip: 'Events', keywords: ['warning', 'reason'] },
      { id: 'namespaces', label: 'Namespaces', path: '/namespaces', icon: Boxes, tip: 'Namespaces', keywords: ['scope'] },
      { id: 'leases', label: 'Leases', path: '/leases', icon: Timer, tip: 'Leases' },
    ],
  },
  {
    id: 'workloads',
    label: 'Workloads',
    items: [
      { id: 'workloads-overview', label: 'Overview', path: '/workloads', icon: Gauge, tip: 'Workloads' },
      { id: 'deployments', label: 'Deployments', path: '/workloads/kind/deployments', icon: Rocket, tip: 'Deployments' },
      { id: 'pods', label: 'Pods', path: '/pods', icon: Container, tip: 'Pods' },
      { id: 'replicasets', label: 'ReplicaSets', path: '/workloads/kind/replicasets', icon: Layers, tip: 'ReplicaSets' },
      { id: 'daemonsets', label: 'DaemonSets', path: '/workloads/kind/daemonsets', icon: Boxes, tip: 'DaemonSets' },
      { id: 'statefulsets', label: 'StatefulSets', path: '/workloads/kind/statefulsets', icon: Database, tip: 'StatefulSets' },
      { id: 'jobs', label: 'Jobs', path: '/workloads/kind/jobs', icon: Package, tip: 'Jobs' },
      { id: 'cronjobs', label: 'CronJobs', path: '/workloads/kind/cronjobs', icon: CalendarClock, tip: 'CronJobs' },
    ],
  },
  {
    id: 'helm',
    label: 'Helm',
    items: [
      { id: 'helm-releases', label: 'Releases', icon: Ship, tip: 'Helm Releases' },
    ],
  },
  {
    id: 'network',
    label: 'Network',
    items: [
      { id: 'services', label: 'Services', path: '/network/services', icon: Network, tip: 'Services' },
      { id: 'endpoints', label: 'Endpoints', icon: ArrowLeftRight, tip: 'Endpoints' },
      { id: 'endpoint-slices', label: 'EndpointSlices', path: '/network/endpoint-slices', icon: Split, tip: 'EndpointSlices' },
      { id: 'ingresses', label: 'Ingresses', path: '/network/ingresses', icon: Globe, tip: 'Ingresses' },
      { id: 'ingress-classes', label: 'IngressClasses', icon: Waypoints, tip: 'IngressClasses' },
      { id: 'network-policies', label: 'NetworkPolicies', icon: Braces, tip: 'NetworkPolicies' },
      { id: 'gateway-api', label: 'Gateway API', icon: Anchor, tip: 'Gateway API' },
      { id: 'port-forwarding', label: 'Port Forwarding', path: '/network/port-forwards', icon: ArrowLeftRight, tip: 'Port Forwarding' },
    ],
  },
  {
    id: 'configuration',
    label: 'Configuration',
    items: [
      { id: 'configmaps', label: 'ConfigMaps', path: '/config/configmaps', icon: Braces, tip: 'ConfigMaps' },
      { id: 'secrets', label: 'Secrets', path: '/config/secrets', icon: Lock, tip: 'Secrets' },
      { id: 'resource-quotas', label: 'ResourceQuotas', path: '/configuration/resource-quotas', icon: Gauge, tip: 'ResourceQuotas' },
      { id: 'limit-ranges', label: 'LimitRanges', path: '/configuration/limit-ranges', icon: Ruler, tip: 'LimitRanges' },
      { id: 'hpa', label: 'HorizontalPodAutoscalers', path: '/configuration/hpas', icon: TrendingUp, tip: 'Horizontal Pod Autoscalers' },
      { id: 'pdb', label: 'PodDisruptionBudgets', path: '/configuration/pdbs', icon: ShieldCheck, tip: 'Pod Disruption Budgets' },
    ],
  },
  {
    id: 'storage',
    label: 'Storage',
    items: [
      { id: 'pv', label: 'PersistentVolumes', path: '/storage/persistent-volumes', icon: HardDrive, tip: 'Persistent Volumes' },
      { id: 'pvc', label: 'PersistentVolumeClaims', path: '/storage/persistent-volume-claims', icon: HardDrive, tip: 'Persistent Volume Claims' },
      { id: 'volume-attachments', label: 'VolumeAttachments', path: '/storage/volume-attachments', icon: HardDrive, tip: 'Volume Attachments' },
      { id: 'storage-classes', label: 'StorageClasses', path: '/storage/storage-classes', icon: HardDrive, tip: 'Storage Classes' },
      { id: 'csi-nodes', label: 'CSINodes', path: '/storage/csi-nodes', icon: HardDrive, tip: 'CSI Nodes' },
      { id: 'csi-drivers', label: 'CSIDrivers', path: '/storage/csi-drivers', icon: HardDrive, tip: 'CSI Drivers' },
    ],
  },
  {
    id: 'access-control',
    label: 'Access Control',
    items: [
      { id: 'service-accounts', label: 'ServiceAccounts', path: '/service-accounts', icon: Users, tip: 'Service Accounts' },
      { id: 'roles', label: 'Roles', icon: KeyRound, tip: 'Roles' },
      { id: 'role-bindings', label: 'RoleBindings', icon: KeyRound, tip: 'Role Bindings' },
      { id: 'cluster-roles', label: 'ClusterRoles', icon: KeyRound, tip: 'Cluster Roles' },
      { id: 'cluster-role-bindings', label: 'ClusterRoleBindings', icon: KeyRound, tip: 'Cluster Role Bindings' },
      { id: 'permissions', label: 'Permissions', path: '/permissions', icon: ShieldCheck, tip: 'Permissions', keywords: ['rbac', 'authorization'] },
    ],
  },
  {
    id: 'observability',
    label: 'Observability',
    items: [
      { id: 'logs', label: 'Logs', path: '/logs', icon: ScrollText, tip: 'Logs' },
    ],
  },
  {
    id: 'administration',
    label: 'Administration',
    items: [
      { id: 'crds', label: 'CustomResourceDefinitions', icon: FileCog, tip: 'CRDs' },
      { id: 'priority-classes', label: 'PriorityClasses', icon: Layers, tip: 'Priority Classes' },
      { id: 'runtime-classes', label: 'RuntimeClasses', icon: Container, tip: 'Runtime Classes' },
      { id: 'admission-webhooks', label: 'Admission Webhooks', icon: FileCog, tip: 'Admission Webhooks' },
    ],
  },
]

/** Pinned at the bottom of the sidebar, outside the Kubernetes tree. */
export const settingsNavItem: NavItem = {
  id: 'settings',
  label: 'Settings',
  path: '/settings',
  icon: Settings,
  tip: 'Settings',
  keywords: ['preferences'],
}
