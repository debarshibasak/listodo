import { test as base, expect, chromium, type Page } from "@playwright/test"
import { day, emptyTask, type Operation } from "../src/lib/api"

// A separate browser per test also supports single-process Chromium in restricted
// macOS environments, where reusing a browser after closing its context can fail.
const test = base.extend({
  context: async ({ launchOptions, viewport, baseURL }, use) => {
    const browser = await chromium.launch(launchOptions)
    const context = await browser.newContext({ viewport, baseURL, reducedMotion: "reduce" })
    try {
      await use(context)
    } finally {
      await browser.close()
    }
  },
})

async function api(page: Page, path: string, body?: unknown) {
  return page.evaluate(
    async ({ path, body }) => {
      const token = document.querySelector<HTMLMetaElement>('meta[name="listodo-token"]')!.content
      const r = await fetch(`/api/${path}`, {
        method: body === undefined ? "GET" : "POST",
        headers: { "X-Listodo-Token": token, "Content-Type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    { path, body },
  )
}

// The source toggle also exercises Markdown -> document conversion.
async function fillTask(page: Page, markdown: string) {
  await page.getByRole("button", { name: "Edit Markdown", exact: true }).click()
  await page.getByRole("textbox", { name: "Task Markdown", exact: true }).fill(markdown)
  await page.getByRole("button", { name: "Show formatted text", exact: true }).click()
  await expect(page.getByRole("textbox", { name: "Task", exact: true })).toBeVisible()
}

async function pasteTask(page: Page, markdown: string) {
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  await input.focus()
  await input.evaluate((element, text) => {
    const clipboardData = new DataTransfer()
    clipboardData.setData("text/plain", text)
    element.dispatchEvent(new ClipboardEvent("paste", { clipboardData, bubbles: true, cancelable: true }))
  }, markdown)
}

test.beforeEach(async ({ page, request }) => {
  const index = await request.get("/")
  const token = (await index.text()).match(/name="listodo-token" content="([a-f0-9]+)"/)![1]
  const headers = { "X-Listodo-Token": token }
  const state = await (await request.get("/api/state", { headers })).json()
  await request.post("/api/settings", {
    headers,
    data: { ...state.settings, autoSync: false, codexEnabled: false, claudeEnabled: false },
  })
  for (const task of state.tasks) await request.delete(`/api/tasks/${task.id}`, { headers })
  await page.goto("/")
  await expect(page.getByRole("heading", { name: "Today", exact: true })).toBeVisible()
})

test("create, start, complete, reopen, edit and delete a persisted task", async ({ page }) => {
  const errors: string[] = []
  page.on("pageerror", (e) => errors.push(e.message))
  await expect(page.getByRole("heading", { name: "No tasks yet" })).toBeVisible()
  await page.getByRole("button", { name: "New task", exact: true }).click()
  await expect(page.getByRole("dialog").getByRole("textbox", { name: "Task", exact: true })).toHaveCount(1)
  await expect(page.getByLabel("Due date", { exact: true })).toHaveValue("")
  await expect(page.getByRole("dialog").getByRole("combobox")).toHaveCount(0)
  await fillTask(page, "Ship a calmer workspace\n\nReview the interface.")
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByRole("dialog")).not.toBeVisible()
  await expect(page.getByText("Ship a calmer workspace", { exact: true })).toBeVisible()
  await page.getByRole("button", { name: "Start", exact: true }).click()
  await expect(page.getByRole("button", { name: "Pause", exact: true })).toBeVisible()
  await page.getByRole("checkbox", { name: "Complete Ship a calmer workspace" }).check()
  await expect(page.getByText("Ship a calmer workspace", { exact: true })).not.toBeVisible()
  await expect(page.getByRole("heading", { name: "No open tasks" })).toBeVisible()
  await page.getByRole("tab", { name: "Done", exact: true }).click()
  await expect(page.getByRole("checkbox")).toBeChecked()
  await page.reload()
  await expect(page.getByText("Ship a calmer workspace", { exact: true })).not.toBeVisible()
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Completed", exact: true }).click()
  await expect(page.getByRole("checkbox")).toBeChecked()
  await page.getByRole("checkbox").uncheck()
  await expect(page.getByText("Ship a calmer workspace", { exact: true })).not.toBeVisible()
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: /^All tasks/ }).click()
  await page
    .getByRole("button", { name: /^Ship a calmer workspace/ })
    .first()
    .click()
  await fillTask(page, "Ship the minimal workspace")
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByText("Ship the minimal workspace", { exact: true })).toBeVisible()
  await page.getByRole("button", { name: /Actions for Ship/ }).click()
  await page.getByRole("menuitem", { name: "Delete task" }).click()
  await expect(page.getByText("Ship the minimal workspace", { exact: true })).not.toBeVisible()
  expect(errors).toEqual([])
})

