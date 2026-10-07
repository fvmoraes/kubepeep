import { createContext, useContext, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

const DetailHost = createContext<HTMLDivElement | null | undefined>(undefined)

/** One viewport split for every route, including route-owned aggregate logs. */
export function ResourceSplitView({ children, detail }: { children: ReactNode; detail: ReactNode }) {
  const [host, setHost] = useState<HTMLDivElement | null>(null)
  return <DetailHost.Provider value={host}>
    <div className="resource-split">
      <div className="resource-list-pane" role="region" aria-label="Page content" tabIndex={0}>{children}</div>
      <div ref={setHost} className="resource-detail-slot">{detail}</div>
    </div>
  </DetailHost.Provider>
}

export function ResourceDetailPortal({ children }: { children: ReactNode }) {
  const host = useContext(DetailHost)
  // Keep standalone component consumers working outside the application shell.
  if (host === undefined) return children
  return host ? createPortal(children, host) : null
}
