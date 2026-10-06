export interface ResourceTab {
  id: string
  label: string
}

/** Underline tabs for views within a resource; route menus use links. */
export function ResourceTabStrip({ tabs, active, onChange, ariaLabel, panelId }: { tabs: readonly ResourceTab[]; active: string; onChange: (id: string) => void; ariaLabel: string; panelId: string }) {
  return (
    <div role="tablist" aria-label={ariaLabel} className="flex w-fit max-w-full gap-0.5 overflow-x-auto border-b border-kp-overlay-0">
      {tabs.map((tab) => {
        const isActive = tab.id === active
        return (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={isActive}
            aria-controls={panelId}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onChange(tab.id)}
            onKeyDown={(event) => {
              const index = tabs.findIndex((item) => item.id === tab.id)
              const next = event.key === 'ArrowRight' ? (index + 1) % tabs.length : event.key === 'ArrowLeft' ? (index - 1 + tabs.length) % tabs.length : event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : null
              if (next === null) return
              event.preventDefault()
              onChange(tabs[next].id)
              const button = event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]
              button?.focus()
              button?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
            }}
            className={`-mb-px h-10 shrink-0 whitespace-nowrap border-b-2 px-3 text-sm cursor-pointer transition-colors ${
              isActive
                ? 'border-kp-mauve font-medium text-kp-text'
                : 'border-transparent text-kp-overlay-text hover:border-kp-overlay-1 hover:text-kp-subtext'
            }`}
          >
            {tab.label}
          </button>
        )
      })}
    </div>
  )
}