test("quick add, search, filters, navigation, backup and responsive layout", async ({ page }) => {
  await page.getByRole("textbox", { name: "Quick add task" }).fill("Review the API changes")
  await page.getByRole("textbox", { name: "Quick add task" }).press("Enter")
  await expect(page.getByText("Review the API changes", { exact: true })).toBeVisible()
  await page.getByRole("textbox", { name: "Search tasks or sessions" }).fill("no matching task")
  await expect(page.getByRole("heading", { name: "No matching tasks" })).toBeVisible()
  await page.getByRole("button", { name: "Clear filters" }).click()
  await page.getByRole("tab", { name: "In progress", exact: true }).click()
  await expect(page.getByRole("heading", { name: "No tasks in this view" })).toBeVisible()
  await page.getByRole("button", { name: "Clear filters" }).click()
  for (const name of ["All tasks", "Upcoming", "Completed", "Activity", "Integrations", "Today"]) {
    if (name === "Integrations")
      await page.getByRole("button", { name: "Integrations", exact: true }).click()
    else
      await page
        .getByRole("navigation", { name: "Main navigation" })
        .getByRole("button", { name: new RegExp(name) })
        .click()
    await expect(page.getByRole("heading", { name: name, exact: true })).toBeVisible()
  }
  const downloadPromise = page.waitForEvent("download")
  await page.getByRole("button", { name: "Export backup" }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toContain("listodo-backup")
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole("heading", { name: "Today", exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  )
  await page.getByRole("button", { name: "New task", exact: true }).click()
  await expect(page.getByRole("dialog")).toBeVisible()
})

test("calendar dates create scheduled tasks and preserve the Upcoming view", async ({ page }) => {
  const planned = new Date()
  planned.setMonth(planned.getMonth() + 1, 12)
  const dueDate = day(planned)
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Upcoming", exact: true }).click()
  await expect(page.getByRole("button", { name: "Calendar view" })).toHaveAttribute("aria-pressed", "true")
  await page.getByRole("button", { name: "Next month" }).click()
  await expect(page.getByRole("heading", { name: planned.toLocaleDateString("en-US", { month: "long", year: "numeric" }) })).toBeVisible()
  await page.locator(`[data-date="${dueDate}"]`).getByRole("button", { name: /^Add task for/ }).click()
  await expect(page.getByLabel("Due date", { exact: true })).toHaveValue(dueDate)
  await fillTask(page, "- [ ] Plan the release\n- [ ] Prepare the announcement")
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByRole("dialog")).not.toBeVisible()
  await expect(page.getByRole("heading", { name: "Upcoming", exact: true })).toBeVisible()
  const cell = page.locator(`[data-date="${dueDate}"]`)
  await expect(cell.getByRole("button", { name: "Edit Plan the release" })).toBeVisible()
  await expect(cell.getByRole("button", { name: "Edit Prepare the announcement" })).toBeVisible()
  const tasks = (await api(page, "state")).tasks
  expect(tasks).toHaveLength(2)
  expect(tasks.every((task: { dueDate: string }) => task.dueDate === dueDate)).toBe(true)
  await page.getByRole("button", { name: "List view" }).click()
  await expect(page.getByRole("region", { name: "Task list" })).toContainText("Plan the release")
  await page.getByRole("button", { name: "Calendar view" }).click()
  await expect(cell.getByRole("button", { name: "Edit Plan the release" })).toBeVisible()
  await page.getByRole("button", { name: "New task", exact: true }).click()
  await expect(page.getByLabel("Due date", { exact: true })).toHaveValue(dueDate)
  await page.keyboard.press("Escape")
  await page.getByRole("button", { name: "Today", exact: true }).last().click()
  await expect(page.getByRole("heading", { name: new Date().toLocaleDateString("en-US", { month: "long", year: "numeric" }) })).toBeVisible()
  await page.reload()
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Upcoming", exact: true }).click()
  await page.getByRole("button", { name: "Next month" }).click()
  await expect(cell.getByRole("button", { name: "Edit Plan the release" })).toBeVisible()
})

test("calendar editor schedules, reschedules and clears dates while preserving task metadata", async ({ page }) => {
  const task = await api(page, "tasks", { ...emptyTask("Platform"), title: "Plan an existing task", notes: "Keep these notes", directory: "/projects/platform", sessionIds: ["codex:linked"], status: "working", priority: 2 })
  await api(page, "tasks", { ...emptyTask(), title: "Finished plan", status: "done", dueDate: day() })
  await page.reload()
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Upcoming", exact: true }).click()
  await page.getByRole("button", { name: "Schedule Plan an existing task" }).click()
  await page.getByLabel("Due date", { exact: true }).fill(day())
  await expect.poll(async () => (await api(page, "state")).tasks.find((t: { id: string }) => t.id === task.id).dueDate).toBe(day())
  await page.keyboard.press("Escape")
  await expect(page.locator(`[data-date="${day()}"]`).getByRole("button", { name: "Edit Plan an existing task" })).toBeVisible()
  await page.getByRole("tab", { name: "To do", exact: true }).click()
  await expect(page.getByRole("button", { name: "Edit Plan an existing task" })).toHaveCount(0)
  await page.getByRole("tab", { name: "In progress", exact: true }).click()
  await page.getByRole("button", { name: "Edit Plan an existing task" }).click()
  const next = new Date()
  next.setDate(next.getDate() === 20 ? 21 : 20)
  await page.getByLabel("Due date", { exact: true }).fill(day(next))
  await page.keyboard.press("Escape")
  await expect(page.locator(`[data-date="${day(next)}"]`).getByRole("button", { name: "Edit Plan an existing task" })).toBeVisible()
  await page.getByRole("button", { name: "Edit Plan an existing task" }).click()
  await page.getByRole("button", { name: "Remove due date" }).click()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("complementary", { name: "Unscheduled tasks" })).toContainText(task.title)
  expect((await api(page, "state")).tasks.find((t: { id: string }) => t.id === task.id)).toMatchObject({
    dueDate: "", title: task.title, notes: task.notes, project: task.project, directory: task.directory,
    sessionIds: task.sessionIds, status: task.status, priority: task.priority, startDate: task.startDate, createdAt: task.createdAt,
  })
  await page.getByRole("tab", { name: "Done", exact: true }).click()
  await expect(page.getByRole("button", { name: "Edit Finished plan" })).toBeVisible()
  await expect(page.getByRole("button", { name: "Schedule Plan an existing task" })).toHaveCount(0)
  await page.getByRole("textbox", { name: "Search tasks or sessions" }).fill("no matching task")
  await expect(page.getByRole("button", { name: "Edit Finished plan" })).toHaveCount(0)
})

