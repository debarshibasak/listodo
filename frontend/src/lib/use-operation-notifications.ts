import { useCallback, useRef, useState } from "react"
import type { Operation } from "./api"

export function useOperationNotifications() {
  const active = useRef(new Set<string>())
  const terminal = useRef(new Map<string, Operation>())
  const announced = useRef(new Set<string>())
  const [finished, setFinished] = useState<Operation[]>([])

  const recordOperations = useCallback((operations: Operation[]) => {
    const notifications: Operation[] = []
    for (const operation of operations) {
      const key = `${operation.id}:${operation.turn || 1}`
      if (operation.status === "running") {
        if (!terminal.current.has(key)) active.current.add(key)
        continue
      }
      terminal.current.set(key, operation)
      if (active.current.delete(key) && !announced.current.has(key) &&
        (operation.status === "succeeded" || operation.status === "failed")) {
        announced.current.add(key)
        notifications.push(operation)
      }
    }
    if (notifications.length) setFinished(current => [...current.filter(previous => !notifications.some(next => next.id === previous.id)), ...notifications])
  }, [])

  const recordStarted = useCallback((operation: Operation) => {
    const key = `${operation.id}:${operation.turn || 1}`
    active.current.add(key)
    setFinished(current => current.filter(previous => previous.id !== operation.id || (previous.turn || 1) === (operation.turn || 1)))
    // A very short run may finish before the launch response reaches the UI.
    recordOperations([terminal.current.get(key) || operation])
  }, [recordOperations])
  const dismiss = useCallback((id: string) => {
    setFinished(current => current.filter(operation => operation.id !== id))
  }, [])

  return { finished, recordOperations, recordStarted, dismiss }
}
