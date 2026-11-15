import { useEffect } from "react"

export function useScrollVisibility() {
  useEffect(() => {
    const timers = new Map<HTMLElement, number>()

    const onScroll = (event: Event) => {
      const target = event.target === document ? document.scrollingElement : event.target
      if (!(target instanceof HTMLElement)) return

      window.clearTimeout(timers.get(target))
      target.setAttribute("data-scrolling", "")
      timers.set(target, window.setTimeout(() => {
        target.removeAttribute("data-scrolling")
        timers.delete(target)
      }, 700))
    }

    // Capture scrolls from nested panels and dialogs rendered through portals.
    document.addEventListener("scroll", onScroll, { capture: true, passive: true })
    return () => {
      document.removeEventListener("scroll", onScroll, true)
      for (const [target, timer] of timers) {
        window.clearTimeout(timer)
        target.removeAttribute("data-scrolling")
      }
    }
  }, [])
}
