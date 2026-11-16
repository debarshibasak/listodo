export type Status = "todo" | "working" | "done"
export type AgentProvider = "codex" | "claude"
export type Operation = {
  id: string
  turn?: number
  taskId: string
  title: string
  prompt?: string
  provider: AgentProvider
  directory: string
  status: "running" | "succeeded" | "failed" | "cancelled" | "interrupted"
  sessionId: string
  output?: string
  error?: string
  startedAt: string
  updatedAt: string
  finishedAt?: string
}
export type Task = {
  id: string
  title: string
  notes: string
  project: string
  directory: string
  startDate: string
  dueDate: string
  priority: number
  status: Status
  order: number
  createdAt: string
  updatedAt: string
  completedAt: string | null
  sessionIds: string[] | null
}
export type Session = {
  id: string
  harness: string
  title: string
  project: string
  directory: string
  summary: string
  updatedAt: string
  startedAt: string
  status: string
}
export type Settings = {
  harness: string
  codexRoot: string
  claudeRoot: string
  codexEnabled: boolean
  claudeEnabled: boolean
  autoSync: boolean
}
export type TaskEvent = { ID: number; TaskID: string; Title: string; Action: string; At: string }
export type Snapshot = {
  tasks: Task[]
  sessions: Session[]
  settings: Settings
  events: TaskEvent[]
  database: string
}
export type SessionFolder = Pick<Session, "id" | "project" | "directory">
export type Workspace = Omit<Snapshot, "sessions" | "events"> & { sessions: SessionFolder[] }
export type Draft = {
  title: string
  notes: string
  project: string
  dueDate: string
  priority: number
}
export type Match = { taskId: string; sessionId: string; reason: string; suggestedStatus: Status }
export type Proposal = { summary: string; tasks: Draft[]; matches: Match[] }

let taskWrites: Promise<unknown> = Promise.resolve()

export function request<T>(path: string, body?: unknown, method?: string): Promise<T> {
  // Undo must follow any task write already in flight, including autosaves.
  if ((path === "tasks" || path.startsWith("tasks/")) && (method ?? (body === undefined ? "GET" : "POST")) !== "GET") {
    const pending = taskWrites.then(() => sendRequest<T>(path, body, method))
    taskWrites = pending.catch(() => {})
    return pending
  }
  return sendRequest<T>(path, body, method)
}

async function sendRequest<T>(path: string, body?: unknown, method?: string): Promise<T> {
  const token = document.querySelector<HTMLMetaElement>('meta[name="listodo-token"]')?.content ?? ""
  const response = await fetch(`/api/${path}`, {
    method: method ?? (body === undefined ? "GET" : "POST"),
    headers: { "Content-Type": "application/json", "X-Listodo-Token": token },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok) {
    const data = await response
      .json()
      .catch(() => ({ error: "Could not connect to Listodo. Please restart the app." }))
    throw new Error(data.error || "Something went wrong. Please try again.")
  }
  return response.json()
}

export function day(date = new Date()) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`
}
export function overdueDays(task: Task) {
  if (task.status === "done" || !task.dueDate) return 0
  return Math.max(0, Math.round((Date.parse(day()) - Date.parse(task.dueDate)) / 86_400_000))
}
export function priority(task: Task) {
  return Math.min(3, task.priority + overdueDays(task))
}
export function isToday(task: Task) {
  return task.status === "done"
    ? !!task.completedAt && day(new Date(task.completedAt)) === day()
    : task.status === "working" || (task.dueDate ? task.dueDate <= day() : !!task.createdAt && startDate(task) <= day())
}
// The server owns the start date. The fallback supports older saved tasks.
export function startDate(task: Task) {
  return task.startDate || task.createdAt.slice(0, 10)
}
export function isActive(session: Session) {
  const elapsed = Date.now() - Date.parse(session.updatedAt)
  return session.status === "working" && elapsed >= 0 && elapsed < 180_000
}
export function emptyTask(project = ""): Task {
  return {
    id: "",
    title: "",
    notes: "",
    project,
    directory: "",
    startDate: "",
    dueDate: "",
    priority: 1,
    status: "todo",
    order: 0,
    createdAt: "",
    updatedAt: "",
    completedAt: null,
    sessionIds: [],
  }
}
export function relativeTime(raw: string) {
  const minutes = Math.max(0, Math.floor((Date.now() - Date.parse(raw)) / 60000))
  if (minutes < 1) return "Just now"
  if (minutes < 60) return `${minutes}m ago`
  if (minutes < 1440) return `${Math.floor(minutes / 60)}h ago`
  return new Date(raw).toLocaleDateString(undefined, { month: "short", day: "numeric" })
}
