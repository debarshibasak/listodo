package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/adaptive-scale/listodo/internal/harness"
	"github.com/adaptive-scale/listodo/internal/model"
	"github.com/adaptive-scale/listodo/internal/sessions"
)

func (u *UI) settingsPage() fyne.CanvasObject {
	settings := u.settings
	provider := widget.NewSelect([]string{"codex", "claude"}, nil)
	provider.SetSelected(settings.Harness)
	codexRoot := widget.NewEntry()
	codexRoot.SetText(settings.CodexRoot)
	claudeRoot := widget.NewEntry()
	claudeRoot.SetText(settings.ClaudeRoot)
	codexEnabled := widget.NewCheck("Import Codex sessions", nil)
	codexEnabled.SetChecked(settings.CodexEnabled)
	claudeEnabled := widget.NewCheck("Import Claude Code sessions", nil)
	claudeEnabled.SetChecked(settings.ClaudeEnabled)
	auto := widget.NewCheck("Sync local sessions every minute while Listodo is open", nil)
	auto.SetChecked(settings.AutoSync)
	status := container.NewVBox()
	for _, name := range []string{"codex", "claude"} {
		path, err := harness.Find(name)
		if err != nil {
			status.Add(wrap(strings.ToUpper(name) + " · not detected"))
		} else {
			status.Add(wrap(strings.ToUpper(name) + " · " + path))
		}
	}
	save := widget.NewButton("Save integrations", func() {
		v := model.Settings{Harness: provider.Selected, CodexRoot: strings.TrimSpace(codexRoot.Text), ClaudeRoot: strings.TrimSpace(claudeRoot.Text), CodexEnabled: codexEnabled.Checked, ClaudeEnabled: claudeEnabled.Checked, AutoSync: auto.Checked}
		for _, root := range []struct {
			path    string
			enabled bool
		}{{v.CodexRoot, v.CodexEnabled}, {v.ClaudeRoot, v.ClaudeEnabled}} {
			if root.enabled {
				info, err := os.Stat(sessions.Expand(root.path))
				if err != nil {
					dialog.ShowError(fmt.Errorf("session folder: %w", err), u.Window)
					return
				}
				if !info.IsDir() {
					dialog.ShowInformation("Choose a folder", "The transcript location must be a directory.", u.Window)
					return
				}
			}
		}
		if err := u.Store.SaveSettings(v); err != nil {
			dialog.ShowError(err, u.Window)
			return
		}
		u.refresh()
		u.syncSessions()
	})
	save.Importance = widget.HighImportance
	body := container.NewVBox(text("Integrations", 32, ink, true), wrap("Your existing harness. One place to keep track."), spacer(12), card(container.NewVBox(text("Task organizer", 20, ink, true), wrap("Codex is the default. Listodo invokes your installed CLI using its existing sign-in. Install and sign in from a terminal before organizing."), widget.NewForm(widget.NewFormItem("Harness", provider)), status)), spacer(6), card(container.NewVBox(text("Local session sources", 20, ink, true), wrap("Import reads transcript files locally. It does not invoke a model. The initial scan reads up to 400 transcripts modified in the last 14 days; previously imported sessions stay in SQLite."), codexEnabled, widget.NewForm(widget.NewFormItem("Codex folder", codexRoot)), claudeEnabled, widget.NewForm(widget.NewFormItem("Claude folder", claudeRoot)), auto)), save, spacer(6), card(container.NewVBox(text("Persistent by design", 20, ink, true), wrap("Tasks, completion history, session summaries, links, and integration settings are saved in SQLite after every change. No account or server is required for the task list."), wrap("Database: "+u.Store.Path), wrap("When you click Organize, task descriptions and imported session excerpts are sent to the selected CLI's model provider. All suggestions are reviewed before they change your list."), wrap("Session formats are best-effort. Active means a recent unfinished turn, not confirmed live process status. Currently supported: Codex and Claude Code; other harnesses need an adapter."))))
	return pad(container.NewVScroll(body), 24)
}

