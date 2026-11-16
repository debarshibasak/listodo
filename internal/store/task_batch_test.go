package store

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
)

func batchStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func batchTask(t *testing.T, s *Store, task model.Task) model.Task {
	t.Helper()
	if task.Status == "" {
		task.Status = "todo"
	}
	saved, err := s.SaveTask(task)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func assertStoredTask(t *testing.T, s *Store, want model.Task) {
	t.Helper()
	got, err := s.Task(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	// JSON comparison ignores time.Time's in-memory location/monotonic state.
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Fatalf("task changed:\n got %s\nwant %s", a, b)
	}
}

func TestCloseSelectionAndUndoPreserveEveryTask(t *testing.T) {
	s := batchStore(t)
	a := batchTask(t, s, model.Task{Title: "A", Notes: "Notes", Priority: 2, DueDate: "2030-01-04", SessionIDs: []string{"session"}})
	b := batchTask(t, s, model.Task{Title: "B", Status: "working", Directory: "/work"})
	c := batchTask(t, s, model.Task{Title: "Already done", Status: "done"})
	hidden := batchTask(t, s, model.Task{Title: "Hidden"})
	if err := s.CloseTasks([]string{a.ID, b.ID, c.ID}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a.ID, b.ID} {
		task, _ := s.Task(id)
		if task.Status != "done" || task.CompletedAt == nil {
			t.Fatal("selected task was not completed")
		}
	}
	assertStoredTask(t, s, c)
	assertStoredTask(t, s, hidden)
	if label, err := s.UndoTasks(); err != nil || label != "close tasks" {
		t.Fatalf("undo: %s %v", label, err)
	}
	for _, task := range []model.Task{a, b, c, hidden} {
		assertStoredTask(t, s, task)
	}
}

func TestMergeAndUndoRestoreNotesMetadataAndRunHistory(t *testing.T) {
	s := batchStore(t)
	a := batchTask(t, s, model.Task{Title: "Keep title", Notes: "First notes", Priority: 1, Project: "Primary", Directory: "/primary", DueDate: "2030-02-02", SessionIDs: []string{"common", "first"}})
	b := batchTask(t, s, model.Task{Title: "Second title", Notes: "- [ ] Second notes", Status: "working", Priority: 3, Project: "Other", Directory: "/other", DueDate: "2030-01-01", SessionIDs: []string{"common", "second"}})
	op := model.Operation{ID: "run", TaskID: b.ID, Title: b.Title, Directory: b.Directory, Status: "succeeded", Output: "Preserve output", SessionID: "second", StartedAt: time.Now()}
	raw, _ := json.Marshal(op)
	if _, err := s.db.Exec("INSERT INTO operations(id,data) VALUES(?,?)", op.ID, string(raw)); err != nil {
		t.Fatal(err)
	}
	merged, err := s.MergeTasks([]string{a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if merged.ID != a.ID || merged.Title != a.Title || merged.Project != a.Project || merged.Directory != a.Directory || merged.Order != a.Order || merged.StartDate != a.StartDate || !merged.CreatedAt.Equal(a.CreatedAt) {
		t.Fatal("merge did not retain the first task's identity and metadata")
	}
	if merged.Status != "working" || merged.CompletedAt != nil || merged.Priority != 3 || merged.DueDate != b.DueDate || !reflect.DeepEqual(merged.SessionIDs, []string{"common", "first", "second"}) {
		t.Fatalf("bad merged metadata: %+v", merged)
	}
	for _, text := range []string{a.Notes, b.Title, b.Notes, b.Project, b.Directory} {
		if !strings.Contains(merged.Notes, text) {
			t.Fatalf("lost merged content: %s", text)
		}
	}
	if tasks, _ := s.Tasks(); len(tasks) != 1 {
		t.Fatal("source task was not removed")
	}
	moved, _ := s.Operation(op.ID)
	if moved.TaskID != a.ID || moved.Output != op.Output || moved.Title != op.Title {
		t.Fatal("lost or detached run history")
	}
	// Changes to run output are independent of the merge and must survive undo.
	moved.Output += "\nMore output"
	if err := s.SaveOperation(moved); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoTasks(); err != nil {
		t.Fatal(err)
	}
	assertStoredTask(t, s, a)
	assertStoredTask(t, s, b)
	restored, _ := s.Operation(op.ID)
	if restored.TaskID != b.ID || restored.Output != moved.Output {
		t.Fatal("undo did not restore run ownership while preserving output")
	}
	// Undoing the preceding create still works after a merge has been undone.
	if _, err := s.UndoTasks(); err != nil {
		t.Fatal(err)
	}
	if tasks, _ := s.Tasks(); len(tasks) != 1 || tasks[0].ID != a.ID {
		t.Fatal("undo stack lost its order")
	}
}

func TestBatchValidationIsAtomicAndDoesNotConsumeUndo(t *testing.T) {
	for _, action := range []string{"close", "merge"} {
		t.Run(action, func(t *testing.T) {
			s := batchStore(t)
			a := batchTask(t, s, model.Task{Title: "A"})
			b := batchTask(t, s, model.Task{Title: "B"})
			for _, ids := range [][]string{nil, {a.ID, a.ID}, {a.ID, "missing"}} {
				var err error
				if action == "close" {
					err = s.CloseTasks(ids)
				} else {
					_, err = s.MergeTasks(ids)
				}
				if err == nil {
					t.Fatal("invalid selection accepted")
				}
				assertStoredTask(t, s, a)
				assertStoredTask(t, s, b)
			}
			if _, err := s.UndoTasks(); err != nil {
				t.Fatal(err)
			}
			if tasks, _ := s.Tasks(); len(tasks) != 1 || tasks[0].ID != a.ID {
				t.Fatal("failed actions changed undo history")
			}
		})
	}
}

func TestMergeRejectsOversizedNotesAndRunningTasks(t *testing.T) {
	s := batchStore(t)
	a := batchTask(t, s, model.Task{Title: "A", Notes: strings.Repeat("a", 20000)})
	b := batchTask(t, s, model.Task{Title: "B", Notes: strings.Repeat("b", 20000)})
	if _, err := s.MergeTasks([]string{a.ID, b.ID}); err == nil {
		t.Fatal("oversized merge accepted")
	}
	assertStoredTask(t, s, a)
	assertStoredTask(t, s, b)
	op := model.Operation{ID: "running", TaskID: b.ID, Status: "running", Directory: "/work"}
	if err := s.BeginOperation(op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MergeTasks([]string{a.ID, b.ID}); err == nil || !strings.Contains(err.Error(), "running agent") {
		t.Fatalf("running task merge: %v", err)
	}
	if _, err := s.UndoTasks(); err == nil {
		t.Fatal("undo removed a running task")
	}
	if tasks, _ := s.Tasks(); len(tasks) != 2 {
		t.Fatal("failed merge removed tasks")
	}
}

func TestUndoPersistsAndRestoresCreateEditReorderDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := batchTask(t, s, model.Task{Title: "A", Status: "done", Notes: "Original"})
	b := batchTask(t, s, model.Task{Title: "B"})
	if err := s.ReorderTasks([]string{b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EditTask(a.ID, TaskEdit{Title: "Edited", Notes: "New", Priority: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTask(a.ID); err != nil {
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
	for i := 0; i < 3; i++ {
		if _, err := s.UndoTasks(); err != nil {
			t.Fatal(err)
		}
	}
	assertStoredTask(t, s, a)
	assertStoredTask(t, s, b)
	for i := 0; i < 2; i++ {
		if _, err := s.UndoTasks(); err != nil {
			t.Fatal(err)
		}
	}
	if tasks, _ := s.Tasks(); len(tasks) != 0 {
		t.Fatal("create undo did not remove tasks")
	}
	if _, err := s.UndoTasks(); err == nil {
		t.Fatal("empty history did not report an error")
	}
}

func TestUndoPreservesConcurrentSessionLinksAndRejectsConflictingChanges(t *testing.T) {
	s := batchStore(t)
	a := batchTask(t, s, model.Task{Title: "A"})
	if _, err := s.EditTask(a.ID, TaskEdit{Title: "Edited"}); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkTaskSession(a.ID, "new-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoTasks(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Task(a.ID)
	if got.Title != a.Title || !reflect.DeepEqual(got.SessionIDs, []string{"new-session"}) {
		t.Fatal("undo clobbered an unrelated session update")
	}
	if _, err := s.UndoTasks(); err == nil {
		t.Fatal("undo deleted a task with newer session data")
	}
	if _, err := s.EditTask(a.ID, TaskEdit{Title: "Next edit"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE tasks SET data=json_set(data, '$.title', 'External edit') WHERE id=?", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoTasks(); err == nil {
		t.Fatal("conflicting external edit was overwritten")
	}
	got, _ = s.Task(a.ID)
	if got.Title != "External edit" {
		t.Fatal("failed undo changed the task")
	}
}
