package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/adaptive-scale/listodo/internal/model"
	"github.com/adaptive-scale/listodo/internal/sessions"
	"github.com/adaptive-scale/listodo/internal/store"
)

type UI struct {
	App                                 fyne.App
	Window                              fyne.Window
	Store                               *store.Store
	tasks                               []model.Task
	sessions                            []model.Session
	settings                            model.Settings
	page, project, query, activityRange string
	filter                              string
	content, sidebar                    *fyne.Container
	search                              *widget.Entry
	syncing, organizing                 bool
	lastSync                            time.Time
	syncStatus                          string
	ctx                                 context.Context
	cancel                              context.CancelFunc
}

func New(a fyne.App, s *store.Store) (*UI, error) {
	a.Settings().SetTheme(appTheme{})
	ctx, cancel := context.WithCancel(context.Background())
	u := &UI{App: a, Store: s, page: "Today", filter: "All", activityRange: "Today", ctx: ctx, cancel: cancel}
	u.Window = a.NewWindow("Listodo")
	u.Window.Resize(fyne.NewSize(1240, 820))
	u.Window.SetMaster()
	u.content = container.NewStack()
	u.sidebar = container.NewVBox()
	u.search = widget.NewEntry()
	u.search.SetPlaceHolder("Search tasks or sessions…")
	u.search.OnChanged = func(q string) { u.query = strings.ToLower(q); u.renderContent() }
	newButton := widget.NewButtonWithIcon("New task", theme.ContentAddIcon(), func() {
		u.editTask(model.Task{Status: "todo", Priority: 1, DueDate: model.Day(time.Now()), Project: u.project})
	})
	newButton.Importance = widget.HighImportance
	top := container.NewBorder(nil, nil, nil, container.NewHBox(widget.NewButtonWithIcon("Organize", theme.ConfirmIcon(), u.organizeDialog), newButton), u.search)
	main := container.NewBorder(container.NewVBox(pad(top, 20), widget.NewSeparator()), nil, nil, nil, u.content)
	bg := canvas.NewRectangle(white)
	side := container.NewStack(bg, pad(container.NewVScroll(u.sidebar), 18))
	u.Window.SetContent(container.NewBorder(nil, nil, fixedWidth(side, 210), nil, main))
	u.Window.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("File", fyne.NewMenuItem("New task", func() { u.editTask(model.Task{Status: "todo", Priority: 1, DueDate: model.Day(time.Now())}) }), fyne.NewMenuItem("Export backup…", u.export)), fyne.NewMenu("Help", fyne.NewMenuItem("About Listodo", func() {
		dialog.ShowInformation("Listodo", "A little focus. A lot of progress.\n\nBuilt with Go + Fyne. Tasks stay in SQLite on this computer.\nCodex / Claude organize only when you ask.", u.Window)
	}))))
	u.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierSuper}, func(fyne.Shortcut) {
		u.editTask(model.Task{Status: "todo", Priority: 1, DueDate: model.Day(time.Now())})
	})
	u.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierSuper}, func(fyne.Shortcut) { u.Window.Canvas().Focus(u.search) })
	if err := u.reload(); err != nil {
		cancel()
		return nil, err
	}
	u.Window.SetOnClosed(cancel)
	return u, nil
}

func (u *UI) Start() {
	u.syncSessions()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-u.ctx.Done():
				return
			case <-ticker.C:
				fyne.Do(func() {
					if u.settings.AutoSync && u.page != "Integrations" {
						u.syncSessions()
					}
					if u.page != "Integrations" {
						u.renderContent()
					}
				})
			}
		}
	}()
}

func (u *UI) reload() error {
	var err error
	if u.tasks, err = u.Store.Tasks(); err != nil {
		return err
	}
	if u.sessions, err = u.Store.Sessions(); err != nil {
		return err
	}
	if u.settings, err = u.Store.Settings(); err != nil {
		return err
	}
	sort.Slice(u.sessions, func(i, j int) bool { return u.sessions[i].UpdatedAt.After(u.sessions[j].UpdatedAt) })
	u.renderSidebar()
	u.renderContent()
	return nil
}
func (u *UI) refresh() {
	if err := u.reload(); err != nil {
		dialog.ShowError(err, u.Window)
	}
}
func (u *UI) save(t model.Task) bool {
	if _, err := u.Store.SaveTask(t); err != nil {
		dialog.ShowError(err, u.Window)
		return false
	}
	u.refresh()
	return true
}
func (u *UI) task(id string) (model.Task, bool) {
	for _, t := range u.tasks {
		if t.ID == id {
			return t, true
		}
	}
	return model.Task{}, false
}
func (u *UI) navigate(page, project string) {
	u.page = page
	u.project = project
	u.filter = "All"
	u.renderSidebar()
	u.renderContent()
}

