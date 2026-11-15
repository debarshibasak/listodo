package ui

import (
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/adaptive-scale/listodo/internal/model"
	"github.com/adaptive-scale/listodo/internal/store"
)

func walk(root fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	seen := map[fyne.CanvasObject]bool{}
	var run func(fyne.CanvasObject)
	run = func(o fyne.CanvasObject) {
		if o == nil || seen[o] {
			return
		}
		seen[o] = true
		visit(o)
		if c, ok := o.(*fyne.Container); ok {
			for _, child := range c.Objects {
				run(child)
			}
		} else if w, ok := o.(fyne.Widget); ok {
			for _, child := range test.WidgetRenderer(w).Objects() {
				run(child)
			}
		}
	}
	run(root)
}

func button(root fyne.CanvasObject, name string) *widget.Button {
	var found *widget.Button
	walk(root, func(o fyne.CanvasObject) {
		if b, ok := o.(*widget.Button); ok && b.Text == name {
			found = b
		}
	})
	return found
}

func TestTaskWorkflowAndViews(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task, err := s.SaveTask(model.Task{Title: "Verify persistent tasks", DueDate: model.Day(time.Now()), Priority: 1, Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(a, s)
	if err != nil {
		t.Fatal(err)
	}
	defer u.cancel()
	u.Window.Show()
	start := button(u.Window.Content(), "Start")
	if start == nil {
		t.Fatal("start control missing")
	}
	test.Tap(start)
	got, ok := u.task(task.ID)
	if !ok || got.Status != "working" {
		t.Fatal("Start did not persist working state")
	}
	var check *widget.Check
	walk(u.Window.Content(), func(o fyne.CanvasObject) {
		if c, ok := o.(*widget.Check); ok {
			check = c
		}
	})
	if check == nil {
		t.Fatal("completion control missing")
	}
	test.TapAt(check, fyne.NewPos(check.MinSize().Width/2, check.Size().Height/2))
	got, _ = u.task(task.ID)
	if got.Status != "done" || got.CompletedAt == nil {
		t.Fatal("completion was not saved")
	}
	for _, page := range []string{"All tasks", "Upcoming", "Completed", "Activity", "Integrations", "Today"} {
		u.navigate(page, "")
		if len(u.content.Objects) != 1 {
			t.Fatalf("%s failed to render", page)
		}
	}
	test.Type(u.search, "impossible search")
	if u.query != "impossible search" {
		t.Fatal("search did not update")
	}
	// Exercise the actual editor and its validation/save path.
	u.search.SetText("")
	newTask := button(u.Window.Content(), "New task")
	if newTask == nil {
		t.Fatal("new task button missing")
	}
	test.Tap(newTask)
	var title *widget.Entry
	var save *widget.Button
	for _, overlay := range u.Window.Canvas().Overlays().List() {
		walk(overlay, func(o fyne.CanvasObject) {
			if e, ok := o.(*widget.Entry); ok && e.PlaceHolder == "What needs to get done?" {
				title = e
			}
			if b, ok := o.(*widget.Button); ok && b.Text == "Save task" {
				save = b
			}
		})
	}
	if title == nil || save == nil {
		t.Fatal("task editor controls missing")
	}
	test.Type(title, "Write release notes")
	test.Tap(save)
	tasks, err := s.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("editor did not save a second task: %d", len(tasks))
	}
}

func TestDashboardCapture(t *testing.T) {
	path := os.Getenv("LISTODO_SCREENSHOT")
	if path == "" {
		t.Skip("set LISTODO_SCREENSHOT to save a rendered dashboard")
	}
	a := test.NewApp()
	defer a.Quit()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	fixtures := []model.Task{
		{Title: "Finish the authentication flow", Notes: "Handle token refresh and verify the sign-in journey.", Project: "Platform", DueDate: model.Day(now.AddDate(0, 0, -2)), Priority: 1, Status: "todo"},
		{Title: "Review the API changes", Notes: "Check the new pagination contract.", Project: "Platform", DueDate: model.Day(now), Priority: 2, Status: "working"},
		{Title: "Write the release notes", Project: "Listodo", DueDate: model.Day(now), Priority: 1, Status: "todo"},
		{Title: "Test SQLite persistence across restarts", Project: "Listodo", DueDate: model.Day(now), Priority: 1, Status: "done"},
	}
	for _, task := range fixtures {
		if _, err := s.SaveTask(task); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveSessions([]model.Session{{ID: "codex:sample", Harness: "codex", Title: "Review the API pagination changes", Project: "Platform", UpdatedAt: now, StartedAt: now, Status: "working"}, {ID: "claude:sample", Harness: "claude", Title: "Verify the task storage and completion history", Project: "Listodo", UpdatedAt: now, StartedAt: now, Status: "idle"}}); err != nil {
		t.Fatal(err)
	}
	u, err := New(a, s)
	if err != nil {
		t.Fatal(err)
	}
	defer u.cancel()
	u.Window.Show()
	u.Window.Resize(fyne.NewSize(1380, 900))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, u.Window.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}
