import { Link, useLocation } from 'react-router'
import { activeNavItem, navGroups, settingsNavItem } from '../../navigation/tree'
import { beginViewNavigation } from '../../observability/uxMetrics'

/** Both navigation surfaces resolve their selection from the same route tree. */
export function ResourceFamilyNav() {
  const { pathname } = useLocation()
  const active = activeNavItem(pathname)
  const group = navGroups.find((entry) => entry.items.some((item) => item.id === active?.id))
  const items = group?.items ?? (active?.id === settingsNavItem.id ? [settingsNavItem] : [])
  if (!items.length) return null
  return <nav aria-label={`${group?.label ?? 'Settings'} resources`} className="resource-family-nav">
    {items.map((item) => item.path ? <Link
      key={item.id}
      to={item.path}
      aria-current={active?.id === item.id ? 'page' : undefined}
      onClick={(event) => { if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey) beginViewNavigation(item.path!) }}
    >{item.label}</Link> : <span key={item.id} aria-disabled="true" title="Available in a future release">{item.label}</span>)}
  </nav>
}