func (u *UI) renderSidebar() {
	brand := container.NewHBox(pill("✓", green), text("listodo", 26, ink, true))
	objects := []fyne.CanvasObject{brand, small("MAKE ROOM FOR WHAT MATTERS"), spacer(24)}
	navs := []struct {
		name string
		icon fyne.Resource
	}{{"Today", theme.HomeIcon()}, {"All tasks", theme.ListIcon()}, {"Upcoming", theme.CalendarIcon()}, {"Completed", theme.ConfirmIcon()}, {"Activity", theme.HistoryIcon()}}
	for _, nav := range navs {
		n := nav
		count := 0
		for _, t := range u.tasks {
			if n.name == "Today" && t.IsToday(time.Now()) && t.Status != "done" {
				count++
			}
			if n.name == "All tasks" && t.Status != "done" {
				count++
			}
		}
		label := n.name
		if count > 0 {
			label = fmt.Sprintf("%s   %d", label, count)
		}
		b := widget.NewButtonWithIcon(label, n.icon, func() { u.navigate(n.name, "") })
		b.Alignment = widget.ButtonAlignLeading
		if u.page == n.name && u.project == "" {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.LowImportance
		}
		objects = append(objects, b)
	}
	objects = append(objects, spacer(20), small("PROJECTS"))
	projects := map[string]int{}
	for _, t := range u.tasks {
		if t.Project != "" && t.Status != "done" {
			projects[t.Project]++
		}
	}
	keys := []string{}
	for p := range projects {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	for _, p := range keys {
		project := p
		b := widget.NewButtonWithIcon(fmt.Sprintf("%s  ·  %d", short(p, 18), projects[p]), theme.FolderIcon(), func() { u.navigate("All tasks", project) })
		b.Alignment = widget.ButtonAlignLeading
		if u.project == p {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.LowImportance
		}
		objects = append(objects, b)
	}
	if len(keys) == 0 {
		objects = append(objects, small("Projects appear as you add tasks."))
	}
	objects = append(objects, spacer(30), widget.NewSeparator(), widget.NewButtonWithIcon("Integrations", theme.SettingsIcon(), func() { u.navigate("Integrations", "") }), widget.NewButtonWithIcon("Export backup", theme.DownloadIcon(), u.export), spacer(8), small("LOCAL & PERSISTENT"), wrap("Your tasks, on your computer."))
	u.sidebar.Objects = objects
	u.sidebar.Refresh()
}

func (u *UI) renderContent() {
	if u.content == nil {
		return
	}
	var page fyne.CanvasObject
	switch u.page {
	case "Activity":
		page = u.activityPage()
	case "Integrations":
		page = u.settingsPage()
	default:
		page = u.tasksPage()
	}
	u.content.Objects = []fyne.CanvasObject{page}
	u.content.Refresh()
}

func (u *UI) tasksPage() fyne.CanvasObject {
	now := time.Now()
	title := u.page
	if u.project != "" {
		title = u.project
	}
	remaining, working, done := 0, 0, 0
	for _, t := range u.tasks {
		if t.IsToday(now) {
			if t.Status == "done" {
				done++
			} else {
				remaining++
			}
			if t.Status == "working" {
				working++
			}
		}
	}
	heading := container.NewVBox(small(strings.ToUpper(now.Format("Monday, January 2"))), text(title, 32, ink, true))
	if u.page == "Today" {
		heading.Add(wrap("A little focus. A lot of progress."))
	}
	stats := container.NewGridWithColumns(3, card(container.NewVBox(text(fmt.Sprint(remaining), 27, ink, true), small("LEFT TODAY"))), card(container.NewVBox(text(fmt.Sprint(working), 27, green, true), small("IN PROGRESS"))), card(container.NewVBox(text(fmt.Sprint(done), 27, green, true), small("DONE TODAY"))))
	filter := widget.NewSelect([]string{"All", "To do", "In progress", "Done"}, nil)
	filter.SetSelected(u.filter)
	filter.OnChanged = func(v string) { u.filter = v; u.renderContent() }
	quick := widget.NewEntry()
	quick.SetPlaceHolder("+  Add a task for today…")
	quick.OnSubmitted = func(v string) {
		if strings.TrimSpace(v) != "" {
			u.save(model.Task{Title: v, Project: u.project, DueDate: model.Day(now), Priority: 1, Status: "todo"})
		}
	}
	list := container.NewVBox()
	visible := []model.Task{}
	for _, t := range u.tasks {
		if u.project != "" && t.Project != u.project {
			continue
		}
		if u.query != "" && !strings.Contains(strings.ToLower(t.Title+" "+t.Notes+" "+t.Project), u.query) {
			continue
		}
		if u.page == "Today" && !t.IsToday(now) {
			continue
		}
		if u.page == "Upcoming" && (t.Status == "done" || t.DueDate == "" || t.DueDate <= model.Day(now)) {
			continue
		}
		if u.page == "Completed" && t.Status != "done" {
			continue
		}
		if (u.filter == "To do" && t.Status != "todo") || (u.filter == "In progress" && t.Status != "working") || (u.filter == "Done" && t.Status != "done") {
			continue
		}
		visible = append(visible, t)
	}
	sort.SliceStable(visible, func(i, j int) bool {
		a, b := visible[i], visible[j]
		if u.page == "Today" && taskGroup(a, now) != taskGroup(b, now) {
			return taskGroup(a, now) < taskGroup(b, now)
		}
		if a.Status == "done" && b.Status != "done" {
			return false
		}
		if b.Status == "done" && a.Status != "done" {
			return true
		}
		if a.EffectivePriority(now) != b.EffectivePriority(now) {
			return a.EffectivePriority(now) > b.EffectivePriority(now)
		}
		if a.DueDate != b.DueDate {
			if a.DueDate == "" {
				return false
			}
			if b.DueDate == "" {
				return true
			}
			return a.DueDate < b.DueDate
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	if len(visible) == 0 {
		message := "Nothing here yet."
		detail := "Add a task above, or describe your plans to the organizer."
		if u.page == "Today" {
			message = "Make a little space for today."
			detail = "Add your first task, or bring a task from All tasks into today."
		}
		if u.query != "" {
			message = "No matches."
			detail = "Try another search or clear the filters."
		}
		list.Add(card(container.NewVBox(spacer(18), text(message, 21, ink, true), wrap(detail), spacer(10), widget.NewButton("Describe your tasks", u.organizeDialog), spacer(18))))
	}
	lastGroup := ""
	for _, t := range visible {
		task := t
		group := "TO DO"
		if task.OverdueDays(now) > 0 {
			group = "NEEDS ATTENTION"
		} else if task.Status == "working" {
			group = "IN PROGRESS"
		} else if task.Status == "done" {
			group = "COMPLETED"
		}
		if u.page == "Today" && group != lastGroup {
			list.Add(spacer(8))
			list.Add(small(group))
			lastGroup = group
		}
		list.Add(u.taskCard(task))
	}
	bar := container.NewBorder(nil, nil, text(fmt.Sprintf("%d tasks", len(visible)), 13, muted, false), fixedWidth(filter, 155))
	mainObjects := []fyne.CanvasObject{heading, spacer(10)}
	if u.page == "Today" {
		mainObjects = append(mainObjects, stats, spacer(8))
	}
	mainObjects = append(mainObjects, quick, bar, list)
	main := container.NewVScroll(container.NewVBox(mainObjects...))
	main.SetMinSize(fyne.NewSize(530, 1))
	if u.page == "Today" {
		return pad(container.NewBorder(nil, nil, nil, pad(fixedWidth(u.todayRail(), 255), 10), main), 24)
	}
	return pad(main, 24)
}

func (u *UI) taskCard(t model.Task) fyne.CanvasObject {
	now := time.Now()
	check := widget.NewCheck("", nil)
	check.SetChecked(t.Status == "done")
	check.OnChanged = func(done bool) {
		current, ok := u.task(t.ID)
		if !ok {
			return
		}
		if done {
			current.Status = "done"
		} else {
			current.Status = "todo"
		}
		u.save(current)
	}
	title := wrap(t.Title)
	title.TextStyle.Bold = true
	meta := t.Project
	if meta == "" {
		meta = "Personal"
	}
	if t.DueDate != "" {
		due := t.DueDate
		if due == model.Day(now) {
			due = "Today"
		}
		meta += "  ·  " + due
	}
	if len(t.SessionIDs) > 0 {
		meta += fmt.Sprintf("  ·  %d session(s)", len(t.SessionIDs))
	}
	info := container.NewVBox(title, caption(meta, theme.ColorNamePlaceHolder))
	if days := t.OverdueDays(now); days > 0 {
		info.Add(caption(t.PriorityReason(now), theme.ColorNameError))
	}
	if t.Notes != "" {
		note := strings.Split(t.Notes, "\n")[0]
		if len([]rune(note)) > 85 {
			note = string([]rune(note)[:85]) + "…"
		}
		info.Add(caption(note, theme.ColorNamePlaceHolder))
	}
	edit := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		current, ok := u.task(t.ID)
		if ok {
			u.editTask(current)
		}
	})
	edit.Importance = widget.LowImportance
	actions := container.NewHBox(edit)
	priorityColor := muted
	if t.EffectivePriority(now) >= 2 {
		priorityColor = red
	}
	if t.Status != "done" {
		actions.Add(pill(model.PriorityName(t.EffectivePriority(now)), priorityColor))
		label := "Start"
		icon := theme.MediaPlayIcon()
		if t.Status == "working" {
			label = "Pause"
			icon = theme.MediaPauseIcon()
		}
		b := widget.NewButtonWithIcon(label, icon, func() {
			current, ok := u.task(t.ID)
			if !ok {
				return
			}
			if current.Status == "working" {
				current.Status = "todo"
			} else {
				current.Status = "working"
			}
			u.save(current)
		})
		b.Importance = widget.LowImportance
		actions.Add(b)
	} else {
		actions.Add(pill("Done", green))
	}
	return card(container.NewBorder(nil, nil, container.NewCenter(check), container.NewCenter(actions), container.NewThemeOverride(info, compactTheme{})))
}

func taskGroup(t model.Task, now time.Time) int {
	if t.Status == "done" {
		return 3
	}
	if t.OverdueDays(now) > 0 {
		return 0
	}
	if t.Status == "working" {
		return 1
	}
	return 2
}

func (u *UI) todayRail() fyne.CanvasObject {
	now := time.Now()
	overdue := 0
	for _, t := range u.tasks {
		if t.OverdueDays(now) > 0 {
			overdue++
		}
	}
	content := container.NewVBox(text("Your day, connected", 17, ink, true), wrap("A single view of your tasks and agent sessions."), spacer(10))
	if overdue > 0 {
		content.Add(card(container.NewVBox(pill("PRIORITY, ADJUSTED", red), wrap(fmt.Sprintf("%d overdue task(s) need a little attention.", overdue)), small("+1 priority level per overdue day."))))
	}
	content.Add(spacer(12))
	content.Add(container.NewBorder(nil, nil, text("SESSION ACTIVITY", 11, muted, true), widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), u.syncSessions)))
	count := 0
	for _, s := range u.sessions {
		if model.Day(s.UpdatedAt) != model.Day(now) {
			continue
		}
		session := s
		status := "Idle"
		if s.IsActive(now) {
			status = "Recently active"
		}
		b := widget.NewButton(s.Project, func() { u.sessionDialog(session) })
		b.Importance = widget.LowImportance
		b.Alignment = widget.ButtonAlignLeading
		content.Add(card(container.NewVBox(b, small(strings.ToUpper(s.Harness)+"  ·  "+s.UpdatedAt.Local().Format("15:04")), wrap(short(s.Title, 85)), pill(status, green))))
		count++
		if count == 4 {
			break
		}
	}
	if count == 0 {
		content.Add(wrap("No sessions imported for today. Sync Codex and Claude from Integrations."))
	}
	if u.syncStatus != "" {
		content.Add(wrap(u.syncStatus))
	}
	content.Add(widget.NewButton("View all activity →", func() { u.navigate("Activity", "") }))
	content.Add(spacer(14))
	content.Add(small("A SMALL NOTE"))
	content.Add(wrap("A session ending doesn't mean a task is done. Review the evidence, then check it off."))
	return container.NewVScroll(content)
}

