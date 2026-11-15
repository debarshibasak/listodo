import { test, expect } from "@playwright/test"
import { emptyTask, isToday, startDate } from "../src/lib/api"
import { parseTaskText } from "../src/lib/task-text"

test("Markdown notes and fenced examples stay with their task", () => {
  const tasks = parseTaskText("- [ ] Document the format\n```md\n- [ ] Example only\n```\n- [x] Ship it", emptyTask())
  expect(tasks.map(t => t.title)).toEqual(["Document the format", "Ship it"])
  expect(tasks[0].notes).toContain("- [ ] Example only")
  expect(tasks[1].status).toBe("done")
  const existing = { ...emptyTask(), id: "existing", status: "working" as const }
  const edited = parseTaskText("Task\n\n- [ ] Subtask", existing)
  expect(edited).toHaveLength(1)
  expect(edited[0]).toMatchObject({ notes: "- [ ] Subtask", status: "working" })
  const titled = parseTaskText("Release v1.2.x\n\n- [ ] Data sync\n- [x] Server UI", emptyTask())
  expect(titled).toHaveLength(1)
  expect(titled[0]).toMatchObject({ title: "Release v1.2.x", notes: "- [ ] Data sync\n- [x] Server UI", status: "todo" })
})

test("start dates use the creation calendar day and open tasks carry forward", () => {
  const task = { ...emptyTask(), createdAt: "2020-01-02T00:15:00+14:00" }
  expect(startDate(task)).toBe("2020-01-02")
  expect(isToday(task)).toBe(true)
  expect(isToday({ ...task, dueDate: "2099-01-01" })).toBe(false)
  expect(startDate({ ...task, startDate: "2020-01-03" })).toBe("2020-01-03")
})
