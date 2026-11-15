import { useState } from "react"
import { ArrowRight, Check, Loader2, Sparkles } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { emptyTask, request, type Proposal, type Task } from "@/lib/api"

export function Organizer({
  provider,
  tasks,
  onClose,
  onSave,
}: {
  provider: string
  tasks: Task[]
  onClose: () => void
  onSave: (task: Task) => Promise<void>
}) {
  const [input, setInput] = useState("")
  const [harness, setHarness] = useState(provider)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [proposal, setProposal] = useState<Proposal | null>(null)
  const [applied, setApplied] = useState<string[]>([])
  const [applying, setApplying] = useState<string | null>(null)
  async function run() {
    setBusy(true)
    setError("")
    try {
      setProposal(await request<Proposal>("organize", { provider: harness, instructions: input }))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  async function apply(key: string, task: Task) {
    setApplying(key)
    setError("")
    try {
      await onSave(task)
      setApplied((a) => [...a, key])
    } catch (e) {
      setError(String(e))
    } finally {
      setApplying(null)
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent className="sm:max-w-[620px] max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <div className="dialog-symbol">
            <Sparkles />
          </div>
          <DialogTitle>
            {proposal ? "Suggested tasks" : "Organize tasks"}
          </DialogTitle>
          <DialogDescription>
            {proposal
              ? proposal.summary
              : "Describe your plans to generate a task list."}
          </DialogDescription>
        </DialogHeader>
        {!proposal ? (
          <>
            <Textarea
              aria-label="Describe your plans"
              autoFocus
              className="min-h-36"
              placeholder="I need to finish the login flow, review the API changes, and write release notes by Friday…"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              disabled={busy}
            />
            <div className="flex items-center justify-between gap-4">
              <Select value={harness} onValueChange={setHarness} disabled={busy}>
                <SelectTrigger className="w-32" aria-label="Organizer provider">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="codex">Codex</SelectItem>
                  <SelectItem value="claude">Claude Code</SelectItem>
                </SelectContent>
              </Select>
              <Button onClick={run} disabled={busy}>
                {busy ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Sparkles className="size-4" />
                )}
                {busy ? "Preparing suggestions…" : "Prepare suggestions"}
              </Button>
            </div>
            <p className="text-[11px] text-muted-foreground leading-relaxed">
              Uses your installed CLI and its existing sign-in. Your task list and up to 60 of
              today’s session excerpts are sent to the selected provider. You review every change
              before applying it.
            </p>
            {busy && (
              <Button
                variant="ghost"
                onClick={() => {
                  void request("organize/cancel", {}).catch((e) => setError(String(e)))
                }}
              >
                Cancel organizer
              </Button>
            )}
          </>
        ) : (
          <div className="space-y-3">
            {(proposal.tasks || []).map((draft, i) => {
              const key = `task-${i}`
              return (
                <div key={key} className="suggestion-row">
                  <div className="min-w-0">
                    <h3>{draft.title}</h3>
                    {draft.notes && <p>{draft.notes}</p>}
                    <div className="flex gap-2 mt-2">
                      {draft.project && <Badge variant="secondary">{draft.project}</Badge>}
                      {draft.dueDate && <Badge variant="outline">{draft.dueDate}</Badge>}
                    </div>
                  </div>
                  <Button
                    size="sm"
                    variant={applied.includes(key) ? "ghost" : "outline"}
                    disabled={applied.includes(key) || applying !== null}
                    onClick={() => apply(key, { ...emptyTask(), ...draft })}
                  >
                    {applied.includes(key) ? (
                      <>
                        <Check />
                        Added
                      </>
                    ) : (
                      "Add task"
                    )}
                  </Button>
                </div>
              )
            })}
            {(proposal.matches || []).map((match, i) => {
              const task = tasks.find((t) => t.id === match.taskId)
              const key = `match-${i}`
              return (
                task && (
                  <div key={key} className="suggestion-row">
                    <div>
                      <h3>{task.title}</h3>
                      <p>{match.reason}</p>
                      <Badge variant="secondary" className="mt-2">
                        {match.suggestedStatus === "working"
                          ? "In progress"
                          : match.suggestedStatus === "done"
                            ? "Completed"
                            : "To do"}
                      </Badge>
                    </div>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={applied.includes(key) || applying !== null}
                      onClick={() =>
                        apply(key, {
                          ...task,
                          status: match.suggestedStatus,
                          sessionIds: [...new Set([...(task.sessionIds || []), match.sessionId])],
                        })
                      }
                    >
                      {applied.includes(key) ? <Check /> : <ArrowRight />}
                      {applied.includes(key) ? "Applied" : "Apply"}
                    </Button>
                  </div>
                )
              )
            })}
            <Button onClick={onClose} className="w-full mt-3">
              Done
            </Button>
          </div>
        )}
        {error && (
          <p role="alert" className="text-xs text-destructive whitespace-pre-wrap">
            {error}
          </p>
        )}
      </DialogContent>
    </Dialog>
  )
}