test("calendar drag planning persists and failed schedule changes can be retried", async ({ page }) => {
  const task = await api(page, "tasks", { ...emptyTask(), title: "Drag this task" })
  await page.reload()
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Upcoming", exact: true }).click()
  const first = new Date()
  first.setDate(10)
  const second = new Date()
  second.setDate(20)
  const cell = page.locator(`[data-date="${day(first)}"]`)
  await page.route(`**/api/tasks/${task.id}`, route => route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Schedule could not be saved" }) }))
  await page.getByRole("button", { name: "Schedule Drag this task" }).dragTo(cell)
  await expect(page.getByRole("alert")).toHaveText("Schedule could not be saved")
  await expect(page.getByRole("button", { name: "Schedule Drag this task" })).toBeVisible()
  expect((await api(page, "state")).tasks[0].dueDate).toBe("")
  await page.unroute(`**/api/tasks/${task.id}`)
  await page.getByRole("button", { name: "Schedule Drag this task" }).dragTo(cell)
  await expect(cell.getByRole("button", { name: "Edit Drag this task" })).toBeVisible()
  await expect(page.getByRole("alert")).toHaveCount(0)
  await cell.getByRole("button", { name: "Edit Drag this task" }).dragTo(page.locator(`[data-date="${day(second)}"]`))
  await expect.poll(async () => (await api(page, "state")).tasks[0].dueDate).toBe(day(second))
  await page.reload()
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Upcoming", exact: true }).click()
  const moved = page.locator(`[data-date="${day(second)}"]`).getByRole("button", { name: "Edit Drag this task" })
  await expect(moved).toBeVisible()
  await moved.dragTo(page.getByRole("complementary", { name: "Unscheduled tasks" }))
  await expect(page.getByRole("button", { name: "Schedule Drag this task" })).toBeVisible()
  expect((await api(page, "state")).tasks[0].dueDate).toBe("")
})

test("calendar supports keyboard creation, dark appearance and narrow windows", async ({ page }) => {
  for (const [title, date] of [["Review the roadmap", 8], ["Write release notes", 12], ["Ship the update", 20]] as const) {
    const planned = new Date()
    planned.setDate(date)
    await api(page, "tasks", { ...emptyTask(), title, dueDate: day(planned) })
  }
  await api(page, "tasks", { ...emptyTask(), title: "Explore next quarter" })
  await page.reload()
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Upcoming", exact: true }).click()
  await page.screenshot({ path: "../build/preview-calendar.png", fullPage: true })
  await page.emulateMedia({ colorScheme: "dark" })
  await page.screenshot({ path: "../build/preview-calendar-dark.png", fullPage: true })
  for (const viewport of [{ width: 900, height: 700 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(viewport)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await expect(page.getByRole("button", { name: "Next month" })).toBeVisible()
    await expect(page.getByRole("button", { name: "Calendar view" })).toBeVisible()
  }
  await page.screenshot({ path: "../build/preview-calendar-mobile.png", fullPage: true })
  await page.setViewportSize({ width: 1380, height: 880 })
  const add = page.locator(`[data-date="${day()}"]`).getByRole("button", { name: /^Add task for/ })
  await add.focus()
  await add.press("Enter")
  await expect(page.getByLabel("Due date", { exact: true })).toHaveValue(day())
  await page.getByRole("textbox", { name: "Task", exact: true }).fill("Plan with the keyboard")
  await page.keyboard.press("Control+Enter")
  await expect(page.getByRole("dialog")).not.toBeVisible()
  await expect(page.getByRole("button", { name: "Edit Plan with the keyboard" })).toBeVisible()
})

test("organizer reviews suggestions before applying and keeps errors visible", async ({ page }) => {
  await page.route("**/api/organize", (route) =>
    route.fulfill({
      json: {
        summary: "One clear next step.",
        tasks: [
          {
            title: "Review the interface",
            notes: "Check the task flow.",
            project: "Listodo",
            dueDate: day(),
            priority: 1,
          },
        ],
        matches: [],
      },
    }),
  )
  await page.getByRole("button", { name: "Organize", exact: true }).click()
  await page
    .getByRole("textbox", { name: "Describe your plans" })
    .fill("Review the interface today")
  await page.getByRole("button", { name: "Prepare suggestions" }).click()
  expect((await api(page, "state")).tasks).toHaveLength(0)
  await page.getByRole("button", { name: "Add task", exact: true }).click()
  await expect(page.getByRole("button", { name: "Added" })).toBeDisabled()
  await page.getByRole("button", { name: "Done", exact: true }).click()
  await expect(page.getByText("Review the interface", { exact: true })).toBeVisible()
  await page.route("**/api/organize", (route) =>
    route.fulfill({ status: 400, json: { error: "Sign in to the Codex CLI and try again." } }),
  )
  await page.getByRole("button", { name: "Organize", exact: true }).click()
  await page.getByRole("button", { name: "Prepare suggestions" }).click()
  await expect(page.getByRole("alert")).toHaveText("Sign in to the Codex CLI and try again.")
})

const sampleSessions = () => [
  {
    id: "codex:sample",
    harness: "codex",
    title: "Review the API pagination changes",
    project: "Platform",
    directory: "/projects/platform",
    summary: "Reviewed pagination and added coverage for the cursor contract.",
    updatedAt: new Date().toISOString(),
    startedAt: new Date().toISOString(),
    status: "working",
  },
  {
    id: "claude:sample",
    harness: "claude",
    title: "Verify task storage and completion history",
    project: "Listodo",
    directory: "/projects/listodo",
    summary: "Verified tasks persist after a restart.",
    updatedAt: new Date(Date.now() - 25 * 60000).toISOString(),
    startedAt: new Date().toISOString(),
    status: "idle",
  },
]
async function withSampleSessions(page: Page) {
  await page.route("**/api/state", async (route) => {
    const response = await route.fetch()
    await route.fulfill({ json: { ...(await response.json()), sessions: sampleSessions() } })
  })
}

test("inspect a session and persist its task link without changing task status", async ({
  page,
}) => {
  const task = await api(page, "tasks", {
    ...emptyTask("Platform"),
    title: "Review the pagination contract",
  })
  await withSampleSessions(page)
  await page.reload()
  await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Activity" }).click()
  await page.getByRole("button", { name: /Review the API pagination/ }).click()
  await expect(page.getByRole("dialog")).toBeVisible()
  await expect(
    page.getByText("Reviewed pagination and added coverage for the cursor contract."),
  ).toBeVisible()
  await page.getByRole("combobox", { name: "Choose task to link" }).click()
  await page.getByRole("option", { name: "Review the pagination contract" }).click()
  await page.getByRole("button", { name: "Link task", exact: true }).click()
  await expect
    .poll(
      async () =>
        (await api(page, "state")).tasks.find((t: { id: string }) => t.id === task.id).sessionIds,
    )
    .toContain("codex:sample")
  expect(
    (await api(page, "state")).tasks.find((t: { id: string }) => t.id === task.id).status,
  ).toBe("todo")
  await page.getByRole("button", { name: "Create a task from this session" }).click()
  await expect(page.getByLabel("Task", { exact: true })).toContainText("Reviewed pagination and added coverage for the cursor contract.")
})

test("capture empty and populated layouts", async ({ page }) => {
  await expect(page.getByRole("complementary", { name: "Session activity" })).toHaveCount(0)
  await page.screenshot({ path: "../build/preview-empty.png", fullPage: true })
  const overdue = new Date()
  overdue.setDate(overdue.getDate() - 2)
  for (const task of [
    { ...emptyTask("Platform"), title: "Finish the authentication flow", dueDate: day(overdue) },
    { ...emptyTask("Platform"), title: "Review the API changes", status: "working", priority: 2 },
    { ...emptyTask("Listodo"), title: "Write the release notes" },
    { ...emptyTask("Listodo"), title: "Test SQLite persistence across restarts", status: "done" },
  ])
    await api(page, "tasks", task)
  await withSampleSessions(page)
  await page.reload()
  await expect(page.getByText("Finish the authentication flow", { exact: true })).toBeVisible()
  await page.screenshot({ path: "../build/preview.png", fullPage: true })
  await page.getByRole("button", { name: "New task", exact: true }).click()
  await page.screenshot({ path: "../build/preview-editor.png", fullPage: true })
})

test("Markdown checklist creates tasks with automatic, immutable start dates", async ({ page }) => {
  await page.getByRole("button", { name: "New task", exact: true }).click()
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  await fillTask(page, "- [ ] Draft the release\n  Include **migration notes**.\n- [x] Review the changes")
  await page.keyboard.press("ControlOrMeta+Enter")
  await expect(page.getByRole("dialog")).not.toBeVisible()
  await expect(page.getByRole("checkbox", { name: "Complete Draft the release" })).not.toBeChecked()
  await expect(page.getByRole("checkbox", { name: "Complete Review the changes" })).not.toBeVisible()
  await page.getByRole("tab", { name: "Done", exact: true }).click()
  await expect(page.getByRole("checkbox", { name: "Complete Review the changes" })).toBeChecked()
  await page.getByRole("tab", { name: /^All tasks/ }).click()
  const tasks = (await api(page, "state")).tasks
  expect(tasks).toHaveLength(2)
  for (const task of tasks) {
    expect(task.startDate).toBe(day())
    expect(task.dueDate).toBe("")
  }
  const draft = tasks.find((task: { title: string }) => task.title === "Draft the release")
  expect(draft.notes).toBe("Include **migration notes**.")
  await page.getByRole("button", { name: /^Draft the release/ }).click()
  await fillTask(page, "Publish the release\n\nInclude **migration notes**.")
  await page.getByRole("button", { name: "Save task" }).click()
  await page.reload()
  const updated = (await api(page, "state")).tasks.find((task: { id: string }) => task.id === draft.id)
  expect(updated.startDate).toBe(draft.startDate)
  expect(updated.createdAt).toBe(draft.createdAt)
  expect(updated.notes).toBe(draft.notes)
  await expect(page.getByTitle(`Start date: ${day()}`).first()).toBeVisible()
})

test("text-only editing preserves task metadata and its saved project folder", async ({ page }) => {
  const task = await api(page, "tasks", {
    ...emptyTask("Platform"), title: "Existing task", notes: "Keep these notes",
    status: "working", priority: 2, dueDate: day(), sessionIds: ["codex:existing"],
    directory: "/projects/saved-folder",
  })
  await page.reload()
  await page.getByRole("button", { name: /^Existing task/ }).click()
  await expect(page.locator(".task-editor-folder")).toContainText(task.directory)
  await fillTask(page, "Renamed task\n\nKeep these notes")
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByRole("dialog")).not.toBeVisible()
  const updated = (await api(page, "state")).tasks.find((t: { id: string }) => t.id === task.id)
  expect(updated).toMatchObject({
    project: task.project, status: task.status, priority: task.priority,
    dueDate: task.dueDate, startDate: task.startDate, createdAt: task.createdAt,
    sessionIds: task.sessionIds, notes: task.notes,
    directory: task.directory,
  })
  await page.reload()
  await page.getByRole("button", { name: "Actions for Renamed task" }).click()
  await page.getByRole("menuitem", { name: "Run with agent" }).hover()
  await page.getByRole("menuitem", { name: "Codex", exact: true }).click()
  await expect(page.getByLabel("Project folder")).toHaveValue(task.directory)
})

test("existing task text and priority auto-save without closing or losing metadata", async ({ page }) => {
  const task = await api(page, "tasks", { ...emptyTask("Platform"), title: "Auto-save this task", notes: "Original notes", directory: "/projects/original" })
  let writes = 0
  await page.route(`**/api/tasks/${task.id}`, async route => { writes++; await route.continue() })
  await page.reload()
  await page.getByRole("button", { name: /^Auto-save this task/ }).click()
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  await expect(page.locator(".task-save-status")).toHaveText("Saved")
  expect(writes).toBe(0)
  await input.locator("p").last().fill("Original notes updated")
  await expect.poll(async () => (await api(page, "state")).tasks[0].notes).toBe("Original notes updated")
  await expect(page.locator(".task-save-status")).toHaveText("Saved")
  await expect(input).toBeFocused()
  await expect(input).toBeEditable()
  // An agent changes these fields while the editor remains open.
  const current = (await api(page, "state")).tasks[0]
  await api(page, "tasks", { ...current, status: "working", directory: "/projects/associated", sessionIds: ["codex:new-session"] })
  await page.getByRole("button", { name: "Edit Markdown", exact: true }).click()
  await page.getByRole("textbox", { name: "Task Markdown", exact: true }).fill("Auto-saved title\n\nSource **notes**.")
  await page.getByRole("button", { name: "Priority: Normal" }).click()
  await page.getByRole("menuitemradio", { name: "Urgent", exact: true }).click()
  await expect.poll(async () => {
    const saved = (await api(page, "state")).tasks[0]
    return { title: saved.title, notes: saved.notes, priority: saved.priority }
  }).toEqual({ title: "Auto-saved title", notes: "Source **notes**.", priority: 3 })
  await expect(page.locator(".task-save-status")).toHaveText("Saved")
  const saved = (await api(page, "state")).tasks[0]
  expect(saved).toMatchObject({ project: "Platform", status: "working", directory: "/projects/associated", sessionIds: ["codex:new-session"], createdAt: task.createdAt })
  const beforeClose = writes
  await page.keyboard.press("Escape")
  await expect(page.getByRole("dialog")).not.toBeVisible()
  expect(writes).toBe(beforeClose)
  await page.reload()
  await page.getByRole("button", { name: /^Auto-saved title/ }).click()
  await expect(page.getByRole("button", { name: "Priority: Urgent" })).toBeVisible()
  await expect(page.locator(".task-editor-folder")).toContainText("/projects/associated")
})

test("closing flushes edits and a failed save preserves the draft for retry", async ({ page }) => {
  const task = await api(page, "tasks", { ...emptyTask(), title: "Original task" })
  let writes = 0
  await page.route(`**/api/tasks/${task.id}`, async route => {
    if (++writes === 1) return route.fulfill({ status: 500, json: { error: "Could not save the changes." } })
    await route.continue()
  })
  await page.reload()
  await page.getByRole("button", { name: /^Original task/ }).click()
  await fillTask(page, "Keep this draft\n\nUnsaved details")
  await page.keyboard.press("Escape")
  await expect(page.getByRole("dialog")).toBeVisible()
  await expect(page.getByRole("alert")).toHaveText("Could not save the changes.")
  await expect(page.locator(".task-save-status")).toHaveText("Not saved")
  await expect(page.getByRole("textbox", { name: "Task", exact: true })).toContainText("Unsaved details")
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByRole("dialog")).not.toBeVisible()
  expect(writes).toBe(2)
  expect((await api(page, "state")).tasks[0]).toMatchObject({ title: "Keep this draft", notes: "Unsaved details" })
})

test("auto-save serializes slow writes and flushes the latest typing before close", async ({ page }) => {
  const task = await api(page, "tasks", { ...emptyTask(), title: "Slow save" })
  let release!: () => void
  const firstWrite = new Promise<void>(resolve => { release = resolve })
  let writes = 0
  let active = 0
  let maxActive = 0
  await page.route(`**/api/tasks/${task.id}`, async route => {
    writes++
    active++
    maxActive = Math.max(active, maxActive)
    if (writes === 1) await firstWrite
    const response = await route.fetch()
    active--
    await route.fulfill({ response })
  })
  await page.reload()
  await page.getByRole("button", { name: /^Slow save/ }).click()
  await page.getByRole("button", { name: "Edit Markdown", exact: true }).click()
  const input = page.getByRole("textbox", { name: "Task Markdown", exact: true })
  try {
    await input.fill("First version")
    await expect.poll(() => writes).toBe(1)
    await expect(page.locator(".task-save-status")).toHaveText("Saving…")
    await expect(input).toBeEditable()
    await input.fill("Latest version\n\nKeep the newest details")
    await page.getByRole("button", { name: "Priority: Normal" }).click()
    await page.getByRole("menuitemradio", { name: "High", exact: true }).click()
    await page.keyboard.press("Escape")
    await expect(page.getByRole("dialog")).toBeVisible()
    expect(writes).toBe(1)
  } finally { release() }
  await expect(page.getByRole("dialog")).not.toBeVisible()
  expect(writes).toBe(2)
  expect(maxActive).toBe(1)
  expect((await api(page, "state")).tasks[0]).toMatchObject({ title: "Latest version", notes: "Keep the newest details", priority: 2 })
})

test("incomplete edits stay local and new tasks are created explicitly", async ({ page }) => {
  const task = await api(page, "tasks", { ...emptyTask(), title: "Keep the saved title" })
  let writes = 0
  await page.route(`**/api/tasks/${task.id}`, async route => { writes++; await route.continue() })
  await page.reload()
  await page.getByRole("button", { name: /^Keep the saved title/ }).click()
  await page.getByRole("button", { name: "Edit Markdown", exact: true }).click()
  const input = page.getByRole("textbox", { name: "Task Markdown", exact: true })
  await input.fill("")
  await page.waitForTimeout(750)
  expect(writes).toBe(0)
  await page.keyboard.press("Escape")
  await expect(page.getByRole("dialog")).toBeVisible()
  await expect(page.getByRole("alert")).toContainText("Give your task a title")
  await input.fill("A valid replacement")
  await expect.poll(async () => (await api(page, "state")).tasks[0].title).toBe("A valid replacement")
  await page.keyboard.press("Escape")
  await expect(page.getByRole("dialog")).not.toBeVisible()
  await page.getByRole("button", { name: "New task", exact: true }).click()
  await fillTask(page, "A new task draft")
  await page.waitForTimeout(750)
  expect((await api(page, "state")).tasks).toHaveLength(1)
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByRole("dialog")).not.toBeVisible()
  expect((await api(page, "state")).tasks).toHaveLength(2)
})

