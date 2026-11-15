import type { Task } from "./api"

export function taskText(task: Task) {
  return task.title + (task.notes ? `\n\n${task.notes}` : "")
}

/** A checklist-only draft creates tasks; a titled draft keeps its checklist in notes. */
export function parseTaskText(text: string, template: Task): Task[] {
  const lines = text.trim().split(/\r?\n/)
  const checklist = /^[-*+]\s+\[([ xX])\]\s+(.+)$/
  const batch = !template.id && checklist.test(lines[0] || "")
  const drafts: Task[] = []
  let body: string[] = []
  let fenced = false
  const flush = () => {
    if (!body.length) return
    const [first, ...rest] = body
    const match = first.match(checklist)
    drafts.push({
      ...template,
      title: (match ? match[2] : first.replace(/^#{1,6}\s+/, "")).trim(),
      notes: rest.join("\n").trim(),
      status: match ? (match[1].toLowerCase() === "x" ? "done" : template.status === "done" ? "todo" : template.status) : template.status,
    })
    body = []
  }
  for (const line of lines) {
    if (batch && !fenced && checklist.test(line)) flush()
    body.push(line)
    if (/^\s*(```|~~~)/.test(line)) fenced = !fenced
  }
  flush()
  for (const draft of drafts) {
    if (!draft.title || draft.title.length > 500)
      throw new Error("Each task needs a title of 1–500 characters.")
    if (draft.notes.length > 30000) throw new Error("Task details are too long.")
  }
  return drafts
}
