import { useEffect, useRef } from "react"
import { Button } from "@/components/ui/button"

const bayer = [0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5]

export function LoadingScreen({ error, onRetry }: { error: string; onRetry: () => void }) {
  const canvas = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const element = canvas.current
    const context = element?.getContext("2d")
    if (!element || !context) return
    const motion = window.matchMedia("(prefers-reduced-motion: reduce)")
    const appearance = window.matchMedia("(prefers-color-scheme: dark)")
    let frame = 0
    let lastFrame = -Infinity
    let foreground = getComputedStyle(element).color
    const draw = (time: number) => {
      context.clearRect(0, 0, 280, 180)
      context.fillStyle = foreground
      const phase = motion.matches || error ? 0 : time / 2600
      for (let y = 0; y < 180; y += 3) {
        for (let x = 0; x < 280; x += 3) {
          const dx = (x - 140) / 67
          const dy = (y - 90) / 67
          const radius = dx * dx + dy * dy
          if (radius > 1) continue
          const depth = Math.sqrt(1 - radius)
          const light = .4 + .28 * depth - .2 * dx - .15 * dy
          const wave = Math.sin(dx * 5 + dy * 3 - phase) * Math.cos(dy * 4 + phase * .7) * .18
          const density = Math.max(0, Math.min(1, (light + wave) * Math.min(1, (1 - radius) * 8)))
          if (density > (bayer[((y / 3) % 4) * 4 + (x / 3) % 4] + .5) / 16) context.fillRect(x, y, 2, 2)
        }
      }
    }
    const tick = (time: number) => {
      if (time - lastFrame >= 80) {
        draw(time)
        lastFrame = time
      }
      frame = requestAnimationFrame(tick)
    }
    const restart = () => {
      cancelAnimationFrame(frame)
      foreground = getComputedStyle(element).color
      draw(0)
      if (!motion.matches && !document.hidden && !error) frame = requestAnimationFrame(tick)
    }
    restart()
    motion.addEventListener("change", restart)
    appearance.addEventListener("change", restart)
    document.addEventListener("visibilitychange", restart)
    return () => {
      cancelAnimationFrame(frame)
      motion.removeEventListener("change", restart)
      appearance.removeEventListener("change", restart)
      document.removeEventListener("visibilitychange", restart)
    }
  }, [error])

  return <main className="startup-screen" aria-busy={!error}>
    <div className="startup-content">
      <canvas ref={canvas} width={280} height={180} className="startup-dither" aria-hidden="true" />
      <div className="startup-brand"><img src="/favicon.svg" alt="" /><span>Listodo</span></div>
      {error ? <>
        <p role="alert">{error}</p>
        <Button variant="outline" onClick={onRetry}>Try again</Button>
      </> : <p role="status">Loading your workspace<span aria-hidden="true">…</span></p>}
    </div>
  </main>
}
