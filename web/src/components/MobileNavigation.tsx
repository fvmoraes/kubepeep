import { useEffect, useRef, useState, type ComponentProps } from 'react'
import { Menu, X } from 'lucide-react'

import { Sidebar } from './Sidebar'
import { Button } from './ui/Button'

/** One navigation catalog, with native modal focus handling on small screens. */
export function MobileNavigation(props: ComponentProps<typeof Sidebar>) {
  const [open, setOpen] = useState(false)
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    if (!open) return
    const element = dialog.current!
    const previousOverflow = document.body.style.overflow
    element.showModal()
    document.body.style.overflow = 'hidden'
    return () => {
      element.close()
      document.body.style.overflow = previousOverflow
    }
  }, [open])
  return <>
    <Button variant="icon" aria-label="Open navigation" aria-haspopup="dialog" aria-expanded={open} aria-controls="mobile-navigation" onClick={() => setOpen(true)}><Menu size={18} aria-hidden="true" /></Button>
    <dialog ref={dialog} id="mobile-navigation" aria-label="Application navigation" className="mobile-navigation" onKeyDown={(event) => event.stopPropagation()} onCancel={() => setOpen(false)} onClick={(event) => { if (event.target === event.currentTarget) setOpen(false) }}>
      <Button variant="icon" className="mobile-navigation-close" aria-label="Close navigation" onClick={() => setOpen(false)}><X size={18} aria-hidden="true" /></Button>
      <Sidebar {...props} compact={false} onNavigate={() => setOpen(false)} />
    </dialog>
  </>
}
