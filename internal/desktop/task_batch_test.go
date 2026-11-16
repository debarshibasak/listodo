package desktop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adaptive-scale/listodo/internal/model"
	"github.com/adaptive-scale/listodo/internal/store"
)

func TestTaskBatchAPIAndUndo(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := NewServer(db)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, title := range []string{"First task", "Second task"} {
		response := operationRequest(s, "POST", "/api/tasks", model.Task{Title: title, Status: "todo"})
		if response.Code != http.StatusOK {
			t.Fatal(response.Body.String())
		}
		var task model.Task
		if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, task.ID)
	}
	for _, path := range []string{"close", "merge", "undo"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/api/tasks/"+path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s accepted", path)
		}
	}
	for _, path := range []string{"close", "merge"} {
		if result := operationRequest(s, "POST", "/api/tasks/"+path, map[string]any{"taskIds": []string{ids[0], "missing"}}); result.Code != http.StatusBadRequest {
			t.Fatal(result.Body.String())
		}
		result := operationRequest(s, "POST", "/api/tasks/"+path, map[string]any{"taskIds": ids})
		if result.Code != http.StatusOK {
			t.Fatal(result.Body.String())
		}
		tasks, _ := db.Tasks()
		if path == "close" && (len(tasks) != 2 || tasks[0].Status != "done" || tasks[1].Status != "done") {
			t.Fatal("close did not persist")
		}
		if path == "merge" && (len(tasks) != 1 || tasks[0].ID != ids[0]) {
			t.Fatal("merge did not persist")
		}
		if result := operationRequest(s, "POST", "/api/tasks/undo", nil); result.Code != http.StatusOK {
			t.Fatal(result.Body.String())
		}
		tasks, _ = db.Tasks()
		if len(tasks) != 2 || tasks[0].Status != "todo" || tasks[1].Status != "todo" {
			t.Fatal("undo did not restore tasks")
		}
	}
}
