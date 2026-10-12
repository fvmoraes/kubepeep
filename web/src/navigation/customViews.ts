import { Layers } from 'lucide-react'
import type { Preferences, SelectionSummary } from '../api/types'
import { dynamicCollection, dynamicListPath } from './dynamic'
import type { NavItem } from './tree'

export function customViewNavItems(preferences: Preferences | undefined, selection: SelectionSummary | null): NavItem[] {
  const views = preferences?.customViews?.find(value => value.clusterProfileId === selection?.clusterProfileId && value.context === selection?.context && value.cluster === selection?.cluster)?.items ?? []
  return views.map(view => ({
    id: dynamicCollection(view),
    label: views.filter(other => other.kind === view.kind).length > 1 ? `${view.kind} · ${view.group || 'core'}/${view.version}` : view.kind,
    path: dynamicListPath(view),
    icon: Layers,
    tip: `${view.kind} · ${view.group || 'core'}/${view.version}`,
  }))
}
