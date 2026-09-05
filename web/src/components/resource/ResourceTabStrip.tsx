export interface ResourceTab {
  id: string
  label: string
}

/** Underline tab strip used to switch resource families inside a page. */
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
            onClick={() => onChange(tab.id)}
            className={`-mb-px h-8 whitespace-nowrap border-b-2 px-3 text-sm cursor-pointer transition-colors ${
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
