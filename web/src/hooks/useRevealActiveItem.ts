import { useEffect, type RefObject } from 'react'

/** Reveal only on the horizontal axis, preserving the page and detail scroll. */
export function useRevealActiveItem(ref: RefObject<HTMLElement | null>, selection: string | undefined) {
  useEffect(() => {
    const element = ref.current
    if (!element) return
    const reveal = () => {
      const active = element.querySelector<HTMLElement>('[aria-current="page"], [aria-selected="true"]')
      if (!active) return
      const container = element.getBoundingClientRect()
      const item = active.getBoundingClientRect()
      if (item.left < container.left) element.scrollLeft += item.left - container.left
      else if (item.right > container.right) element.scrollLeft += item.right - container.right
    }
    reveal()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(reveal)
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref, selection])
}