test("failed checklist saves can be retried without duplicating saved tasks", async ({ page }) => {
  let fail = true
  await page.route("**/api/tasks", async (route) => {
    if (route.request().postDataJSON().title === "Second task" && fail) {
      fail = false
      return route.fulfill({ status: 500, json: { error: "Could not save this task." } })
    }
    await route.continue()
  })
  await page.getByRole("button", { name: "New task", exact: true }).click()
  await fillTask(page, "- [ ] First task\n- [ ] Second task")
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByRole("alert")).toHaveText("Could not save this task.")
  await expect(page.getByRole("textbox", { name: "Task", exact: true })).toContainText("Second task")
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByRole("dialog")).not.toBeVisible()
  expect((await api(page, "state")).tasks).toHaveLength(2)
})

test("a failed refresh after saving does not duplicate a task", async ({ page }) => {
  await page.route("**/api/state", (route) => route.fulfill({ status: 503, json: { error: "Unavailable" } }))
  await page.getByRole("button", { name: "New task", exact: true }).click()
  await fillTask(page, "Saved despite refresh failure")
  await page.getByRole("button", { name: "Save task" }).click()
  await expect(page.getByRole("dialog")).not.toBeVisible()
  await expect(page.getByText("Saved despite refresh failure", { exact: true })).toBeVisible()
  await page.unroute("**/api/state")
  expect((await api(page, "state")).tasks).toHaveLength(1)
})

