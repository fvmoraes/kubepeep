import { createContext, useContext, useLayoutEffect, useMemo, useState, type Dispatch, type ReactNode, type SetStateAction } from 'react'
import { createPortal } from 'react-dom'

const DetailHost = createContext<{ host: HTMLDivElement | null; setPortalCount: Dispatch<SetStateAction<number>> } | undefined>(undefined)

/** One bottom overlay for every route, including route-owned aggregate logs. */
export function ResourceSplitView({ children, detail }: { children: ReactNode; detail: ReactNode }) {
  const [host, setHost] = useState<HTMLDivElement | null>(null)
  const [portalCount, setPortalCount] = useState(0)
  const context = useMemo(() => ({ host, setPortalCount }), [host])
  const detailOpen = Boolean(detail) || portalCount > 0
  return <DetailHost.Provider value={context}>
    <div className="resource-split" data-detail-open={detailOpen}>
      <div className="resource-list-pane" role="region" aria-label="Page content" tabIndex={0}>{children}</div>
      <div ref={setHost} className="resource-detail-slot" hidden={!detailOpen}>{detail}</div>
    </div>
  </DetailHost.Provider>
}

export function ResourceDetailPortal({ children }: { children: ReactNode }) {
  const context = useContext(DetailHost)
  const setPortalCount = context?.setPortalCount
  const active = Boolean(children)
  useLayoutEffect(() => {
    if (!setPortalCount || !active) return
    setPortalCount((count) => count + 1)
    return () => setPortalCount((count) => count - 1)
  }, [active, setPortalCount])
  // Keep standalone component consumers working outside the application shell.
  if (context === undefined) return children
  return context.host ? createPortal(children, context.host) : null
}