func short(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func (u *UI) editTask(t model.Task) {
	title := widget.NewEntry()
	title.SetText(t.Title)
	title.SetPlaceHolder("What needs to get done?")
	notes := widget.NewMultiLineEntry()
	notes.SetText(t.Notes)
	notes.SetMinRowsVisible(4)
	notes.SetPlaceHolder("Notes or acceptance criteria…")
	project := widget.NewEntry()
	project.SetText(t.Project)
	project.SetPlaceHolder("Personal, project, or repository name")
	due := widget.NewEntry()
	due.SetText(t.DueDate)
	due.SetPlaceHolder("YYYY-MM-DD, or leave empty")
	priority := widget.NewSelect([]string{"Low", "Normal", "High", "Urgent"}, nil)
	priority.SetSelected(model.PriorityName(t.Priority))
	status := widget.NewSelect([]string{"To do", "In progress", "Done"}, nil)
	statuses := map[string]string{"todo": "To do", "working": "In progress", "done": "Done"}
	status.SetSelected(statuses[t.Status])
	form := widget.NewForm(widget.NewFormItem("Task", title), widget.NewFormItem("Notes", notes), widget.NewFormItem("Project", project), widget.NewFormItem("Due date", due), widget.NewFormItem("Priority", priority), widget.NewFormItem("Status", status))
	var d *dialog.CustomDialog
	save := widget.NewButton("Save task", func() {
		draft := t
		draft.Title = title.Text
		draft.Notes = notes.Text
		draft.Project = project.Text
		draft.DueDate = strings.TrimSpace(due.Text)
		for i, name := range []string{"Low", "Normal", "High", "Urgent"} {
			if priority.Selected == name {
				draft.Priority = i
			}
		}
		for key, label := range statuses {
			if status.Selected == label {
				draft.Status = key
			}
		}
		if u.save(draft) {
			d.Hide()
		}
	})
	save.Importance = widget.HighImportance
	buttons := container.NewHBox(layout.NewSpacer())
	if t.ID != "" {
		del := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
			dialog.ShowConfirm("Delete task?", "The task will be removed. Its change history stays in the activity log.", func(ok bool) {
				if ok {
					if err := u.Store.DeleteTask(t.ID); err != nil {
						dialog.ShowError(err, u.Window)
						return
					}
					d.Hide()
					u.refresh()
				}
			}, u.Window)
		})
		del.Importance = widget.DangerImportance
		buttons.Add(del)
	}
	buttons.Add(widget.NewButton("Cancel", func() { d.Hide() }))
	buttons.Add(save)
	if len(t.SessionIDs) > 0 {
		form.Append("Linked sessions", wrap(strings.Join(t.SessionIDs, "\n")))
	}
	d = dialog.NewCustomWithoutButtons("Task details", container.NewVBox(form, buttons), u.Window)
	d.Resize(fyne.NewSize(620, 560))
	d.Show()
	u.Window.Canvas().Focus(title)
}