test("task agent menu launches Codex or Claude and shows live operations", async ({ page }) => {
  const task = await api(page, "tasks", { ...emptyTask("Platform"), title: "Implement pagination", notes: "Add tests." })
  await withSampleSessions(page)
  let operation: any = null
  let cancelFailure = true
  await page.route("**/api/operations", async (route) => {
    if (route.request().method() === "POST") {
      const input = route.request().postDataJSON()
      cancelFailure = true
      expect(input.taskId).toBe(task.id)
      expect(input.directory).toBe("/projects/platform")
      operation = { id: "run-test", taskId: task.id, title: task.title, provider: input.provider,
        directory: input.directory, status: "running", sessionId: "agent-session-123", output: "Reading the project…",
        startedAt: new Date().toISOString(), updatedAt: new Date().toISOString() }
      return route.fulfill({ status: 202, json: operation })
    }
    await route.fulfill({ json: operation ? [operation] : [] })
  })
  await page.route("**/api/operations/run-test", route => route.fulfill({ json: operation }))
  await page.route("**/api/operations/run-test/cancel", route => {
    if (cancelFailure) {
      cancelFailure = false
      return route.fulfill({ status: 500, json: { error: "Could not stop the run. Try again." } })
    }
    operation = { ...operation, status: "cancelled", finishedAt: new Date().toISOString() }
    return route.fulfill({ json: { ok: true } })
  })
  await page.reload()
  for (const provider of ["Codex", "Claude Code"]) {
    await page.getByRole("navigation", { name: "Main navigation" }).getByRole("button", { name: "Today" }).click()
    await page.getByRole("button", { name: "Actions for Implement pagination" }).click()
    await page.getByRole("menuitem", { name: "Run with agent" }).hover()
    await page.getByRole("menuitem", { name: provider, exact: true }).click()
    await expect(page.getByRole("dialog").getByRole("heading", { name: `Run with ${provider}` })).toBeVisible()
    await expect(page.getByLabel("Project folder")).toHaveValue("/projects/platform")
    await page.getByRole("button", { name: "Run task", exact: true }).click()
    await expect(page.getByRole("heading", { name: "View operations", exact: true })).toBeVisible()
    await expect(page.getByLabel("Session output")).toContainText("Reading the project…")
    expect(operation.provider).toBe(provider === "Codex" ? "codex" : "claude")
    await page.getByRole("button", { name: "Stop run" }).click()
    await expect(page.getByRole("alert")).toHaveText("Could not stop the run. Try again.")
    await expect(page.getByRole("button", { name: "Stop run" })).toBeEnabled()
    await page.getByRole("button", { name: "Stop run" }).click()
    await expect(page.locator(".operation-detail .operation-status")).toHaveText("Stopped")
    await expect(page.getByRole("alert")).toHaveCount(0)
    await expect(page.getByRole("button", { name: "Stopped", exact: true })).toBeDisabled()
    await page.getByRole("button", { name: "All operations" }).click()
    await expect(page.getByRole("button", { name: /Implement pagination.*Stopped/ })).toBeVisible()
  }
  await page.screenshot({ path: "../build/preview-operations.png", fullPage: true })
})

test("run completion notifications show each outcome once and open the result", async ({ page }) => {
  const make = (id: string, status: Operation["status"]): Operation => ({
    id, taskId: id, title: `Task ${id}`, provider: "codex", directory: "/projects/test", status,
    sessionId: `session-${id}`, output: `Result for ${id}`, startedAt: new Date().toISOString(), updatedAt: new Date().toISOString(),
  })
  let operations = [make("historical", "succeeded"), make("success", "running"), make("failure", "running"), make("stopped", "running")]
  let listRequests = 0
  await page.route("**/api/operations", route => {
    listRequests++
    return route.fulfill({ json: operations })
  })
  await page.route(/\/api\/operations\/[^/]+$/, route => route.fulfill({ json: operations.find(op => route.request().url().endsWith(`/${op.id}`)) }))
  await page.reload()
  await expect(page.getByRole("navigation").getByRole("button", { name: /View operations/ }).locator(".nav-count")).toHaveText("3")
  await expect(page.locator(".toast")).toHaveCount(0)
  operations = operations.map(op => ({ ...op, status: op.id === "failure" ? "failed" : op.id === "stopped" ? "cancelled" : "succeeded" }))
  const succeeded = page.locator(".toast").filter({ hasText: "Codex run finished: Task success" })
  const failed = page.locator(".toast").filter({ hasText: "Codex run failed: Task failure" })
  await expect(succeeded).toBeVisible()
  await expect(failed).toBeVisible()
  await expect(page.locator(".toast")).toHaveCount(2)
  await succeeded.getByRole("button", { name: "View run" }).click()
  await expect(page.getByLabel("Session output")).toHaveText("Result for success")
  await failed.getByRole("button", { name: "Dismiss notification for Task failure" }).click()
  const previousRequests = listRequests
  await expect.poll(() => listRequests).toBeGreaterThan(previousRequests)
  await expect(page.locator(".toast")).toHaveCount(0)
  // Native notification clicks use this fragment, including after an app launch.
  await page.goto("/#operation=failure")
  await expect(page.getByLabel("Session output")).toHaveText("Result for failure")
  await expect(page.locator(".operation-detail .operation-status")).toHaveText("Failed")
  await expect(page.locator(".toast")).toHaveCount(0)
})

test("a run that fails during launch still notifies", async ({ page }) => {
  const task = await api(page, "tasks", { ...emptyTask(), title: "Immediate failure", directory: "/projects/test" })
  let operation: Operation | null = null
  await page.route("**/api/operations", route => {
    if (route.request().method() === "POST") {
      operation = { id: "instant-run", taskId: task.id, turn: 1, title: task.title, provider: "codex", directory: task.directory,
        status: "failed", sessionId: "", error: "Could not start the agent", startedAt: new Date().toISOString(), updatedAt: new Date().toISOString() }
      return route.fulfill({ json: operation })
    }
    return route.fulfill({ json: operation ? [operation] : [] })
  })
  await page.route("**/api/operations/instant-run", route => route.fulfill({ json: operation }))
  await page.reload()
  await page.getByRole("button", { name: "Actions for Immediate failure" }).click()
  await page.getByRole("menuitem", { name: "Run with agent" }).hover()
  await page.getByRole("menuitem", { name: "Codex", exact: true }).click()
  await page.getByRole("button", { name: "Run task", exact: true }).click()
  await expect(page.locator(".toast").filter({ hasText: "Codex run failed: Immediate failure" })).toHaveCount(1)
  await expect(page.getByRole("textbox", { name: "Continue agent session" })).toBeDisabled()
  await expect(page.getByText("No session was recorded. Start a new run from the task.")).toBeVisible()
})

