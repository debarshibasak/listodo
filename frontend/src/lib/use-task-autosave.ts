import { useCallback, useEffect, useRef, useState } from "react"
import type { Task } from "./api"
import { parseTaskText, taskText } from "./task-text"

function draft(text: string, priority: number, task: Task) {
  if (!text.trim()) throw new Error("Give your task a title before saving.")
  const taskDraft = parseTaskText(text, { ...task, priority })[0]
  return { task: taskDraft, key: JSON.stringify([taskDraft.title, taskDraft.notes, taskDraft.priority, taskDraft.status, taskDraft.dueDate]) }
}

export function useTaskAutosave(task: Task, text: string, priority: number, dueDate: string, onSave: (task: Task) => Promise<void>) {
  const latest = useRef({ task: { ...task, dueDate }, text, priority, onSave })
  latest.current = { task: { ...task, dueDate }, text, priority, onSave }
  const [initialKey] = useState(() => task.id ? draft(taskText(task), task.priority, task).key : "")
  const saved = useRef(initialKey)
  const pending = useRef<Promise<void> | null>(null)
  const timer = useRef<number | undefined>(undefined)
  const mounted = useRef(true)
  const [savedKey, setSavedKey] = useState(initialKey)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState("")

  const flush = useCallback(async () => {
    window.clearTimeout(timer.current)
    if (!latest.current.task.id) return
    try {
      // Serial writes prevent an older request from overwriting newer typing.
      for (;;) {
        while (pending.current) await pending.current
        const current = latest.current
        const snapshot = draft(current.text, current.priority, current.task)
        if (snapshot.key === saved.current) {
          if (mounted.current) setError("")
          return
        }
        if (mounted.current) { setSaving(true); setError("") }
        const job = current.onSave(snapshot.task).then(() => {
          saved.current = snapshot.key
          if (mounted.current) setSavedKey(snapshot.key)
        })
        pending.current = job
        try { await job }
        finally {
          pending.current = null
          if (mounted.current) setSaving(false)
        }
        // Include edits made while this request was in flight.
      }
    } catch (e) {
      if (mounted.current) setError(e instanceof Error ? e.message : String(e))
      throw e
    }
  }, [])

  let currentKey: string | null = null
  try { currentKey = draft(text, priority, { ...task, dueDate }).key } catch { /* Keep incomplete edits local. */ }
  const dirty = currentKey !== savedKey
  useEffect(() => {
    if (!task.id || currentKey === null) return
    if (currentKey === saved.current) { setError(""); return }
    timer.current = window.setTimeout(() => { void flush().catch(() => {}) }, 600)
    return () => window.clearTimeout(timer.current)
  }, [task.id, text, priority, dueDate, currentKey, flush])
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      // Also flush if another action replaces the editor before the timer fires.
      void flush().catch(() => {})
    }
  }, [flush])

  return { saving, dirty, error, flush }
}
