import { useMemo } from 'react'
import { NavLink, useLocation } from 'react-router'
import { PanelLeftClose, PanelLeftOpen } from 'lucide-react'

import { BrandLogo } from './BrandLogo'
import { BrandWordmark } from './BrandWordmark'
import { navGroups, settingsNavItem, type NavGroup, type NavItem } from '../navigation/tree'

// Sidebar preferences stay in memory on purpose: production sources never
// touch browser storage (see src/security.test.ts). The collapsed/compact
// state resets when the application restarts.
function groupOwnsPath(group: NavGroup, pathname: string): boolean {
  return group.items.some((item) => item.path && (item.path === '/' ? pathname === '/' : pathname.startsWith(item.path)))
}

interface SidebarProps {
  version: string
  compact: boolean
  onToggleCompact: () => void
  collapsedGroups: string[]
  onToggleGroup: (id: string) => void
}

export function Sidebar({ version, compact, onToggleCompact, collapsedGroups, onToggleGroup }: SidebarProps) {
  const location = useLocation()

  // The group owning the active route is always rendered expanded (derived,
  // never stored, so navigation can never hide the active item).
  const activeGroupId = useMemo(
    () => navGroups.find((group) => groupOwnsPath(group, location.pathname))?.id ?? null,
    [location.pathname],
  )

  const toggleGroup = (id: string) => {
    onToggleGroup(id)
  }

  const renderItem = useMemo(() => {
    return (item: NavItem) => {
      const Icon = item.icon
      if (!item.path) {
        return (
          <span
            key={item.id}
            aria-disabled="true"
            data-tip={compact ? `${item.tip ?? item.label} — available in a future release` : `${item.tip ?? item.label} — available in a future release`}
            className="flex h-8 items-center gap-2.5 rounded-md px-2.5 text-kp-text-disabled cursor-default select-none"
          >
            <Icon size={16} strokeWidth={1.8} aria-hidden="true" />
            {!compact ? <span className="truncate">{item.label}</span> : null}
          </span>
        )
      }
      return (
        <NavLink
          key={item.id}
          to={item.path}
          end={item.path === '/' || item.path === '/workloads'}
          data-tip={compact ? item.tip ?? item.label : undefined}
          className={({ isActive }) => `flex h-8 items-center gap-2.5 rounded-md px-2.5 text-sm transition-colors ${
            isActive
              ? 'bg-kp-accent-bg text-kp-mauve font-medium'
              : 'text-kp-subtext hover:bg-kp-surface-3 hover:text-kp-text'
          } ${compact ? 'justify-center px-0' : ''}`}
        >
          <Icon size={16} strokeWidth={1.8} aria-hidden="true" />
          {!compact ? <span className="truncate">{item.label}</span> : null}
        </NavLink>
      )
    }
  }, [compact])

  return (
    <aside
      className={`flex h-full flex-col border-r border-kp-overlay-0 bg-kp-mantle ${compact ? 'w-[var(--sidebar-width-compact)]' : 'w-[var(--sidebar-width)]'}`}
    >
      <div className={`flex h-[var(--header-height)] shrink-0 items-center gap-2.5 border-b border-kp-overlay-0 px-3 ${compact ? 'justify-center px-0' : ''}`}>
        <BrandLogo size={28} />
        {!compact ? (
          <div className="min-w-0 leading-tight">
            <BrandWordmark height={15} />
            <small className="block text-2xs text-kp-overlay-text" title={`KubePeep ${version}`}>{version}</small>
          </div>
        ) : null}
      </div>

      <nav aria-label="Primary navigation" className="flex min-h-0 flex-1 flex-col">
        <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden px-2 py-2">
          {compact ? (
            <div className="grid gap-0.5">
              {navGroups.flatMap((group) => group.items.map(renderItem))}
            </div>
          ) : (
            <div className="grid gap-3">
              {navGroups.map((group) => {
                const isCollapsed = collapsedGroups.includes(group.id) && group.id !== activeGroupId
                return (
                  <section key={group.id} aria-label={group.label}>
                    <button
                      type="button"
                      aria-expanded={!isCollapsed}
                      onClick={() => toggleGroup(group.id)}
                      className="flex h-6 w-full items-center gap-1 rounded px-1.5 text-2xs font-semibold text-kp-overlay-text uppercase tracking-wider hover:text-kp-subtext"
                    >
                      <svg
                        viewBox="0 0 24 24"
                        aria-hidden="true"
                        className={`h-3 w-3 transition-transform duration-100 ${isCollapsed ? '-rotate-90' : ''}`}
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2.5"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="m6 9 6 6 6-6" />
                      </svg>
                      {group.label}
                    </button>
                    {!isCollapsed ? <div className="mt-0.5 grid gap-0.5">{group.items.map(renderItem)}</div> : null}
                  </section>
                )
              })}
            </div>
          )}
        </div>
        <div className="shrink-0 border-t border-kp-overlay-0 px-2 py-2">
          <div className="grid gap-0.5">
            {renderItem(settingsNavItem)}
            <button
              type="button"
              onClick={onToggleCompact}
              data-tip={compact ? 'Expand sidebar' : 'Collapse sidebar'}
              aria-label={compact ? 'Expand sidebar' : 'Collapse sidebar'}
              className={`flex h-8 items-center gap-2.5 rounded-md px-2.5 text-kp-overlay-text hover:bg-kp-surface-3 hover:text-kp-text ${compact ? 'justify-center px-0' : ''}`}
            >
              {compact ? <PanelLeftOpen size={16} strokeWidth={1.8} aria-hidden="true" /> : <PanelLeftClose size={16} strokeWidth={1.8} aria-hidden="true" />}
              {!compact ? <span className="text-sm">Collapse</span> : null}
            </button>
          </div>
        </div>
      </nav>
    </aside>
  )
}
