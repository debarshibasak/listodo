package store

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/adaptive-scale/listodo/internal/model"
)

func TestReorderPersistsAndPreservesHiddenTasksAndMetadata(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var tasks []model.Task
	for _, title := range []string{"A", "Hidden", "B", "C"} {
		task, err := s.SaveTask(model.Task{Title: title, Notes: "Keep notes", Status: "todo", Priority: 1})
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
	}
	if err := s.ReorderTasks([]string{tasks[3].ID, tasks[0].ID, tasks[2].ID}); err != nil {
		t.Fatal(err)
	}
	// A stale editor must not undo a saved reorder.
	tasks[0].Title = "Edited A"
	if _, err := s.SaveTask(tasks[0]); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stored, _ := s.Tasks()
	sort.Slice(stored, func(i, j int) bool { return stored[i].Order < stored[j].Order })
	for i, want := range []string{"C", "Hidden", "Edited A", "B"} {
		if stored[i].Title != want || stored[i].Notes != "Keep notes" {
			t.Fatal(stored)
		}
	}
	if !stored[2].CreatedAt.Equal(tasks[0].CreatedAt) || stored[2].StartDate != tasks[0].StartDate {
		t.Fatal("reordering changed dates")
	}
	if err := s.ReorderTasks([]string{tasks[0].ID, tasks[0].ID}); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
	if err := s.ReorderTasks([]string{tasks[0].ID, "missing"}); err == nil {
		t.Fatal("missing task accepted")
	}
}
