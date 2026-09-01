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

export function Tabs({ tabs, activeTab, onChange, className = '' }: TabsProps) {
  return (
    <div className={className}>
      <div
        role="tablist"
        className="flex w-fit max-w-full gap-1 overflow-x-auto p-1 border border-kp-overlay-0 rounded-xl bg-kp-surface-0"
      >
        {tabs.map((tab) => {
          const isActive = tab.id === activeTab
          return (
            <button
              key={tab.id}
              role="tab"
              aria-selected={isActive}
              type="button"
              onClick={() => onChange(tab.id)}
              className={`
                min-h-9 px-2.5 rounded-md text-base whitespace-nowrap cursor-pointer
                ${isActive ? 'text-kp-mauve bg-kp-surface-4 border border-kp-overlay-4' : 'text-kp-subtext bg-transparent border border-transparent hover:bg-kp-surface-2 hover:text-kp-text'}
              `}
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