func (u *UI) syncSessions() {
	if u.syncing {
		return
	}
	u.syncing = true
	u.syncStatus = "Syncing local sessions…"
	settings := u.settings
	u.renderContent()
	go func() {
		report := sessions.Scan(settings, time.Now())
		err := u.Store.SaveSessions(report.Sessions)
		fyne.Do(func() {
			u.syncing = false
			if u.ctx.Err() != nil {
				return
			}
			if err != nil {
				u.syncStatus = "Could not save imported sessions."
				dialog.ShowError(err, u.Window)
			} else {
				u.lastSync = time.Now()
				u.syncStatus = fmt.Sprintf("Synced %d local sessions at %s", len(report.Sessions), u.lastSync.Format("15:04"))
				if len(report.Warnings) > 0 {
					u.syncStatus += "\n" + strings.Join(report.Warnings, "\n")
				}
			}
			u.refresh()
		})
	}()
}

func (u *UI) export() {
	d := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, u.Window)
			return
		}
		if writer == nil {
			return
		}
		defer writer.Close()
		tasks, err := u.Store.Tasks()
		if err != nil {
			dialog.ShowError(err, u.Window)
			return
		}
		ss, err := u.Store.Sessions()
		if err != nil {
			dialog.ShowError(err, u.Window)
			return
		}
		settings, err := u.Store.Settings()
		if err != nil {
			dialog.ShowError(err, u.Window)
			return
		}
		events, err := u.Store.Events("")
		if err != nil {
			dialog.ShowError(err, u.Window)
			return
		}
		value := struct {
			Version    int             `json:"version"`
			ExportedAt time.Time       `json:"exportedAt"`
			Tasks      []model.Task    `json:"tasks"`
			Sessions   []model.Session `json:"sessions"`
			Settings   model.Settings  `json:"settings"`
			Events     []model.Event   `json:"events"`
		}{1, time.Now(), tasks, ss, settings, events}
		enc := json.NewEncoder(writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(value); err != nil {
			dialog.ShowError(err, u.Window)
		} else {
			dialog.ShowInformation("Backup exported", "Your tasks, session links, settings, and recent change history have been exported as JSON. The SQLite database remains the authoritative local copy.", u.Window)
		}
	}, u.Window)
	d.SetFileName("listodo-backup-" + model.Day(time.Now()) + ".json")
	d.Show()
}
