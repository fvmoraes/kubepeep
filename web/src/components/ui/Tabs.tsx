import type { ReactNode } from 'react'

export interface Tab {
  id: string
  label: ReactNode
  content: ReactNode
}

export interface TabsProps {
  tabs: Tab[]
  activeTab: string
  onChange: (id: string) => void
  className?: string
}

/** Underline-style tab strip — compact and consistent across pages. */
export function Tabs({ tabs, activeTab, onChange, className = '' }: TabsProps) {
  return (
    <div className={className}>
      <div role="tablist" className="flex w-fit max-w-full gap-0.5 overflow-x-auto border-b border-kp-overlay-0">
        {tabs.map((tab) => {
          const isActive = tab.id === activeTab
          return (
            <button
              key={tab.id}
              role="tab"
              aria-selected={isActive}
              type="button"
              onClick={() => onChange(tab.id)}
              className={`-mb-px h-8 px-3 border-b-2 whitespace-nowrap cursor-pointer text-sm transition-colors ${
                isActive
                  ? 'border-kp-mauve text-kp-text font-medium'
                  : 'border-transparent text-kp-overlay-text hover:text-kp-subtext hover:border-kp-overlay-1'
              }`}
            >
              {tab.label}
            </button>
          )
        })}
      </div>
      <div role="tabpanel" className="mt-4">
        {tabs.find((tab) => tab.id === activeTab)?.content}
      </div>
    </div>
  )
}
