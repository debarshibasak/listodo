package store

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
)

func TestPersistenceAndCompletionHistory(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.SaveTask(model.Task{Title: "Ship the desktop app", Project: "Listodo", Directory: " /projects/listodo ", Priority: 1, Status: "todo", DueDate: model.Day(time.Now()), SessionIDs: []string{"codex:abc"}})
	if err != nil {
		t.Fatal(err)
	}
	created := task.CreatedAt
	task.Status = "working"
	task, err = s.SaveTask(task)
	if err != nil {
		t.Fatal(err)
	}
	task.Status = "done"
	task, err = s.SaveTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if task.CompletedAt == nil {
		t.Fatal("completion time missing")
	}
	completed := *task.CompletedAt
	task.Notes = "Verified"
	task, err = s.SaveTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if !task.CompletedAt.Equal(completed) {
		t.Fatal("editing changed the completion date")
	}
	settings := model.Settings{Harness: "claude", CodexRoot: "/custom/codex", ClaudeRoot: "/custom/claude", AutoSync: true, ClaudeEnabled: true}
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	session := model.Session{ID: "codex:abc", Harness: "codex", Title: "Ship Listodo", Project: "Listodo", UpdatedAt: time.Now()}
	if err := s.SaveSessions([]model.Session{session}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tasks, err := s.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Title != task.Title || !tasks[0].CreatedAt.Equal(created) || !tasks[0].CompletedAt.Equal(completed) || len(tasks[0].SessionIDs) != 1 {
		t.Fatalf("task did not survive restart: %+v", tasks)
	}
	if tasks[0].Directory != "/projects/listodo" {
		t.Fatal("task folder did not survive edits and restart")
	}
	gotSettings, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(settings, gotSettings) {
		t.Fatalf("settings not persisted: %+v", gotSettings)
	}
	ss, err := s.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 1 || ss[0].ID != session.ID {
		t.Fatal("session not persisted")
	}
	events, err := s.Events(model.Day(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	done := 0
	for _, e := range events {
		if e.Action == "done" {
			done++
		}
	}
	if done != 1 {
		t.Fatalf("got %d completion events", done)
	}
	task.Status = "todo"
	task, err = s.SaveTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if task.CompletedAt != nil {
		t.Fatal("reopened task retains completion")
	}
	if err := s.DeleteTask(task.ID); err != nil {
		t.Fatal(err)
	}
	tasks, err = s.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatal("deleted task remains")
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("database permissions are too broad")
	}
}

func TestTaskDirectoriesRestoreFromHistoryAndSessions(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	session := model.Session{ID: "codex:linked", Directory: "/projects/linked", UpdatedAt: time.Now()}
	if err := s.SaveSessions([]model.Session{session}); err != nil {
		t.Fatal(err)
	}
	tasks := make([]model.Task, 3)
	for i, directory := range []string{"", "", "/projects/chosen"} {
		tasks[i], err = s.SaveTask(model.Task{Title: "Task with a folder", Status: "todo", Directory: directory, SessionIDs: []string{session.ID}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if tasks[1].Directory != session.Directory || tasks[2].Directory != "/projects/chosen" {
		t.Fatal("session links must supply a folder without replacing a chosen folder")
	}
	for i, directory := range []string{"/projects/older", "/projects/latest"} {
		op := model.Operation{ID: model.ID(), TaskID: tasks[0].ID, Status: "running", Directory: directory, StartedAt: time.Now().Add(time.Duration(i-1) * time.Hour)}
		if err := s.BeginOperation(op); err != nil {
			t.Fatal(err)
		}
		op.Status = "succeeded"
		if err := s.SaveOperation(op); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate the task records written by versions without a directory field.
	if _, err := s.db.Exec("UPDATE tasks SET data = json_remove(data, '$.directory') WHERE id IN (?, ?)", tasks[0].ID, tasks[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i, want := range []string{"/projects/latest", session.Directory, "/projects/chosen"} {
		task, err := s.Task(tasks[i].ID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Directory != want {
			t.Fatalf("restored folder = %q; want %q", task.Directory, want)
		}
	}
}

func TestValidationDoesNotWrite(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, task := range []model.Task{{Title: "", Status: "todo"}, {Title: "bad date", Status: "todo", DueDate: "tomorrow"}, {Title: "bad status", Status: "maybe"}} {
		if _, err := s.SaveTask(task); err == nil {
			t.Fatal("invalid task accepted")
		}
	}
	tasks, err := s.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatal("invalid write persisted")
	}
}

func TestStartDateIsAssignedAndPreserved(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task, err := s.SaveTask(model.Task{Title: "Write release notes", Status: "todo", Priority: 1, StartDate: "2000-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	start := task.StartDate
	if start != task.CreatedAt.Format("2006-01-02") || task.DueDate != "" {
		t.Fatal("start must be the creation day without an implicit deadline")
	}
	task.StartDate = "2001-01-01"
	task.Status = "working"
	task, err = s.SaveTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if task.StartDate != start {
		t.Fatal("editing changed the start date")
	}
	tasks, err := s.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].StartDate != start {
		t.Fatal("start date was not persisted")
	}
}