func (u *UI) organizeDialog() {
	if u.organizing {
		dialog.ShowInformation("Organizer is running", "Your suggestions are being prepared. You can keep using the task list.", u.Window)
		return
	}
	input := widget.NewMultiLineEntry()
	input.SetMinRowsVisible(6)
	input.SetPlaceHolder("Today I need to finish the login flow, review the API changes, and write release notes by Friday…")
	provider := widget.NewSelect([]string{"codex", "claude"}, nil)
	provider.SetSelected(u.settings.Harness)
	info := wrap("Describe what you want to do, or leave this blank to match today's sessions to your existing tasks. You will review every suggestion before applying it.")
	privacy := wrap("Sends your task list and up to 60 of today's session excerpts to the selected harness. Uses your existing CLI sign-in and plan.")
	var d *dialog.CustomDialog
	run := widget.NewButton("Prepare suggestions", func() {
		if _, err := harness.Find(provider.Selected); err != nil {
			dialog.ShowError(err, u.Window)
			return
		}
		instructions := strings.TrimSpace(input.Text)
		recent := []model.Session{}
		now := time.Now()
		for _, s := range u.sessions {
			if model.Day(s.UpdatedAt) == model.Day(now) {
				recent = append(recent, s)
			}
		}
		if instructions == "" && len(u.tasks) == 0 {
			dialog.ShowInformation("Describe your plans", "Add a task description so the organizer has something to work with.", u.Window)
			return
		}
		// Snapshot the current tasks; no UI fields are read from the worker.
		tasks := append([]model.Task(nil), u.tasks...)
		name := provider.Selected
		u.organizing = true
		d.Hide()
		progress := widget.NewProgressBarInfinite()
		cancelCtx, cancel := context.WithCancel(u.ctx)
		busy := dialog.NewCustom("Organizing with "+name, "Cancel", container.NewVBox(wrap("Preparing task suggestions and checking session evidence…"), progress), u.Window)
		busy.SetOnClosed(cancel)
		busy.Resize(fyne.NewSize(480, 140))
		busy.Show()
		go func() {
			p, err := harness.Organize(cancelCtx, name, instructions, tasks, recent)
			fyne.Do(func() {
				u.organizing = false
				busy.Hide()
				if u.ctx.Err() != nil {
					return
				}
				if err != nil {
					dialog.ShowError(err, u.Window)
					return
				}
				u.proposalDialog(p)
			})
		}()
	})
	run.Importance = widget.HighImportance
	d = dialog.NewCustomWithoutButtons("A clearer plan starts here", container.NewVBox(info, input, widget.NewForm(widget.NewFormItem("Use harness", provider)), privacy, container.NewHBox(layout.NewSpacer(), widget.NewButton("Cancel", func() { d.Hide() }), run)), u.Window)
	d.Resize(fyne.NewSize(640, 450))
	d.Show()
	u.Window.Canvas().Focus(input)
}

func (u *UI) proposalDialog(p harness.Proposal) {
	body := container.NewVBox(wrap(p.Summary), small("REVIEW BEFORE APPLYING"))
	for _, draft := range p.Tasks {
		d := draft
		var add *widget.Button
		add = widget.NewButton("Add task", func() {
			if u.save(model.Task{Title: d.Title, Notes: d.Notes, Project: d.Project, DueDate: d.DueDate, Priority: d.Priority, Status: "todo"}) {
				add.SetText("Added")
				add.Disable()
			}
		})
		add.Importance = widget.HighImportance
		body.Add(card(container.NewVBox(text(d.Title, 16, ink, true), wrap(d.Notes), small(d.Project+"  ·  "+d.DueDate+"  ·  "+model.PriorityName(d.Priority)), add)))
	}
	for _, match := range p.Matches {
		m := match
		t, ok := u.task(m.TaskID)
		if !ok {
			continue
		}
		var apply *widget.Button
		apply = widget.NewButton("Link session & set "+m.SuggestedStatus, func() {
			current, ok := u.task(m.TaskID)
			if !ok {
				return
			}
			found := false
			for _, id := range current.SessionIDs {
				if id == m.SessionID {
					found = true
				}
			}
			if !found {
				current.SessionIDs = append(current.SessionIDs, m.SessionID)
			}
			current.Status = m.SuggestedStatus
			if u.save(current) {
				apply.SetText("Applied")
				apply.Disable()
			}
		})
		body.Add(card(container.NewVBox(text(t.Title, 16, ink, true), wrap(m.Reason), small("Evidence: "+m.SessionID), apply)))
	}
	if len(p.Tasks) == 0 && len(p.Matches) == 0 {
		body.Add(wrap("No changes suggested. Add more detail to your tasks or import more sessions."))
	}
	d := dialog.NewCustom("Your suggested plan", "Done", container.NewVScroll(body), u.Window)
	d.Resize(fyne.NewSize(760, 600))
	d.Show()
}