test("agent output follows updates and accepts follow-up prompts in the same session", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  const task = await api(page, "tasks", { ...emptyTask(), title: "Continue this task" })
  let operation: Operation = { id: "live-run", turn: 1, taskId: task.id, title: task.title, provider: "codex", directory: "/projects/test",
    status: "running", sessionId: "same-session", output: "Initial output\n".repeat(100), startedAt: new Date().toISOString(), updatedAt: new Date().toISOString() }
  await page.route("**/api/operations", route => route.fulfill({ json: [operation] }))
  await page.route("**/api/operations/live-run", route => route.fulfill({ json: operation }))
  let failContinuation = true
  const prompts: string[] = []
  await page.route("**/api/operations/live-run/continue", route => {
    if (failContinuation) {
      failContinuation = false
      return route.fulfill({ status: 400, json: { error: "Could not continue. Try again." } })
    }
    const input = route.request().postDataJSON()
    expect(input.turn).toBe(operation.turn)
    prompts.push(input.prompt)
    operation = { ...operation, turn: input.turn + 1, status: "running", output: operation.output + `\n> ${input.prompt}\n`, updatedAt: new Date().toISOString() }
    return route.fulfill({ status: 202, json: operation })
  })
  await page.reload()
  await page.getByRole("navigation").getByRole("button", { name: /View operations/ }).click()
  await page.getByRole("button", { name: /Continue this task.*Running/ }).click()
  const output = page.getByLabel("Session output")
  const remaining = () => output.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)
  await expect.poll(remaining).toBeLessThan(1)
  operation = { ...operation, output: operation.output + "First streamed message\n" }
  await expect(output).toContainText("First streamed message")
  await expect.poll(remaining).toBeLessThan(1)
  await output.evaluate(el => { el.scrollTop = 0 })
  await expect.poll(() => output.evaluate(el => el.scrollTop)).toBe(0)
  operation = { ...operation, output: operation.output + "Read earlier output without jumping\n" }
  await expect(output).toContainText("Read earlier output without jumping")
  expect(await output.evaluate(el => el.scrollTop)).toBe(0)
  await output.evaluate(el => { el.scrollTop = el.scrollHeight })
  await expect.poll(remaining).toBeLessThan(1)
  operation = { ...operation, status: "succeeded", output: operation.output + "First turn complete\n" }
  await expect(output).toContainText("First turn complete")
  const prompt = page.getByRole("textbox", { name: "Continue agent session" })
  await expect(prompt).toBeFocused()
  await expect.poll(remaining).toBeLessThan(1)
  await prompt.fill("Please test the changes.")
  await prompt.press("Enter")
  await expect(page.getByRole("alert").filter({ hasText: "Could not continue. Try again." })).toBeVisible()
  await expect(prompt).toHaveValue("Please test the changes.")
  await prompt.press("Enter")
  await expect(page.locator(".operation-detail .operation-status")).toHaveText("Running")
  await expect(prompt).toHaveCount(0)
  await expect(output).toContainText("> Please test the changes.")
  expect(operation.sessionId).toBe("same-session")
  operation = { ...operation, status: "cancelled", output: operation.output + "Second turn stopped\n" }
  await expect(prompt).toBeFocused()
  await prompt.fill("One more check.")
  await prompt.press("Enter")
  await expect(page.locator(".operation-detail .operation-status")).toHaveText("Running")
  operation = { ...operation, status: "succeeded", output: operation.output + "Final turn complete\n" }
  await expect(prompt).toBeFocused()
  await expect(output).toContainText("Final turn complete")
  await expect(page.locator(".toast").filter({ hasText: "Codex run finished: Continue this task" })).toHaveCount(1)
  expect(prompts).toEqual(["Please test the changes.", "One more check."])
  await page.screenshot({ path: "../build/preview-operation-prompt.png", fullPage: true, caret: "initial" })
})

test("drag reorder persists across reloads and keyboard reordering works", async ({ page }) => {
  for (const title of ["First task", "Second task", "Third task"]) await api(page, "tasks", { ...emptyTask(), title })
  await page.reload()
  const first = page.locator(".task-row").filter({ hasText: "First task" })
  const third = page.locator(".task-row").filter({ hasText: "Third task" })
  await third.hover()
  await third.getByRole("button", { name: "Reorder Third task" }).dragTo(first, { targetPosition: { x: 35, y: 3 } })
  await expect(page.locator(".task-title")).toHaveText(["Third task", "First task", "Second task"])
  await page.reload()
  await expect(page.locator(".task-title")).toHaveText(["Third task", "First task", "Second task"])
  await page.getByRole("button", { name: "Reorder Third task" }).focus()
  await page.keyboard.press("Alt+ArrowDown")
  await expect(page.locator(".task-title")).toHaveText(["First task", "Third task", "Second task"])
})

test("only the task list scrolls and switching tabs keeps controls in place", async ({ page }) => {
  for (let i = 0; i < 25; i++) await api(page, "tasks", { ...emptyTask(), title: `Scrollable task ${i}` })
  await page.reload()
  const heading = page.getByRole("heading", { name: "Today", exact: true })
  const input = page.getByRole("textbox", { name: "Quick add task" })
  const beforeHeading = await heading.boundingBox()
  const beforeInput = await input.boundingBox()
  const list = page.getByRole("region", { name: "Task list" })
  await list.evaluate(element => { element.scrollTop = element.scrollHeight })
  expect(await list.evaluate(element => element.scrollTop)).toBeGreaterThan(0)
  expect(await heading.boundingBox()).toEqual(beforeHeading)
  expect(await input.boundingBox()).toEqual(beforeInput)
  for (const tab of ["In progress", "Done", "To do"]) {
    await page.getByRole("tab", { name: tab, exact: true }).click()
    expect(await heading.boundingBox()).toEqual(beforeHeading)
    expect(await input.boundingBox()).toEqual(beforeInput)
  }
  expect(await page.evaluate(() => document.documentElement.scrollHeight <= window.innerHeight)).toBe(true)
})

test("editor clicks place the caret at the intended text position", async ({ page }) => {
  await page.getByRole("button", { name: "New task", exact: true }).click()
  const dialog = await page.getByRole("dialog").boundingBox()
  const viewport = page.viewportSize()!
  expect(Math.abs(dialog!.x + dialog!.width / 2 - viewport.width / 2)).toBeLessThan(1)
  expect(Math.abs(dialog!.y + dialog!.height / 2 - viewport.height / 2)).toBeLessThan(1)
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  await fillTask(page, "Alpha line\n\nBravo line")
  const paragraph = input.locator("p").filter({ hasText: "Bravo line" })
  const position = await paragraph.evaluate(element => {
    const style = getComputedStyle(element)
    const context = document.createElement("canvas").getContext("2d")!
    context.font = `${style.fontWeight} ${style.fontSize} ${style.fontFamily}`
    return { x: context.measureText("Bravo ").width, y: parseFloat(style.lineHeight) / 2 }
  })
  await paragraph.click({ position })
  expect(await input.evaluate(() => window.getSelection()?.anchorOffset)).toBe(6)
  await page.keyboard.insertText("new ")
  await expect(input.locator("p").last()).toHaveText("Bravo new line")
  await page.screenshot({ path: "../build/preview-editor-caret.png", fullPage: true, caret: "initial" })
})

