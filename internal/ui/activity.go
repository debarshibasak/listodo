package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/adaptive-scale/listodo/internal/model"
)

func (u *UI) activityPage() fyne.CanvasObject {
	now := time.Now()
	rangeSelect := widget.NewSelect([]string{"Today", "Last 7 days", "All imported"}, nil)
	rangeSelect.SetSelected(u.activityRange)
	rangeSelect.OnChanged = func(v string) { u.activityRange = v; u.renderContent() }
	heading := container.NewVBox(text("Activity", 32, ink, true), wrap("Your work across harnesses, brought together by project."))
	controls := container.NewBorder(nil, nil, rangeSelect, widget.NewButtonWithIcon("Sync sessions", theme.ViewRefreshIcon(), u.syncSessions))
	groups := map[string][]model.Session{}
	count, linked, active := 0, 0, 0
	for _, s := range u.sessions {
		if u.activityRange == "Today" && model.Day(s.UpdatedAt) != model.Day(now) {
			continue
		}
		if u.activityRange == "Last 7 days" && s.UpdatedAt.Before(now.AddDate(0, 0, -7)) {
			continue
		}
		if u.query != "" && !strings.Contains(strings.ToLower(s.Title+" "+s.Summary+" "+s.Project+" "+s.Harness), u.query) {
			continue
		}
		groups[s.Project] = append(groups[s.Project], s)
		count++
		if u.sessionLinked(s.ID) {
			linked++
		}
		if s.IsActive(now) {
			active++
		}
	}
	stats := container.NewGridWithColumns(3, card(container.NewVBox(text(fmt.Sprint(count), 27, ink, true), small("SESSIONS"))), card(container.NewVBox(text(fmt.Sprint(linked), 27, green, true), small("LINKED TO TASKS"))), card(container.NewVBox(text(fmt.Sprint(active), 27, green, true), small("RECENTLY ACTIVE"))))
	body := container.NewVBox(heading, spacer(8), controls, stats)
	if u.syncStatus != "" {
		body.Add(wrap(u.syncStatus))
	}
	keys := []string{}
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return groups[keys[i]][0].UpdatedAt.After(groups[keys[j]][0].UpdatedAt) })
	for _, key := range keys {
		body.Add(spacer(14))
		body.Add(text(fmt.Sprintf("%s  ·  %d sessions", key, len(groups[key])), 17, ink, true))
		for _, v := range groups[key] {
			s := v
			status := "Idle"
			if s.IsActive(now) {
				status = "Recently active"
			}
			if u.sessionLinked(s.ID) {
				status += " · linked"
			}
			open := widget.NewButton("Review session", func() { u.sessionDialog(s) })
			body.Add(card(container.NewBorder(nil, nil, nil, open, container.NewVBox(container.NewHBox(pill(strings.ToUpper(s.Harness), green), small(s.UpdatedAt.Local().Format("Jan 2 · 15:04")), pill(status, muted)), wrap(s.Title), small(short(s.Directory, 95))))))
		}
	}
	if count == 0 {
		body.Add(card(container.NewVBox(text("Bring your work into view.", 22, ink, true), wrap("Listodo reads local Codex and Claude Code transcripts. Sync your sessions, then link them to the tasks you described."), widget.NewButton("Configure integrations", func() { u.navigate("Integrations", "") }))))
	}
	events, err := u.Store.Events(model.Day(now))
	if err == nil && len(events) > 0 {
		body.Add(spacer(16))
		body.Add(text("Task changes today", 18, ink, true))
		for i, e := range events {
			if i >= 30 {
				break
			}
			body.Add(container.NewHBox(small(e.At.Local().Format("15:04")), wrap(e.Title+"  ·  "+e.Action)))
		}
	}
	return pad(container.NewVScroll(body), 24)
}

func (u *UI) sessionLinked(id string) bool {
	for _, t := range u.tasks {
		for _, sid := range t.SessionIDs {
			if sid == id {
				return true
			}
		}
	}
	return false
}

func (u *UI) sessionDialog(s model.Session) {
	status := "Idle / no recent turn in progress"
	if s.IsActive(time.Now()) {
		status = "Recent activity suggests this session is working"
	}
	body := container.NewVBox(container.NewHBox(pill(strings.ToUpper(s.Harness), green), pill(status, muted)), wrap(s.Directory), text("User request", 16, ink, true), wrap(s.Title), text("Latest assistant summary", 16, ink, true))
	if s.Summary == "" {
		body.Add(wrap("No final assistant summary found in this transcript."))
	} else {
		body.Add(wrap(s.Summary))
	}
	body.Add(small("Session: " + s.ID))
	body.Add(small("Last activity: " + s.UpdatedAt.Local().Format(time.RFC1123)))
	options := []string{}
	ids := map[string]string{}
	for _, t := range u.tasks {
		label := t.Title + " [" + t.ID[:8] + "]"
		options = append(options, label)
		ids[label] = t.ID
	}
	selectTask := widget.NewSelect(options, nil)
	selectTask.PlaceHolder = "Choose an existing task"
	link := widget.NewButton("Link selected task", func() {
		t, ok := u.task(ids[selectTask.Selected])
		if !ok {
			return
		}
		for _, id := range t.SessionIDs {
			if id == s.ID {
				dialog.ShowInformation("Already linked", "This session is already linked to that task.", u.Window)
				return
			}
		}
		t.SessionIDs = append(t.SessionIDs, s.ID)
		if u.save(t) {
			dialog.ShowInformation("Session linked", "The session is attached as evidence. The task status has not changed.", u.Window)
		}
	})
	create := widget.NewButton("Create a task from this session", func() {
		u.editTask(model.Task{Title: s.Title, Notes: "From " + s.Harness + " session.\n\n" + s.Summary, Project: s.Project, DueDate: model.Day(time.Now()), Priority: 1, Status: "todo", SessionIDs: []string{s.ID}})
	})
	body.Add(widget.NewSeparator())
	body.Add(text("Connect to your task list", 16, ink, true))
	body.Add(selectTask)
	body.Add(link)
	body.Add(create)
	d := dialog.NewCustom("Session details", "Close", container.NewVScroll(body), u.Window)
	d.Resize(fyne.NewSize(760, 640))
	d.Show()
}
