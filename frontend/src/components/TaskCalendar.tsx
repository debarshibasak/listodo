import { useState, type CSSProperties, type DragEvent } from "react"
import { CalendarDays, ChevronLeft, ChevronRight, Circle, CircleCheck, CircleDashed, GripVertical, Loader2, Plus } from "lucide-react"
import { Button } from "@/components/ui/button"
import { day, type Task } from "@/lib/api"
import { cn } from "@/lib/utils"

const taskDragType = "application/x-listodo-calendar-task"
const weekdays = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]
const dateLabel = (date: Date) => date.toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric", year: "numeric" })

export function TaskCalendar({ tasks, selectedDate, onSelectDate, onCreate, onEdit, onSchedule }: {
  tasks: Task[]
  selectedDate: string
  onSelectDate: (date: string) => void
  onCreate: (date: string) => void
  onEdit: (task: Task) => void
  onSchedule: (task: Task, dueDate: string) => Promise<void>
}) {
  const [dragged, setDragged] = useState<string | null>(null)
  const [dropDate, setDropDate] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState("")
  const [error, setError] = useState("")
  // Use local calendar arithmetic so month boundaries also work across DST.
  const selected = new Date(`${selectedDate}T12:00:00`)
  const first = new Date(selected.getFullYear(), selected.getMonth(), 1, 12)
  const offset = (first.getDay() + 6) % 7
  const daysInMonth = new Date(selected.getFullYear(), selected.getMonth() + 1, 0).getDate()
  const cellCount = Math.ceil((offset + daysInMonth) / 7) * 7
  const dates = Array.from({ length: cellCount }, (_, index) => new Date(first.getFullYear(), first.getMonth(), 1 - offset + index, 12))
  const unscheduled = tasks.filter(task => !task.dueDate && task.status !== "done")
  const byDate = new Map<string, Task[]>()
  for (const task of tasks) {
    if (task.dueDate) {
      const tasksOnDay = byDate.get(task.dueDate)
      if (tasksOnDay) tasksOnDay.push(task)
      else byDate.set(task.dueDate, [task])
    }
  }
  const scheduledCount = tasks.filter(task => task.dueDate.slice(0, 7) === selectedDate.slice(0, 7)).length

  function changeMonth(amount: number) {
    onSelectDate(day(new Date(first.getFullYear(), first.getMonth() + amount, 1, 12)))
  }
  function startDrag(e: DragEvent, task: Task) {
    e.dataTransfer.setData(taskDragType, task.id)
    e.dataTransfer.effectAllowed = "move"
    setDragged(task.id)
  }
  function endDrag() { setDragged(null); setDropDate(null) }
  function dragOver(e: DragEvent, date: string) {
    if (saving || !e.dataTransfer.types.includes(taskDragType)) return
    e.preventDefault()
    e.dataTransfer.dropEffect = "move"
    setDropDate(date)
  }
  async function dropTask(e: DragEvent, date: string) {
    e.preventDefault()
    const task = tasks.find(task => task.id === e.dataTransfer.getData(taskDragType))
    endDrag()
    if (saving || !task || task.dueDate === date) return
    setSaving(true)
    setError("")
    setMessage("")
    try {
      await onSchedule(task, date)
      setMessage(date ? `Scheduled “${task.title}” for ${dateLabel(new Date(`${date}T12:00:00`))}.` : `Removed the due date from “${task.title}”.`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally { setSaving(false) }
  }
  function taskButton(task: Task, unscheduled = false) {
    const Icon = task.status === "done" ? CircleCheck : task.status === "working" ? CircleDashed : Circle
    return <button key={task.id} type="button"
      className={cn("calendar-task", task.status === "done" && "is-completed", dragged === task.id && "is-dragging")}
      draggable={!saving} onDragStart={e => startDrag(e, task)} onDragEnd={endDrag}
      onClick={() => onEdit(task)} disabled={saving}
      aria-label={`${unscheduled ? "Schedule" : "Edit"} ${task.title}`} title={task.title}>
      {unscheduled ? <GripVertical /> : <Icon />}
      <span>{task.title}</span>
      {task.priority >= 2 && <span className="calendar-priority" aria-label={task.priority === 3 ? "Urgent priority" : "High priority"}>{task.priority === 3 ? "!!" : "!"}</span>}
    </button>
  }

  return <section className="task-calendar" aria-label="Task calendar" aria-busy={saving}>
    <div className="calendar-toolbar">
      <div className="calendar-month"><h2 aria-live="polite">{first.toLocaleDateString("en-US", { month: "long", year: "numeric" })}</h2><span>{scheduledCount} scheduled</span></div>
      <div className="calendar-navigation">
        <Button variant="outline" size="sm" onClick={() => onSelectDate(day())}>Today</Button>
        <Button variant="ghost" size="icon-xs" aria-label="Previous month" onClick={() => changeMonth(-1)}><ChevronLeft /></Button>
        <Button variant="ghost" size="icon-xs" aria-label="Next month" onClick={() => changeMonth(1)}><ChevronRight /></Button>
      </div>
    </div>
    <p className="calendar-hint">Click a date to add a task. Drag tasks to plan or reschedule.</p>
    <div className="calendar-planner">
      <div className="calendar-month-scroll" tabIndex={0} aria-label="Month calendar">
        <div className="calendar-grid" style={{ "--calendar-weeks": cellCount / 7 } as CSSProperties}>
          {weekdays.map(weekday => <div className="calendar-weekday" key={weekday}>{weekday}</div>)}
          {dates.map(date => {
            const key = day(date)
            const dayTasks = byDate.get(key) || []
            return <div key={key} data-date={key} role="group" aria-label={dateLabel(date)}
              className={cn("calendar-day", date.getMonth() !== first.getMonth() && "outside-month", key === day() && "is-today", key === selectedDate && "is-selected", dropDate === key && "is-drop-target")}
              onDragOver={e => dragOver(e, key)} onDragLeave={e => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDropDate(null) }} onDrop={e => void dropTask(e, key)}>
              <button type="button" className="calendar-add-day" aria-label={`Add task for ${dateLabel(date)}`} disabled={saving}
                onClick={() => { onSelectDate(key); onCreate(key) }}><Plus /></button>
              <time className="calendar-date" dateTime={key} aria-current={key === day() ? "date" : undefined}>{date.getDate()}</time>
              <div className="calendar-day-tasks">{dayTasks.map(task => taskButton(task))}</div>
            </div>
          })}
        </div>
      </div>
      <aside className={cn("calendar-unscheduled", dropDate === "" && "is-drop-target")}
        aria-label="Unscheduled tasks" onDragOver={e => dragOver(e, "")} onDragLeave={() => setDropDate(null)} onDrop={e => void dropTask(e, "")}>
        <h3><CalendarDays />Unscheduled<span>{unscheduled.length}</span></h3>
        <p>Drag onto a day, or open a task to set its due date.</p>
        <div className="calendar-unscheduled-list">{unscheduled.map(task => taskButton(task, true))}</div>
        {!unscheduled.length && <div className="calendar-unscheduled-empty"><CircleCheck /><span>All clear</span><p>Tasks without a due date appear here.</p></div>}
      </aside>
    </div>
    <div className="calendar-feedback" role="status">{saving ? <><Loader2 className="animate-spin" />Saving schedule…</> : message}</div>
    {error && <p className="calendar-error" role="alert">{error}</p>}
  </section>
}
