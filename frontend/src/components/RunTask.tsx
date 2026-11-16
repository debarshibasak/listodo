import { useState } from "react"
import { FolderOpen, Loader2, Play } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { request, type Task, type SessionFolder, type Operation, type AgentProvider } from "@/lib/api"

export const agentName = (provider: AgentProvider) => provider === "codex" ? "Codex" : "Claude Code"

export function RunTask({ task, provider, sessions, operations, onClose, onStarted }: {
  task: Task
  provider: AgentProvider
  sessions: SessionFolder[]
  operations: Operation[]
  onClose: () => void
  onStarted: (operation: Operation) => void
}) {
  const previous = operations.find((op) => op.taskId === task.id)
  const linked = sessions.find((session) => task.sessionIds?.includes(session.id) && session.directory)
  const project = sessions.find((session) => task.project && session.project === task.project && session.directory)
  const [directory, setDirectory] = useState(task.directory || previous?.directory || linked?.directory || project?.directory || "")
  const [busy, setBusy] = useState(false)
  const [choosing, setChoosing] = useState(false)
  const [error, setError] = useState("")
  const folders = [...new Set([task.directory, ...operations.map(op => op.directory), ...sessions.map(session => session.directory)].filter(Boolean))]
  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    setError("")
    try {
      const operation = await request<Operation>("operations", { taskId: task.id, provider, directory: directory.trim() })
      onStarted(operation)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }
  return (
    <Dialog open onOpenChange={open => !open && !busy && !choosing && onClose()}>
      <DialogContent className="run-dialog" showCloseButton={!busy && !choosing}>
        <DialogHeader>
          <DialogTitle>Run with {agentName(provider)}</DialogTitle>
          <DialogDescription>{task.title}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="agent-directory">Project folder</Label>
            <div className="flex gap-2">
              <Input id="agent-directory" autoFocus required placeholder="/path/to/your/project"
                list="agent-folders" value={directory} disabled={busy || choosing}
                onChange={e => setDirectory(e.target.value)} />
              <datalist id="agent-folders">{folders.map(folder => <option value={folder} key={folder} />)}</datalist>
              {navigator.userAgent.includes("Listodo/") && <Button type="button" variant="outline" aria-label="Choose project folder"
                disabled={busy || choosing} onClick={async () => {
                  setChoosing(true)
                  try {
                    const result = await request<{ path: string }>("folders/choose", {})
                    if (result.path) setDirectory(result.path)
                  } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
                  finally { setChoosing(false) }
                }}><FolderOpen /></Button>}
            </div>
            <p className="text-xs text-muted-foreground leading-relaxed">
              {agentName(provider)} will run this task and can edit files in this folder, using your CLI sign-in and permissions.
            </p>
          </div>
          {task.notes && <details className="run-task-notes"><summary>Task details</summary><p>{task.notes}</p></details>}
          {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose} disabled={busy || choosing}>Cancel</Button>
            <Button type="submit" disabled={busy || choosing || !directory.trim()}>
              {busy ? <Loader2 className="animate-spin" /> : <Play />} {busy ? "Starting…" : "Run task"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
