import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  ArrowDownToLine,
  ArrowRight,
  CalendarDays,
  ChevronRight,
  Circle,
  CircleCheck,
  Command,
  Ellipsis,
  Flag,
  Folder,
  GripVertical,
  Layers3,
  List,
  Link2,
  Loader2,
  LockKeyhole,
  PanelLeft,
  Pause,
  Play,
  Plus,
  Search,
  Settings2,
  SlidersHorizontal,
  Sparkles,
  Sun,
  Terminal,
  X,
  type LucideIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import { TaskCalendar } from "@/components/TaskCalendar"
import { Organizer } from "@/components/Organizer"
import { Operations } from "@/components/Operations"
import { RunTask, agentName } from "@/components/RunTask"
import { LoadingScreen } from "@/components/LoadingScreen"
import { cn } from "@/lib/utils"
import { parseTaskText } from "@/lib/task-text"
import { useOperationNotifications } from "@/lib/use-operation-notifications"
import { useScrollVisibility } from "@/lib/use-scroll-visibility"
import { usePageVisible } from "@/lib/use-page-visible"
import {
  day,
  emptyTask,
  isToday,
  overdueDays,
  priority,
  startDate,
  request,
  type Settings,
  type Workspace,
  type Status,
  type Task,
  type AgentProvider,
  type Operation,
} from "@/lib/api"

const TaskEditor = lazy(() => import("@/components/TaskEditor").then(module => ({ default: module.TaskEditor })))

type Page = "Today" | "All tasks" | "Upcoming" | "Completed" | "View operations" | "Integrations"
const navItems: { name: Page; icon: LucideIcon }[] = [
  { name: "Today", icon: Sun },
  { name: "All tasks", icon: Layers3 },
  { name: "Upcoming", icon: CalendarDays },
  { name: "Completed", icon: CircleCheck },
  { name: "View operations", icon: Terminal },
]
const initialSettings: Settings = {
  harness: "codex",
  codexRoot: "",
  claudeRoot: "",
  codexEnabled: true,
  claudeEnabled: true,
  autoSync: true,
}
const initialState: Workspace = {
  tasks: [],
  sessions: [],
  settings: initialSettings,
  database: "",
}

