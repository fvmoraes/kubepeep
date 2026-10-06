import { useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

/** Portaled so column controls remain usable inside a scrolling table. */
export function TableMenu({ label, icon, children, active = false }: { label: string; icon: ReactNode; children: ReactNode; active?: boolean }) {
  const trigger = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null)
  useEffect(() => {
    if (!position) return
    panel.current?.focus()
    const close = () => setPosition(null)
    const outside = (event: PointerEvent) => {
      if (!panel.current?.contains(event.target as Node) && !trigger.current?.contains(event.target as Node)) close()
    }
    const keyboard = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.stopPropagation(); close(); trigger.current?.focus() }
    }
    const scroll = (event: Event) => { if (!panel.current?.contains(event.target as Node)) close() }
    document.addEventListener('pointerdown', outside)
    document.addEventListener('keydown', keyboard, true)
    document.addEventListener('scroll', scroll, true)
    window.addEventListener('resize', close)
    return () => {
      document.removeEventListener('pointerdown', outside)
      document.removeEventListener('keydown', keyboard, true)
      document.removeEventListener('scroll', scroll, true)
      window.removeEventListener('resize', close)
    }
  }, [position])
  return <>
    <button ref={trigger} type="button" className={`table-menu-trigger ${active ? 'text-kp-mauve' : ''}`} aria-label={label} aria-expanded={Boolean(position)} aria-haspopup="dialog" onClick={() => {
      const rect = trigger.current!.getBoundingClientRect()
      setPosition(position ? null : { top: Math.max(8, Math.min(rect.bottom + 4, window.innerHeight - 320)), left: Math.max(8, Math.min(rect.left, window.innerWidth - 272)) })
    }}>{icon}</button>
    {position ? createPortal(<div ref={panel} role="dialog" aria-label={label} tabIndex={-1} className="table-menu" style={{ ...position, maxHeight: `min(480px, calc(100dvh - ${position.top + 8}px))` }} onBlur={(event) => { if (event.relatedTarget && !event.currentTarget.contains(event.relatedTarget as Node) && event.relatedTarget !== trigger.current) setPosition(null) }}>{children}</div>, document.body) : null}
  </>
}
