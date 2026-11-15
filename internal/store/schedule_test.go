package store

import (
	"reflect"
	"testing"

	"github.com/adaptive-scale/listodo/internal/model"
)

func TestEditTaskSchedule(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.SaveTask(model.Task{Title: "Plan the release", Notes: "Release notes", Project: "Platform", Directory: "/projects/platform", Priority: 2, Status: "working", SessionIDs: []string{"codex:linked"}})
	if err != nil {
		t.Fatal(err)
	}
	edit := TaskEdit{Title: task.Title, Notes: task.Notes, Priority: task.Priority}
	for _, date := range []string{"2026-12-08", "2027-01-04", ""} {
		edit.DueDate = &date
		saved, err := s.EditTask(task.ID, edit)
		if err != nil {
			t.Fatal(err)
		}
		want := task
		if !saved.CreatedAt.Equal(task.CreatedAt) {
			t.Fatal("scheduling changed the creation timestamp")
		}
		want.DueDate, want.CreatedAt, want.UpdatedAt = date, saved.CreatedAt, saved.UpdatedAt
		if !reflect.DeepEqual(saved, want) {
			t.Fatalf("scheduling changed other task fields: got %+v, want %+v", saved, want)
		}
		// Older clients omitting the date must leave the saved schedule intact.
		edit.DueDate = nil
		again, err := s.EditTask(task.ID, edit)
		if !again.UpdatedAt.Equal(saved.UpdatedAt) {
			t.Fatal("an unchanged schedule wrote a new update")
		}
		// JSON round trips strip time.Time's process-local monotonic clock.
		again.UpdatedAt = saved.UpdatedAt
		if err != nil || !reflect.DeepEqual(saved, again) {
			t.Fatalf("omitted date changed the task: %+v, %v", again, err)
		}
	}
	invalid := "2026-02-30"
	edit.DueDate = &invalid
	if _, err := s.EditTask(task.ID, edit); err == nil {
		t.Fatal("accepted an invalid calendar date")
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
	if err != nil || len(tasks) != 1 || tasks[0].DueDate != "" {
		t.Fatalf("schedule removal did not persist or failed update changed it: %+v, %v", tasks, err)
	}
}
