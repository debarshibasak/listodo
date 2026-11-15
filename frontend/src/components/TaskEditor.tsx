import { useRef, useState } from "react"
import { ArrowUp, CalendarDays, Check, Flag, FolderOpen, Loader2, Maximize2, Minimize2, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { TaskMarkdownInput } from "@/components/TaskMarkdownInput"
import { parseTaskText, taskText } from "@/lib/task-text"
import type { Task } from "@/lib/api"
import { useTaskAutosave } from "@/lib/use-task-autosave"

const priorities = ["Low", "Normal", "High", "Urgent"]

export function TaskEditor({ task, onClose, onSave }: {
  task: Task
  onClose: () => void
  onSave: (task: Task) => Promise<void>
}) {
  const [text, setText] = useState(taskText(task))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [source, setSource] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [priority, setPriority] = useState(task.priority)
  const [dueDate, setDueDate] = useState(task.dueDate)
  const autosave = useTaskAutosave(task, text, priority, dueDate, onSave)
  const closing = useRef(false)

  async function closeEditor() {
    if (busy || closing.current) return
    if (!task.id) { onClose(); return }
    closing.current = true
    setBusy(true)
    setError("")
    try {
      await autosave.flush()
      onClose()
    } catch { /* Keep the draft open and show the save error. */ }
    finally { closing.current = false; setBusy(false) }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (busy || !text.trim()) return
    if (task.id) { await closeEditor(); return }
    setError("")
    let drafts: Task[]
    try {
      drafts = parseTaskText(text, { ...task, priority, dueDate })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      return
    }
    setBusy(true)
    let saved = 0
    try {
      for (const draft of drafts) {
        await onSave(draft)
        saved++
      }
      onClose()
    } catch (e) {
      // Keep only unsaved tasks so retrying cannot duplicate the batch.
      if (saved) setText(drafts.slice(saved).map((draft) =>
        `- [${draft.status === "done" ? "x" : " "}] ${taskText(draft)}`,
      ).join("\n"))
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && void closeEditor()}>
      <DialogContent className={`task-dialog${expanded ? " task-dialog-expanded" : ""}`} positioning="layout" showCloseButton={!busy}>
        <DialogTitle className="sr-only">{task.id ? "Edit task" : "New task"}</DialogTitle>
        <DialogDescription className="sr-only">
          {task.id ? "Changes save automatically. Press Command or Control Enter to save and close." : "Write a task with Markdown. Use arrow keys to navigate, Tab to indent lists, and Command or Control Enter to save."}
        </DialogDescription>
        <Button type="button" variant="ghost" size="icon-xs" className="task-editor-expand"
          aria-label={expanded ? "Restore compact editor" : "Expand editor"}
          title={expanded ? "Restore compact editor" : "Expand editor"}
          aria-pressed={expanded} onClick={() => setExpanded(!expanded)}>
          {expanded ? <Minimize2 /> : <Maximize2 />}
        </Button>
        <form onSubmit={submit} onKeyDown={(e) => {
          if (!e.defaultPrevented && (e.metaKey || e.ctrlKey) && e.key === "Enter" && !e.nativeEvent.isComposing) {
            e.preventDefault()
            e.currentTarget.requestSubmit()
          }
        }}>
          <TaskMarkdownInput value={text} onChange={setText} disabled={busy} source={source} />
          {(error || autosave.error) && <p className="task-editor-error" role="alert">{error || autosave.error}</p>}
          {task.directory && <div className="task-editor-folder" title={task.directory}>
            <FolderOpen /><span className="sr-only">Project folder: </span><code>{task.directory}</code>
          </div>}
          <div className="task-editor-schedule">
            <label><CalendarDays /><span>Due date</span>
              <input type="date" aria-label="Due date" value={dueDate} disabled={busy}
                onChange={e => setDueDate(e.target.value)} />
            </label>
            {dueDate && <button type="button" aria-label="Remove due date" title="Remove due date" disabled={busy} onClick={() => setDueDate("")}><X /></button>}
          </div>
          <div className="task-editor-footer">
            <div className="task-editor-options">
              <span><button type="button" className="markdown-source-toggle" aria-label={source ? "Show formatted text" : "Edit Markdown"} aria-pressed={source} disabled={busy} onClick={() => setSource(!source)}>{source ? "Show formatted text" : "Markdown"}</button> <span aria-hidden="true">·</span> {task.id
                ? <span className="task-save-status" role="status" data-error={!!autosave.error}>{autosave.saving ? <><Loader2 className="animate-spin" />Saving…</> : autosave.error ? "Not saved" : autosave.dirty ? "Unsaved changes" : <><Check />Saved</>}</span>
                : <><kbd>⌘ ↵</kbd> to save</>}</span>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button type="button" variant="ghost" size="sm" className="task-priority" disabled={busy} aria-label={`Priority: ${priorities[priority]}`}>
                    <Flag />{priorities[priority]}
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  <DropdownMenuRadioGroup value={String(priority)} onValueChange={value => setPriority(Number(value))}>
                    {priorities.map((label, value) => <DropdownMenuRadioItem key={value} value={String(value)}>{label}</DropdownMenuRadioItem>)}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
            <Button type="submit" aria-label="Save task" title={task.id ? "Save and close (⌘ Enter)" : "Save task (⌘ Enter)"} disabled={busy || !text.trim()} size="icon-sm">
              {busy ? <Loader2 className="animate-spin" /> : task.id ? <Check /> : <ArrowUp />}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