test("task editor stays compact and centered across window sizes", async ({ page }) => {
  await page.getByRole("button", { name: "New task", exact: true }).click()
  const dialog = page.getByRole("dialog")
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  await fillTask(page, "Release v1.2.x")
  for (const viewport of [
    { width: 1380, height: 880 },
    { width: 700, height: 900 },
    { width: 665, height: 880 },
    { width: 390, height: 844 },
    { width: 900, height: 360 },
  ]) {
    await page.setViewportSize(viewport)
    const bounds = (await dialog.boundingBox())!
    const footer = (await page.locator(".task-editor-footer").boundingBox())!
    expect(bounds.height).toBeLessThan(320)
    expect(Math.abs(bounds.y + bounds.height / 2 - viewport.height / 2)).toBeLessThan(1)
    expect(Math.abs(bounds.x + bounds.width / 2 - viewport.width / 2)).toBeLessThan(1)
    expect(bounds.y + bounds.height - footer.y - footer.height).toBeLessThan(3)
    await expect(page.getByRole("button", { name: "Save task" })).toBeVisible()
  }
  await page.setViewportSize({ width: 665, height: 880 })
  await fillTask(page, "Release v1.2.x\n\n" + "Release notes\n".repeat(100))
  expect((await dialog.boundingBox())!.height).toBeLessThan(320)
  expect(await input.evaluate(element => element.scrollHeight > element.clientHeight)).toBe(true)
  await fillTask(page, "Release v1.2.x")
  await page.screenshot({ path: "../build/preview-compact-editor.png", fullPage: true })
  await page.mouse.click(4, 4)
  await expect(dialog).not.toBeVisible()
})

test("system appearance updates the workspace, editor and menus without losing a draft", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await api(page, "tasks", { ...emptyTask(), title: "Review the release notes" })
  await page.reload()
  await expect(page.getByText("Review the release notes", { exact: true })).toBeVisible()
  await expect(page.locator("html")).toHaveCSS("background-color", "rgb(24, 24, 24)")
  await page.screenshot({ path: "../build/preview-dark-workspace.png", fullPage: true })
  await page.getByRole("button", { name: "New task", exact: true }).click()
  const dialog = page.getByRole("dialog")
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  const draft = "Release Exo\n\nReview the release notes."
  await fillTask(page, draft)
  const bounds = await dialog.boundingBox()

  for (const colorScheme of ["dark", "light", "dark"] as const) {
    await page.emulateMedia({ colorScheme })
    const dark = colorScheme === "dark"
    const text = dark ? "rgb(232, 232, 232)" : "rgb(38, 38, 38)"
    await expect(page.locator("html")).toHaveCSS("background-color", dark ? "rgb(24, 24, 24)" : "rgb(250, 250, 250)")
    await expect(page.locator(".sidebar")).toHaveCSS("background-color", dark ? "rgb(30, 30, 30)" : "rgb(243, 243, 243)")
    await expect(dialog).toHaveCSS("background-color", dark ? "rgb(34, 34, 34)" : "rgb(255, 255, 255)")
    await expect(input).toHaveCSS("color", text)
    await expect(input).toHaveCSS("caret-color", text)
    await expect(input).toContainText("Review the release notes.")
    await expect(page.getByRole("button", { name: "Save task" })).toHaveCSS("background-color", text)
    expect(await dialog.boundingBox()).toEqual(bounds)
  }
  await page.setViewportSize({ width: 700, height: 900 })
  expect((await dialog.boundingBox())!.height).toBeLessThan(320)
  await page.screenshot({ path: "../build/preview-dark-editor.png", fullPage: true })
  await input.press("Meta+Enter")
  await expect(dialog).not.toBeVisible()
  await page.getByRole("button", { name: "Actions for Release Exo" }).click()
  await expect(page.getByRole("menu")).toHaveCSS("background-color", "rgb(34, 34, 34)")
  await page.getByRole("menuitem", { name: "Edit task", exact: true }).click()
  await expect(input).toContainText("Review the release notes.")
  expect((await dialog.boundingBox())!.height).toBeLessThan(320)
  await page.keyboard.press("Escape")
  await page.setViewportSize({ width: 1380, height: 880 })
  await page.getByRole("button", { name: "Integrations", exact: true }).click()
  await expect(page.locator('[data-slot="card"]').first()).toHaveCSS("background-color", "rgb(34, 34, 34)")
})

test("Markdown renders as one document and preserves parent task metadata", async ({ page }) => {
  const task = await api(page, "tasks", {
    ...emptyTask("Platform"), title: "Release v1.2.x", status: "working", priority: 2,
    notes: "- [ ] Data sync issues\n- [ ] Server group issues\n- [ ] Server UI issues",
  })
  await page.reload()
  await page.getByRole("button", { name: /^Release v1.2.x/ }).click()
  const dialog = page.getByRole("dialog")
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  await expect(dialog.getByRole("textbox", { name: "Task", exact: true })).toHaveCount(1)
  await expect(dialog.getByRole("checkbox")).toHaveCount(3)
  await dialog.getByRole("checkbox", { name: "Complete Data sync issues" }).check()
  await page.keyboard.press("ControlOrMeta+z")
  await expect(dialog.getByRole("checkbox", { name: "Complete Data sync issues" })).not.toBeChecked()
  await page.keyboard.press("ControlOrMeta+Shift+z")
  await expect(dialog.getByRole("checkbox", { name: "Complete Data sync issues" })).toBeChecked()
  await input.locator('li[data-type="taskItem"] p').nth(1).click()
  await page.keyboard.press("Meta+ArrowLeft")
  await page.keyboard.press("Meta+Shift+ArrowRight")
  await page.keyboard.insertText("Server grouping issues")
  await expect(input).toContainText("Release v1.2.x")
  expect((await dialog.boundingBox())!.height).toBeLessThan(320)
  await page.screenshot({ path: "../build/preview-checklist.png", fullPage: true })
  await page.getByRole("button", { name: "Save task" }).click()
  const updated = (await api(page, "state")).tasks.find((item: { id: string }) => item.id === task.id)
  expect(updated).toMatchObject({
    title: task.title, status: "working", priority: 2, project: "Platform", startDate: task.startDate,
    notes: "- [x] Data sync issues\n- [ ] Server grouping issues\n- [ ] Server UI issues",
  })
  await page.reload()
  await page.getByRole("button", { name: /^Release v1.2.x/ }).click()
  await expect(dialog.getByRole("checkbox", { name: "Complete Data sync issues" })).toBeChecked()
  await page.getByRole("button", { name: "Edit Markdown" }).click()
  await expect(dialog.getByRole("textbox", { name: "Task Markdown", exact: true })).toHaveValue(`Release v1.2.x\n\n${updated.notes}`)
  await page.getByRole("button", { name: "Show formatted text" }).click()
  await expect(dialog.getByRole("checkbox")).toHaveCount(3)
})