function IconButton({
  label,
  icon: Icon,
  onClick,
  className,
  disabled,
}: {
  label: string
  icon: LucideIcon
  onClick: () => void
  className?: string
  disabled?: boolean
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className={cn("size-8 text-muted-foreground", className)}
          aria-label={label}
          disabled={disabled}
          onClick={onClick}
        >
          <Icon className="size-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
function HarnessIcon({ harness }: { harness: string }) {
  return (
    <span className={cn("harness-icon", harness === "claude" && "claude")} aria-label={harness}>
      {harness === "claude" ? <Sparkles /> : <Command />}
    </span>
  )
}

export default function App() {
  useScrollVisibility()
  const pageVisible = usePageVisible()
  const [data, setData] = useState(initialState)
  const [loaded, setLoaded] = useState(false)
  const [loadError, setLoadError] = useState("")
  const [page, setPage] = useState<Page>("Today")
  const [project, setProject] = useState("")
  const [query, setQuery] = useState("")
  const [filter, setFilter] = useState("all")
  const [sort, setSort] = useState("manual")
  const [upcomingView, setUpcomingView] = useState("calendar")
  const [planningDate, setPlanningDate] = useState(() => day())
  const [draggedTask, setDraggedTask] = useState<string | null>(null)
  const [dropTarget, setDropTarget] = useState<{ id: string; before: boolean } | null>(null)
  const [reordering, setReordering] = useState(false)
  const [quick, setQuick] = useState("")
  const [quickBusy, setQuickBusy] = useState(false)
  const [editor, setEditor] = useState<Task | null>(null)
  const [organizer, setOrganizer] = useState(false)
  const [runTask, setRunTask] = useState<{ task: Task; provider: AgentProvider } | null>(null)
  const [operations, setOperations] = useState<Operation[]>([])
  const [operationsError, setOperationsError] = useState("")
  const [selectedOperation, setSelectedOperation] = useState<string | null>(null)
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const [notification, setNotification] = useState<{ text: string; error?: boolean } | null>(null)
  const [today, setToday] = useState(() => day())
  const searchRef = useRef<HTMLInputElement>(null)
  const syncingRef = useRef(false)
  const lastSyncRef = useRef(0)
  const operationsRequest = useRef<Promise<void> | null>(null)
  const runNotifications = useOperationNotifications()
  const { recordOperations, recordStarted } = runNotifications

  const notify = useCallback((text: string, error = false) => setNotification({ text, error }), [])
  const refreshOperations = useCallback(() => {
    if (operationsRequest.current) return operationsRequest.current
    const pending = request<Operation[]>("operations").then(result => {
      recordOperations(result)
      // Output progress changes updatedAt, but only the detail view displays it.
      setOperations(current => current.length === result.length && current.every((op, i) =>
        op.id === result[i].id && op.status === result[i].status && op.turn === result[i].turn &&
        op.sessionId === result[i].sessionId && op.error === result[i].error,
      ) ? current : result)
      setOperationsError("")
    }).catch(e => {
      setOperationsError(e instanceof Error ? e.message : String(e))
    }).finally(() => { operationsRequest.current = null })
    operationsRequest.current = pending
    return pending
  }, [recordOperations])
  const hasRunningOperations = operations.some(op => op.status === "running")
  useEffect(() => {
    if (!pageVisible) return
    let active = true
    let timer: number | undefined
    const poll = async () => {
      await refreshOperations()
      if (active && hasRunningOperations) timer = window.setTimeout(poll, 2000)
    }
    void poll()
    return () => { active = false; window.clearTimeout(timer) }
  }, [page, hasRunningOperations, refreshOperations, pageVisible])
  const reload = useCallback(async () => {
    const result = await request<Workspace>("workspace")
    setData(result)
    setLoaded(true)
    setLoadError("")
    setToday(day())
    return result
  }, [])
  const sync = useCallback(
    async (silent = false) => {
      if (syncingRef.current) return
      syncingRef.current = true
      try {
        const result = await request<{ count: number; updated: number; warnings: string[] | null }>("sync", {})
        if (result.updated) await reload()
        lastSyncRef.current = Date.now()
        if (!silent)
          notify(
            result.warnings?.length
              ? `Imported ${result.count} sessions. ${result.warnings.join(" ")}`
              : `${result.count} local sessions synced.`,
          )
      } catch (e) {
        if (!silent) notify(e instanceof Error ? e.message : String(e), true)
      } finally {
        syncingRef.current = false
      }
    },
    [reload, notify],
  )

  useEffect(() => {
    let active = true
    reload()
      .then((result) => {
        if (active && !document.hidden && result.settings.autoSync) void sync(true)
      })
      .catch((e) => {
        if (active) setLoadError(String(e.message || e))
      })
    return () => {
      active = false
    }
  }, [reload, sync])
  useEffect(() => {
    if (!pageVisible) return
    const tick = () => {
      setToday(day())
      if (loaded && data.settings.autoSync && page !== "Integrations" && Date.now() - lastSyncRef.current >= 60000) void sync(true)
    }
    tick()
    const id = window.setInterval(tick, 60000)
    return () => window.clearInterval(id)
  }, [loaded, data.settings.autoSync, page, sync, pageVisible])
  useEffect(() => {
    if (!notification) return
    const id = window.setTimeout(() => setNotification(null), notification.error ? 12000 : 5000)
    return () => window.clearTimeout(id)
  }, [notification])
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey) {
        if (e.key.toLowerCase() === "k" || e.key.toLowerCase() === "f") {
          e.preventDefault()
          searchRef.current?.focus()
        }
        if (e.key.toLowerCase() === "n") {
          e.preventDefault()
          setEditor({ ...emptyTask(project), dueDate: page === "Upcoming" && upcomingView === "calendar" ? planningDate : "" })
        }
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [project, page, upcomingView, planningDate])

  function taskDraft() {
    return { ...emptyTask(project), dueDate: page === "Upcoming" && upcomingView === "calendar" ? planningDate : "" }
  }
  function navigate(next: Page, nextProject = "") {
    setPage(next)
    setProject(nextProject)
    setFilter("all")
    setQuery("")
    setSelectedOperation(null)
  }
  function operationStarted(operation: Operation) {
    // Output is owned by the selected detail view, never the app-wide history.
    const { output: _output, prompt: _prompt, ...summary } = operation
    recordStarted(summary)
    setRunTask(null)
    setOperations(current => [summary, ...current.filter(op => op.id !== operation.id)])
    navigate("View operations")
    setSelectedOperation(operation.id)
    void reload().catch(() => notify("Agent started. Refresh the task list to see its status."))
  }
  useEffect(() => {
    const openOperation = () => {
      const id = new URLSearchParams(window.location.hash.slice(1)).get("operation")
      if (!id) return
      navigate("View operations")
      setSelectedOperation(id)
      runNotifications.dismiss(id)
      window.history.replaceState(null, "", window.location.pathname + window.location.search)
    }
    window.addEventListener("hashchange", openOperation)
    openOperation()
    return () => window.removeEventListener("hashchange", openOperation)
  }, [runNotifications.dismiss])
  async function saveTask(task: Task) {
    const saved = await request<Task>("tasks", task)
    setData((current) => ({
      ...current,
      tasks: [...current.tasks.filter((t) => t.id !== saved.id), saved],
    }))
    try {
      await reload()
    } catch {
      // The write succeeded: a refresh failure must not cause a duplicate on retry.
      notify("Task saved. Could not refresh tasks; try reloading.", true)
    }
  }
  async function updateTask(task: Task) {
    try {
      await saveTask(task)
    } catch (e) {
      notify(e instanceof Error ? e.message : String(e), true)
    }
  }
  async function scheduleTask(task: Task, dueDate: string) {
    const saved = await request<Task>(`tasks/${task.id}`, {
      title: task.title, notes: task.notes, priority: task.priority, dueDate,
    }, "PATCH")
    setData(current => ({ ...current, tasks: current.tasks.map(existing => existing.id === saved.id ? saved : existing) }))
  }
  async function deleteTask(id: string) {
    await request(`tasks/${id}`, undefined, "DELETE")
    await reload()
    notify("Task deleted.")
  }
  async function reorderTasks(ids: string[]) {
    if (reordering) return
    setReordering(true)
    try {
      await request("tasks/reorder", { taskIds: ids })
      await reload()
      setSort("manual")
    } catch (e) {
      notify(e instanceof Error ? e.message : String(e), true)
    } finally {
      setReordering(false)
      setDraggedTask(null)
      setDropTarget(null)
    }
  }
  function moveTask(ids: string[], source: string, target: string, before: boolean) {
    if (source === target || !ids.includes(source)) return
    const next = ids.filter(id => id !== source)
    next.splice(next.indexOf(target) + (before ? 0 : 1), 0, source)
    void reorderTasks(next)
  }
  async function addQuick(e: React.FormEvent) {
    e.preventDefault()
    if (!quick.trim() || quickBusy) return
    setQuickBusy(true)
    try {
      const [draft] = parseTaskText(quick, emptyTask(project))
      await saveTask(draft)
      setQuick("")
      setFilter("all")
      if (page === "Upcoming") navigate("Today")
    } catch (e) {
      notify(e instanceof Error ? e.message : String(e), true)
    } finally {
      setQuickBusy(false)
    }
  }
  async function exportBackup() {
    try {
      // A native save panel is used inside WKWebView; browsers use a download.
      if (navigator.userAgent.includes("Listodo/")) {
        const result = await request<{ saved: boolean }>("export/save", {})
        if (result.saved) notify("Backup saved.")
      } else {
        const backup = await request("export")
        const url = URL.createObjectURL(
          new Blob([JSON.stringify(backup, null, 2)], { type: "application/json" }),
        )
        const anchor = document.createElement("a")
        anchor.href = url
        anchor.download = `listodo-backup-${day()}.json`
        anchor.click()
        window.setTimeout(() => URL.revokeObjectURL(url), 1000)
        notify("Backup exported.")
      }
    } catch (e) {
      notify(String(e), true)
    }
  }

  const { remaining, working, done, openCount, projects, projectCounts } = useMemo(() => {
    let remaining = 0, working = 0, done = 0, openCount = 0
    const projectCounts = new Map<string, number>()
    for (const task of data.tasks) {
      if (task.status !== "done") {
        openCount++
        if (task.project) projectCounts.set(task.project, (projectCounts.get(task.project) || 0) + 1)
      }
      if (!isToday(task)) continue
      if (task.status === "done") done++
      else remaining++
      if (task.status === "working") working++
    }
    return { remaining, working, done, openCount, projectCounts, projects: [...projectCounts.keys()].sort() }
  }, [data.tasks, today])
  const taskPages = !["View operations", "Integrations"].includes(page)
  const title = project || page
  const calendarView = page === "Upcoming" && upcomingView === "calendar"
  const { scopedTasks, visible, groups } = useMemo(() => {
    const search = query.toLowerCase()
    const scopedTasks = data.tasks.filter((t) => {
      if (project && t.project !== project) return false
      if (search && !`${t.title} ${t.notes} ${t.project}`.toLowerCase().includes(search))
        return false
      if (page === "Today" && !isToday(t)) return false
      if (page === "Upcoming" && !calendarView && (t.status === "done" || !t.dueDate || t.dueDate <= day()))
        return false
      if (page === "Completed" && t.status !== "done") return false
      return true
    })
    const visible = scopedTasks
      .filter((t) => t.status !== "done" || page === "Completed" || filter === "done")
      .filter((t) => filter === "all" || t.status === filter)
      .sort((a, b) => {
        if ((a.status === "done") !== (b.status === "done")) return a.status === "done" ? 1 : -1
        if (sort === "manual") return (a.order || 0) - (b.order || 0) || a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id)
        if (sort === "newest") return Date.parse(b.createdAt) - Date.parse(a.createdAt)
        if (sort === "due") return (a.dueDate || "9999").localeCompare(b.dueDate || "9999")
        return priority(b) - priority(a) || a.createdAt.localeCompare(b.createdAt)
      })
    const groups = visible.reduce<Record<string, Task[]>>((all, task) => {
      const group =
        task.status === "done"
          ? "Completed"
          : overdueDays(task) > 0
            ? "Needs attention"
            : task.status === "working"
              ? "In progress"
              : "To do"
      ;(all[group] ||= []).push(task)
      return all
    }, {})
    return { scopedTasks, visible, groups }
  }, [data.tasks, project, query, page, calendarView, filter, sort, today])
  const groupNames = ["Needs attention", "In progress", "To do", "Completed"]

  if (!loaded) return <LoadingScreen error={loadError} onRetry={() => {
    setLoadError("")
    void reload().catch(e => setLoadError(String(e)))
  }} />

  return (
    <TooltipProvider delayDuration={300}>
      <div className={cn("app-shell", !sidebarOpen && "sidebar-collapsed")}>
        <aside className="sidebar" aria-label="Sidebar">
          <div className="brand">
            <img src="/favicon.svg" alt="" className="brand-mark" />
            <span>Listodo</span>
          </div>
          <div className="workspace-label">Personal workspace</div>
          <nav className="navigation" aria-label="Main navigation">
            {navItems.map(({ name, icon: Icon }) => {
              const count =
                name === "Today"
                  ? remaining
                  : name === "All tasks"
                    ? openCount
                    : name === "View operations"
                      ? operations.filter(op => op.status === "running").length || null
                      : null
              return (
                <Button
                  variant="ghost"
                  key={name}
                  className={cn("nav-item", page === name && !project && "is-active")}
                  onClick={() => navigate(name)}
                  aria-current={page === name && !project ? "page" : undefined}
                >
                  <Icon />
                  <span>{name}</span>
                  {count !== null && <span className="nav-count">{count}</span>}
                </Button>
              )
            })}
          </nav>
          {projects.length > 0 && <>
            <div className="sidebar-section-label project-label">Projects</div>
            <nav className="navigation project-nav" aria-label="Projects">
              {projects.map((name) => (
                <Button variant="ghost" key={name}
                  className={cn("nav-item", project === name && "is-active")}
                  onClick={() => navigate("All tasks", name)}>
                  <Folder />
                  <span className="truncate">{name}</span>
                  <span className="nav-count">
                    {projectCounts.get(name)}
                  </span>
                </Button>
              ))}
            </nav>
          </>}
          <div className="sidebar-bottom">
            <Button
              variant="ghost"
              className={cn("nav-item", page === "Integrations" && "is-active")}
              onClick={() => navigate("Integrations")}
            >
              <Settings2 />
              <span>Integrations</span>
            </Button>
            <Button variant="ghost" className="nav-item" onClick={exportBackup}>
              <ArrowDownToLine />
              <span>Export backup</span>
            </Button>
            <Separator className="my-4" />
            <div className="local-status"><LockKeyhole /> Saved on this device</div>
          </div>
        </aside>

        <div className="workspace-main">
          <header className="toolbar">
            <div className="breadcrumb">
              <IconButton
                label={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
                icon={PanelLeft}
                onClick={() => setSidebarOpen((v) => !v)}
              />
              <span className="breadcrumb-workspace">Workspace</span>
              <ChevronRight className="breadcrumb-chevron" />
              <span>{title}</span>
            </div>
            <div className="toolbar-actions">
              <div className="search-control">
                <Search />
                <Input
                  ref={searchRef}
                  aria-label="Search tasks or operations"
                  placeholder="Search…"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
                <kbd>⌘ K</kbd>
                {query && (
                  <button aria-label="Clear search" onClick={() => setQuery("")}>
                    <X />
                  </button>
                )}
              </div>
              <Button
                variant="outline"
                className="organize-button"
                onClick={() => setOrganizer(true)}
              >
                <Sparkles />
                Organize
              </Button>
              <Button onClick={() => setEditor(taskDraft())} className="new-task-button">
                <Plus />
                New task
              </Button>
            </div>
          </header>

            <div className="content-layout">
              <main className={cn("main-content", taskPages ? "tasks-content" : "details-content", calendarView && "calendar-content")} key={`${page}-${project}`}>
                <div className="page-heading">
                  <div>
                    <h1>{title}</h1>
                    {page === "Today" && <p>
                      {new Date(`${today}T12:00:00`).toLocaleDateString("en-US", {
                        weekday: "long", month: "long", day: "numeric",
                      })}
                    </p>}
                  </div>
                  {page === "Today" && remaining + done > 0 && (
                    <div className="day-summary">
                      <span>{remaining} open</span><span>{working} in progress</span><span>{done} done</span>
                    </div>
                  )}
                </div>

                {taskPages && (
                  <>
                    <div className="task-list-toolbar">
                      <Tabs value={filter} onValueChange={setFilter}>
                        <TabsList>
                          <TabsTrigger value="all">
                            All tasks<span>{scopedTasks.filter(task => (page === "Completed" || task.status !== "done") && (!calendarView || task.dueDate.slice(0, 7) === planningDate.slice(0, 7))).length}</span>
                          </TabsTrigger>
                          <TabsTrigger value="todo">To do</TabsTrigger>
                          <TabsTrigger value="working">In progress</TabsTrigger>
                          <TabsTrigger value="done">Done</TabsTrigger>
                        </TabsList>
                      </Tabs>
                      <div className="task-view-options">
                      {page === "Upcoming" && <div className="task-view-toggle" role="group" aria-label="Upcoming view">
                        <Button variant="ghost" size="sm" aria-label="List view" aria-pressed={!calendarView} onClick={() => setUpcomingView("list")}><List /><span>List</span></Button>
                        <Button variant="ghost" size="sm" aria-label="Calendar view" aria-pressed={calendarView} onClick={() => setUpcomingView("calendar")}><CalendarDays /><span>Calendar</span></Button>
                      </div>}
                      {!calendarView && <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="sm" className="display-button">
                            <SlidersHorizontal />
                            Display
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuLabel>Sort tasks by</DropdownMenuLabel>
                          <DropdownMenuSeparator />
                          <DropdownMenuRadioGroup value={sort} onValueChange={setSort}>
                            <DropdownMenuRadioItem value="manual">Manual order</DropdownMenuRadioItem>
                            <DropdownMenuRadioItem value="priority">Priority</DropdownMenuRadioItem>
                            <DropdownMenuRadioItem value="due">Due date</DropdownMenuRadioItem>
                            <DropdownMenuRadioItem value="newest">
                              Newest first
                            </DropdownMenuRadioItem>
                          </DropdownMenuRadioGroup>
                        </DropdownMenuContent>
                      </DropdownMenu>}
                      </div>
                    </div>
                    {calendarView ? <TaskCalendar tasks={visible} selectedDate={planningDate} onSelectDate={setPlanningDate}
                      onCreate={dueDate => setEditor({ ...emptyTask(project), dueDate })} onEdit={setEditor} onSchedule={scheduleTask} /> : <>
                    {page !== "Completed" && (
                      <form onSubmit={addQuick} className="quick-add">
                        <Plus />
                        <Input
                          aria-label="Quick add task"
                          placeholder="Add a task…"
                          value={quick}
                          maxLength={500}
                          disabled={quickBusy}
                          onChange={(e) => setQuick(e.target.value)}
                          onPaste={(e) => {
                            const text = e.clipboardData.getData("text")
                            if (/\r?\n/.test(text)) {
                              e.preventDefault()
                              setEditor({ ...emptyTask(project), title: text })
                            }
                          }}
                        />
                        {quickBusy ? (
                          <Loader2 className="animate-spin" />
                        ) : (
                          <button type="submit" aria-label="Add task">
                            <span>Enter</span>
                            <span>↵</span>
                          </button>
                        )}
                      </form>
                    )}
                    {visible.length ? (
                      <div className="task-groups" aria-label="Task list" role="region">
                        {groupNames
                          .filter((name) => groups[name]?.length)
                          .map((name) => (
                            <section className="task-group" key={name}>
                              <h2>
                                <span
                                  className={cn(
                                    "group-indicator",
                                    name === "Needs attention" && "attention",
                                    name === "In progress" && "working",
                                    name === "Completed" && "done",
                                  )}
                                />
                                {name}
                                <span className="group-count">{groups[name].length}</span>
                              </h2>
                              <div className="task-rows">
                                {groups[name].map((task) => (
                                  <TaskRow
                                    key={task.id}
                                    task={task}
                                    onEdit={() => setEditor(task)}
                                    onRun={(provider) => setRunTask({ task, provider })}
                                    running={operations.some(op => op.taskId === task.id && op.status === "running")}
                                    onViewOperation={() => {
                                      const op = operations.find(op => op.taskId === task.id)
                                      if (op) { navigate("View operations"); setSelectedOperation(op.id) }
                                    }}
                                    reordering={reordering}
                                    dragging={draggedTask === task.id}
                                    dropPosition={dropTarget?.id === task.id ? (dropTarget.before ? "before" : "after") : null}
                                    onDragStart={(e) => {
                                      e.dataTransfer.setData("application/x-listodo-task", task.id)
                                      e.dataTransfer.effectAllowed = "move"
                                      const row = e.currentTarget.closest(".task-row")
                                      if (row) e.dataTransfer.setDragImage(row, 20, 20)
                                      setDraggedTask(task.id)
                                    }}
                                    onDragEnd={() => { setDraggedTask(null); setDropTarget(null) }}
                                    onDragOver={(e) => {
                                      if (reordering || !draggedTask || !groups[name].some(t => t.id === draggedTask)) return
                                      e.preventDefault()
                                      e.dataTransfer.dropEffect = "move"
                                      const bounds = e.currentTarget.getBoundingClientRect()
                                      setDropTarget({ id: task.id, before: e.clientY < bounds.top + bounds.height / 2 })
                                    }}
                                    onDrop={(e) => {
                                      e.preventDefault()
                                      if (draggedTask && dropTarget) moveTask(groups[name].map(t => t.id), draggedTask, task.id, dropTarget.before)
                                      setDraggedTask(null)
                                      setDropTarget(null)
                                    }}
                                    onMove={(direction) => {
                                      const ids = groups[name].map(t => t.id)
                                      const index = ids.indexOf(task.id)
                                      const target = ids[index + direction]
                                      if (target) moveTask(ids, task.id, target, direction < 0)
                                    }}
                                    onUpdate={updateTask}
                                    onDelete={() =>
                                      deleteTask(task.id).catch((e) => notify(String(e), true))
                                    }
                                  />
                                ))}
                              </div>
                            </section>
                          ))}
                      </div>
                    ) : (
                      <div className="empty-state">
                        <h2>
                          {query ? "No matching tasks" : filter !== "all" ? "No tasks in this view"
                            : page === "Completed" ? "No completed tasks"
                            : page === "Upcoming" ? "Nothing scheduled" : scopedTasks.length ? "No open tasks" : "No tasks yet"}
                        </h2>
                        <p>
                          {query ? "Try a different search." : filter !== "all" ? "Try another filter."
                            : page === "Completed" ? "Completed tasks will appear here."
                            : page === "Upcoming" ? "Tasks with a future due date will appear here."
                            : scopedTasks.length ? "Your finished tasks are in Completed."
                            : "Write a task above, or paste a Markdown checklist."}
                        </p>
                        <div className="empty-actions">
                          {query || filter !== "all" ? (
                            <Button
                              variant="outline"
                              onClick={() => {
                                setQuery("")
                                setFilter("all")
                              }}
                            >
                              Clear filters
                            </Button>
                          ) : page === "Completed" ? (
                            <Button variant="outline" onClick={() => navigate("All tasks")}>
                              Back to your tasks
                              <ArrowRight />
                            </Button>
                          ) : scopedTasks.length > 0 ? (
                            <Button variant="outline" onClick={() => navigate("Completed")}>View completed</Button>
                          ) : null}
                        </div>
                        <div className="empty-shortcut">
                          <kbd>⌘</kbd>
                          <kbd>N</kbd> New task
                        </div>
                      </div>
                    )}
                    </>}
                  </>
                )}
                {page === "View operations" && (
                  <Operations operations={operations} error={operationsError} query={query}
                    selected={selectedOperation} onSelect={setSelectedOperation}
                    onContinued={operationStarted}
                    onRefresh={refreshOperations} onTask={(id) => {
                      const task = data.tasks.find(t => t.id === id)
                      if (task) setEditor(task)
                      else notify("This task has been deleted. Its operation history is still available.")
                    }} />
                )}
                {page === "Integrations" && (
                  <Integrations
                    data={data}
                    onSave={async (settings) => {
                      await request("settings", settings)
                      await reload()
                      notify("Integrations saved.")
                      void sync(true)
                    }}
                  />
                )}
              </main>

            </div>
        </div>
        {editor && <Suspense fallback={null}>
          <TaskEditor
            key={editor.id || "new"}
            task={editor}
            onClose={() => {
              setEditor(null)
              if (editor.id) void reload().catch(() => notify("Task saved. Could not refresh tasks; try reloading.", true))
            }}
            onSave={async (task) => {
              if (task.id) {
                const saved = await request<Task>(`tasks/${task.id}`, {
                  title: task.title, notes: task.notes, priority: task.priority, dueDate: task.dueDate,
                  ...(task.status !== editor.status ? { status: task.status } : {}),
                }, "PATCH")
                setData(current => ({ ...current, tasks: current.tasks.map(existing => existing.id === saved.id ? saved : existing) }))
              } else {
                await saveTask(task)
                setFilter("all")
                if ((page === "Upcoming" && !calendarView && !task.dueDate) || page === "Completed") navigate("Today")
              }
            }}
          />
        </Suspense>}
        {organizer && (
          <Organizer
            provider={data.settings.harness}
            tasks={data.tasks}
            onClose={() => setOrganizer(false)}
            onSave={saveTask}
          />
        )}
        {runTask && <RunTask task={runTask.task} provider={runTask.provider}
          sessions={data.sessions} operations={operations} onClose={() => setRunTask(null)}
          onStarted={operationStarted} />}
        {(notification || runNotifications.finished.length > 0) && <div className="toast-stack">
          {runNotifications.finished.map(operation => <div key={operation.id}
            role={operation.status === "failed" ? "alert" : "status"}
            className={cn("toast", operation.status === "failed" && "toast-error")}>
            <span>{operation.status === "failed" ? <Circle /> : <CircleCheck />}</span>
            <p>{agentName(operation.provider)} run {operation.status === "failed" ? "failed" : "finished"}: {operation.title}</p>
            <Button variant="ghost" size="sm" className="toast-action" onClick={() => {
              navigate("View operations")
              setSelectedOperation(operation.id)
              runNotifications.dismiss(operation.id)
            }}>View run</Button>
            <button aria-label={`Dismiss notification for ${operation.title}`} onClick={() => runNotifications.dismiss(operation.id)}><X /></button>
          </div>)}
          {notification && (
          <div
            role={notification.error ? "alert" : "status"}
            className={cn("toast", notification.error && "toast-error")}
          >
            <span>{notification.error ? <Circle /> : <CircleCheck />}</span>
            <p>{notification.text}</p>
            <button aria-label="Dismiss notification" onClick={() => setNotification(null)}>
              <X />
            </button>
          </div>
          )}
        </div>}
      </div>
    </TooltipProvider>
  )
}

function TaskRow({
  task,
  onEdit,
  onUpdate,
  onDelete,
  onRun,
  running,
  onViewOperation,
  reordering,
  dragging,
  dropPosition,
  onDragStart,
  onDragEnd,
  onDragOver,
  onDrop,
  onMove,
}: {
  task: Task
  onEdit: () => void
  onUpdate: (task: Task) => Promise<void>
  onDelete: () => void
  onRun: (provider: AgentProvider) => void
  running: boolean
  onViewOperation: () => void
  reordering: boolean
  dragging: boolean
  dropPosition: "before" | "after" | null
  onDragStart: React.DragEventHandler<HTMLButtonElement>
  onDragEnd: React.DragEventHandler<HTMLButtonElement>
  onDragOver: React.DragEventHandler<HTMLDivElement>
  onDrop: React.DragEventHandler<HTMLDivElement>
  onMove: (direction: -1 | 1) => void
}) {
  const [busy, setBusy] = useState(false)
  const [pendingStatus, setPendingStatus] = useState<Status | null>(null)
  const currentStatus = pendingStatus ?? task.status
  const days = overdueDays(task)
  const effective = priority(task)
  const completed = currentStatus === "done"
  async function change(status: Status) {
    setPendingStatus(status)
    setBusy(true)
    try {
      await onUpdate({ ...task, status })
    } finally {
      setBusy(false)
      setPendingStatus(null)
    }
  }
  return (
    <div className={cn("task-row", completed && "completed", dragging && "is-dragging", dropPosition && `drop-${dropPosition}`)}
      data-task-id={task.id} onDragOver={onDragOver} onDrop={onDrop}>
      <button className="task-drag-handle" aria-label={`Reorder ${task.title}`}
        title="Drag to reorder · Alt + ↑ / ↓" draggable={!reordering} disabled={reordering}
        onDragStart={onDragStart} onDragEnd={onDragEnd}
        onKeyDown={(e) => {
          if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
            e.preventDefault()
            onMove(e.key === "ArrowUp" ? -1 : 1)
          }
        }}><GripVertical /></button>
      <Checkbox
        aria-label={`Complete ${task.title}`}
        className="task-checkbox"
        checked={completed}
        disabled={busy}
        onCheckedChange={(checked) => change(checked ? "done" : "todo")}
      />
      <button className="task-info" onClick={onEdit}>
        <span className="task-title">{task.title}</span>
        <span className="task-meta">
          {task.project && (
            <>
              <span className="task-project-dot" />
              {task.project}
              <i>·</i>
            </>
          )}
          {task.createdAt && (
            <time dateTime={startDate(task)} title={`Start date: ${startDate(task)}`}>
              Started {new Date(`${startDate(task)}T12:00:00`).toLocaleDateString(undefined, { month: "short", day: "numeric" })}
            </time>
          )}
          {task.dueDate && (
            <span className={days ? "overdue-text" : ""}>
              <CalendarDays />
              {days
                ? `${days}d overdue`
                : task.dueDate === day()
                  ? "Today"
                  : new Date(`${task.dueDate}T12:00:00`).toLocaleDateString(undefined, {
                      month: "short",
                      day: "numeric",
                    })}
            </span>
          )}
          {(task.sessionIds?.length || 0) > 0 && (
            <>
              <i>·</i>
              <span>
                <Link2 />
                {task.sessionIds!.length} {task.sessionIds!.length === 1 ? "session" : "sessions"}
              </span>
            </>
          )}
        </span>
      </button>
      {effective >= 2 && !completed && (
        <Badge
          variant="outline"
          className={cn("priority-badge", effective === 3 && "urgent")}
          title={
            days
              ? `Originally ${["low", "normal", "high", "urgent"][task.priority]}. Priority rises one level each overdue day.`
              : undefined
          }
        >
          <Flag />
          {effective === 3 ? "Urgent" : "High"}
        </Badge>
      )}
      {running ? (
        <Button variant="ghost" size="sm" className="start-button" onClick={onViewOperation}>
          <Loader2 className="animate-spin" />Running
        </Button>
      ) : !completed && (
        <Button
          variant="ghost"
          size="sm"
          className={cn("start-button", task.status === "working" && "pause-button")}
          disabled={busy}
          onClick={() => change(task.status === "working" ? "todo" : "working")}
        >
          {busy ? (
            <Loader2 className="animate-spin" />
          ) : task.status === "working" ? (
            <Pause />
          ) : (
            <Play />
          )}
          {task.status === "working" ? "Pause" : "Start"}
        </Button>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label={`Actions for ${task.title}`}
            variant="ghost"
            size="icon"
            className="task-menu size-7"
          >
            <Ellipsis />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={onEdit}>Edit task</DropdownMenuItem>
          {running && <DropdownMenuItem onSelect={onViewOperation}>View operation</DropdownMenuItem>}
          <DropdownMenuSub>
            <DropdownMenuSubTrigger disabled={completed || running}>
              {running ? "Agent running" : "Run with agent"}
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuItem onSelect={() => onRun("codex")}><Command />Codex</DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onRun("claude")}><Sparkles />Claude Code</DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
          <DropdownMenuItem onClick={() => onUpdate({ ...task, dueDate: day() })}>
            Move to today
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onClick={onDelete} disabled={running}>
            Delete task
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function Integrations({
  data,
  onSave,
}: {
  data: Workspace
  onSave: (settings: Settings) => Promise<void>
}) {
  const [settings, setSettings] = useState(data.settings)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError("")
    try {
      await onSave(settings)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <form onSubmit={submit} className="integration-form">
      <Card>
        <CardContent>
          <div className="settings-section-heading">
            <Sparkles />
            <div>
              <h2>Your task organizer</h2>
              <p>Turn your notes into tasks.</p>
            </div>
          </div>
          <div className="settings-row">
            <div>
              <Label>Preferred harness</Label>
              <p>Uses your installed CLI and existing sign-in.</p>
            </div>
            <Select
              value={settings.harness}
              onValueChange={(v) => setSettings((s) => ({ ...s, harness: v }))}
            >
              <SelectTrigger aria-label="Preferred harness" className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="codex">Codex</SelectItem>
                <SelectItem value="claude">Claude Code</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardContent>
          <div className="settings-section-heading">
            <Terminal />
            <div>
              <h2>Local session sources</h2>
              <p>Read locally. No model calls needed.</p>
            </div>
          </div>
          {(["codex", "claude"] as const).map((provider) => (
            <div className="source-setting" key={provider}>
              <div className="settings-row">
                <div className="source-title">
                  <HarnessIcon harness={provider} />
                  <Label htmlFor={`${provider}-enabled`}>
                    {provider === "codex" ? "Codex" : "Claude Code"}
                  </Label>
                </div>
                <Switch
                  id={`${provider}-enabled`}
                  checked={settings[`${provider}Enabled`]}
                  onCheckedChange={(v) => setSettings((s) => ({ ...s, [`${provider}Enabled`]: v }))}
                />
              </div>
              <Label className="sr-only" htmlFor={`${provider}-root`}>
                {provider} session folder
              </Label>
              <Input
                id={`${provider}-root`}
                value={settings[`${provider}Root`]}
                disabled={!settings[`${provider}Enabled`]}
                onChange={(e) =>
                  setSettings((s) => ({ ...s, [`${provider}Root`]: e.target.value }))
                }
              />
            </div>
          ))}
          <div className="settings-row">
            <div>
              <Label htmlFor="auto-sync">Keep things in sync</Label>
              <p>Import local sessions every minute while the window is visible.</p>
            </div>
            <Switch
              id="auto-sync"
              checked={settings.autoSync}
              onCheckedChange={(v) => setSettings((s) => ({ ...s, autoSync: v }))}
            />
          </div>
        </CardContent>
      </Card>
      <div className="settings-privacy">
        <LockKeyhole />
        <div>
          <h3>Local by default. Yours by design.</h3>
          <p>
            Your tasks, history, and sessions are saved on this computer. The organizer sends task
            descriptions and session excerpts to your selected provider only when you ask. Every
            suggestion is yours to review.
          </p>
          <code>{data.database}</code>
        </div>
      </div>
      {error && (
        <p className="text-destructive text-xs" role="alert">
          {error}
        </p>
      )}
      <Button type="submit" disabled={busy}>
        {busy && <Loader2 className="animate-spin" />}Save integrations
      </Button>
    </form>
  )
}