test("typing Markdown converts to elements and keyboard navigation crosses blocks", async ({ page }) => {
  await page.getByRole("button", { name: "New task", exact: true }).click()
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  await input.pressSequentially("Release Alpha")
  await page.keyboard.press("Enter")
  await input.pressSequentially("## Plan")
  await page.keyboard.press("Enter")
  await input.pressSequentially("- [ ] Review changes")
  await page.keyboard.press("Enter")
  await input.pressSequentially("Publish release")
  await expect(input.locator("h2")).toHaveText("Plan")
  await expect(input.getByRole("checkbox")).toHaveCount(2)
  await page.keyboard.press("Tab")
  await expect(input.locator('li[data-type="taskItem"] ul[data-type="taskList"]')).toHaveCount(1)
  await page.keyboard.press("Shift+Tab")
  await expect(input.locator('li[data-type="taskItem"] ul[data-type="taskList"]')).toHaveCount(0)
  await page.keyboard.press("Meta+ArrowLeft")
  await page.keyboard.press("ArrowUp")
  await expect.poll(() => input.evaluate(element => (element as any).editor.state.selection.$from.parent.textContent)).toBe("Review changes")
  await page.keyboard.press("ControlOrMeta+Shift+x")
  await expect(input.getByRole("checkbox", { name: "Complete Review changes" })).toBeChecked()
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("Meta+ArrowRight")
  await expect.poll(() => input.evaluate(element => { const point = (element as any).editor.state.selection.$from; return [point.parent.textContent, point.parentOffset] })).toEqual(["Publish release", 15])
  await page.keyboard.press("Enter")
  await page.keyboard.press("Enter")
  await input.pressSequentially("Keep the rollout gradual.")
  // Selection works across headings and list items in the same document.
  await page.keyboard.press("ControlOrMeta+a")
  expect(await input.evaluate(() => window.getSelection()?.toString())).toContain("Release Alpha")
  expect(await input.evaluate(() => window.getSelection()?.toString())).toContain("Keep the rollout gradual.")
  await page.keyboard.press("ArrowRight")
  await page.keyboard.press("ControlOrMeta+Enter")
  const tasks = (await api(page, "state")).tasks
  expect(tasks).toHaveLength(1)
  expect(tasks[0].title).toBe("Release Alpha")
  expect(tasks[0].notes).toContain("## Plan")
  expect(tasks[0].notes).toContain("- [x] Review changes")
  expect(tasks[0].notes).toContain("- [ ] Publish release")
  expect(tasks[0].notes).toContain("Keep the rollout gradual.")
})

test("loading screen animates dithering, respects reduced motion and handles retry", async ({ page }) => {
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route("**/api/state", async route => { await gate; await route.continue() })
  await page.emulateMedia({ reducedMotion: "no-preference", colorScheme: "light" })
  await page.reload()
  try {
    await expect(page.getByRole("status")).toContainText("Loading your workspace")
    const canvas = page.locator(".startup-dither")
    const frame = () => canvas.evaluate(element => (element as HTMLCanvasElement).toDataURL())
    const before = await frame()
    await expect.poll(frame).not.toBe(before)
    await page.screenshot({ path: "../build/preview-loading-light.png", fullPage: true })
    await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" })
    await expect(page.locator(".startup-screen")).toHaveCSS("background-color", "rgb(24, 24, 24)")
    // Allow the media change handlers to draw their single static frame.
    await page.waitForTimeout(100)
    const still = await frame()
    await page.waitForTimeout(250)
    expect(await frame()).toBe(still)
    await page.screenshot({ path: "../build/preview-loading-dark.png", fullPage: true })
  } finally {
    release()
  }
  await expect(page.getByRole("heading", { name: "Today", exact: true })).toBeVisible()
  await expect(page.locator(".startup-screen")).not.toBeVisible()
  await page.unroute("**/api/state")
  await page.route("**/api/state", route => route.fulfill({ status: 503, json: { error: "Workspace unavailable" } }))
  await page.reload()
  await expect(page.getByRole("alert")).toContainText("Workspace unavailable")
  await page.unroute("**/api/state")
  await page.getByRole("button", { name: "Try again" }).click()
  await expect(page.getByRole("heading", { name: "Today", exact: true })).toBeVisible()
})

test("pasted Markdown renders headings, formatting, lists, code, tables and images and survives reopening", async ({ page }) => {
  await page.getByRole("button", { name: "New task", exact: true }).click()
  const markdown = [
    "Release document", "", "# Overview", "", "## Scope", "", "### Details", "", "#### Fourth", "", "##### Fifth", "", "###### Sixth", "",
    "**Bold** and *italic*, ~~removed~~, `inline code`, and [documentation](https://example.com/docs).", "",
    "- Parent item", "  - Nested item", "", "1. First step", "2. Second step", "",
    "- [ ] Review changes", "- [x] Write release notes", "", "> Ship carefully.", "",
    "```ts", "const ready = true", "// - [ ] code, not a checkbox", "```", "",
    "| Area | Status |", "| --- | --- |", "| API | Ready |", "", "---", "", "![App logo](/favicon.svg)",
  ].join("\n")
  await pasteTask(page, markdown)
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  for (let level = 1; level <= 6; level++) await expect(input.locator(`h${level}`)).toHaveCount(1)
  await expect(input.locator("strong")).toHaveText("Bold")
  await expect(input.locator("em")).toHaveText("italic")
  await expect(input.locator("s")).toHaveText("removed")
  await expect(input.getByRole("link", { name: "documentation" })).toHaveAttribute("href", "https://example.com/docs")
  await expect(input.locator("ul:not([data-type]) ul")).toHaveCount(1)
  await expect(input.locator("ol > li")).toHaveCount(2)
  await expect(input.getByRole("checkbox")).toHaveCount(2)
  await expect(input.locator("blockquote")).toContainText("Ship carefully.")
  await expect(input.locator("pre code")).toContainText("const ready = true")
  await expect(input.getByRole("columnheader", { name: "Area" })).toBeVisible()
  await expect(input.getByRole("cell", { name: "Ready" })).toBeVisible()
  await expect(input.locator("hr")).toHaveCount(1)
  await expect(input.getByRole("img", { name: "App logo" })).toHaveAttribute("src", "/favicon.svg")
  await page.screenshot({ path: "../build/preview-markdown.png", fullPage: true })
  await page.getByRole("button", { name: "Save task" }).click()
  const tasks = (await api(page, "state")).tasks
  expect(tasks).toHaveLength(1)
  expect(tasks[0].notes).toContain("## Scope")
  expect(tasks[0].notes).toContain("**Bold**")
  expect(tasks[0].notes).toContain("```ts")
  expect(tasks[0].notes).toContain("![App logo](/favicon.svg)")
  await page.reload()
  await page.getByRole("button", { name: /^Release document/ }).click()
  await expect(input.locator("table")).toHaveCount(1)
  await expect(input.getByRole("checkbox", { name: "Complete Write release notes" })).toBeChecked()
  await page.emulateMedia({ colorScheme: "dark" })
  await expect(input).toHaveCSS("color", "rgb(232, 232, 232)")
  await page.screenshot({ path: "../build/preview-markdown-dark.png", fullPage: true })
})

test("keyboard navigation moves through table cells and Markdown shortcuts format inline text", async ({ page }) => {
  await page.getByRole("button", { name: "New task", exact: true }).click()
  const input = page.getByRole("textbox", { name: "Task", exact: true })
  await input.pressSequentially("Keyboard test")
  await page.keyboard.press("Enter")
  await input.pressSequentially("**bold** *italic* `code` [docs](https://example.com/docs) ")
  await expect(input.locator("strong")).toHaveText("bold")
  await expect(input.locator("em")).toHaveText("italic")
  await expect(input.locator("code")).toHaveText("code")
  await expect(input.getByRole("link", { name: "docs" })).toHaveAttribute("href", "https://example.com/docs")
  await page.keyboard.press("Enter")
  await pasteTask(page, "| Key | Value |\n| --- | --- |\n| One | Two |")
  await input.getByRole("cell", { name: "One", exact: true }).click()
  await page.keyboard.press("Tab")
  await page.keyboard.insertText("Updated")
  await expect(input.getByRole("cell", { name: "Updated", exact: true })).toBeVisible()
  await page.keyboard.press("Shift+Tab")
  await page.keyboard.insertText("Previous")
  await expect(input.getByRole("cell", { name: "Previous", exact: true })).toBeVisible()
})
