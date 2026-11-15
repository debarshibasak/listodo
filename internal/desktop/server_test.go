package desktop

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
	"github.com/adaptive-scale/listodo/internal/store"
)

func TestAPIWorkflowPersistenceAndAuthorization(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(db)
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	call := func(method, path string, body any, authenticated bool) *httptest.ResponseRecorder {
		var payload bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&payload).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, &payload)
		if authenticated {
			r.Header.Set("X-Listodo-Token", s.token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, route := range []struct{ method, path string }{{"GET", "/api/state"}, {"POST", "/api/tasks"}, {"DELETE", "/api/tasks/anything"}, {"GET", "/api/export"}, {"POST", "/api/organize"}} {
		if got := call(route.method, route.path, nil, false); got.Code != http.StatusUnauthorized {
			t.Fatalf("%s should reject unauthenticated requests: %d", route.path, got.Code)
		}
	}
	index := call("GET", "/", nil, false)
	if !strings.Contains(index.Body.String(), `name="listodo-token" content="`+s.token+`"`) {
		t.Fatal("frontend token is missing")
	}
	if !strings.Contains(index.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing content policy")
	}
	invalid := call("POST", "/api/tasks", model.Task{Title: "", Status: "todo", Priority: 1}, true)
	if invalid.Code != http.StatusBadRequest {
		t.Fatal("invalid task was accepted")
	}
	created := call("POST", "/api/tasks", map[string]any{"title": "Ship the new interface", "project": "Listodo", "directory": "/projects/listodo", "dueDate": model.Day(time.Now()), "status": "todo", "priority": 1, "createdAt": "", "updatedAt": ""}, true)
	if created.Code != http.StatusOK {
		t.Fatal(created.Body.String())
	}
	var task model.Task
	if err := json.Unmarshal(created.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	task.Status = "working"
	if r := call("POST", "/api/tasks", task, true); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	task.Status = "done"
	completed := call("POST", "/api/tasks", task, true)
	if err := json.Unmarshal(completed.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.CompletedAt == nil {
		t.Fatal("completion timestamp missing")
	}
	backup := call("GET", "/api/export", nil, true)
	var exported struct {
		Version int
		Tasks   []model.Task
		Events  []model.Event
	}
	if err := json.Unmarshal(backup.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Version != 1 || len(exported.Tasks) != 1 || len(exported.Events) != 3 {
		t.Fatal("export is incomplete")
	}
	if exported.Tasks[0].Directory != "/projects/listodo" {
		t.Fatal("task folder was lost during an update or export")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.store = db
	state := call("GET", "/api/state", nil, true)
	var snapshot snapshot
	if err := json.Unmarshal(state.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].Status != "done" {
		t.Fatal("completed task did not survive a restart")
	}
	if snapshot.Tasks[0].Directory != "/projects/listodo" {
		t.Fatal("task folder did not survive a restart")
	}
	if r := call("DELETE", "/api/tasks/"+task.ID, nil, true); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	state = call("GET", "/api/state", nil, true)
	if err := json.Unmarshal(state.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tasks) != 0 || len(snapshot.Events) != 4 {
		t.Fatal("deletion or retained history is incorrect")
	}
}

func TestEditorPatchPreservesConcurrentTaskMetadata(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server, err := NewServer(db)
	if err != nil {
		t.Fatal(err)
	}
	task, err := db.SaveTask(model.Task{Title: "Original title", Notes: "Original notes", Status: "todo", Priority: 1, Project: "Project", DueDate: "2099-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	// These changes happen after the editor took its initial task snapshot.
	task.Status, task.Directory, task.SessionIDs = "working", "/projects/associated", []string{"codex:linked"}
	task, err = db.SaveTask(task)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"title": "Edited title", "notes": "Edited notes", "priority": 3}
	path := "/api/tasks/" + task.ID
	response := operationRequest(server, "PATCH", path, payload)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var saved model.Task
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Title != "Edited title" || saved.Notes != "Edited notes" || saved.Priority != 3 {
		t.Fatal("editor changes were not saved")
	}
	if saved.Directory != task.Directory || saved.Status != "working" || len(saved.SessionIDs) != 1 || saved.SessionIDs[0] != "codex:linked" || saved.Project != task.Project || saved.DueDate != task.DueDate || saved.Order != task.Order || saved.StartDate != task.StartDate || !saved.CreatedAt.Equal(task.CreatedAt) {
		t.Fatalf("editor overwrote unrelated task metadata: %+v", saved)
	}
	events, _ := db.Events("")
	operationRequest(server, "PATCH", path, payload)
	after, _ := db.Events("")
	if len(events) != len(after) {
		t.Fatal("saving unchanged content must not duplicate history")
	}
	if invalid := operationRequest(server, "PATCH", path, map[string]any{"title": "", "priority": 1}); invalid.Code != http.StatusBadRequest {
		t.Fatal("invalid auto-save accepted")
	}
	stillSaved, _ := db.Task(task.ID)
	if stillSaved.Title != saved.Title || stillSaved.Notes != saved.Notes || !stillSaved.UpdatedAt.Equal(saved.UpdatedAt) {
		t.Fatal("invalid changes modified the saved task")
	}
	payload["status"] = "done"
	operationRequest(server, "PATCH", path, payload)
	completed, _ := db.Task(task.ID)
	delete(payload, "status")
	payload["notes"] = "Further notes"
	operationRequest(server, "PATCH", path, payload)
	edited, _ := db.Task(task.ID)
	if completed.CompletedAt == nil || edited.CompletedAt == nil || !edited.CompletedAt.Equal(*completed.CompletedAt) {
		t.Fatal("editing a completed task changed its completion time")
	}
	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest("PATCH", path, strings.NewReader(`{"title":"Unauthorized"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatal("unauthenticated edit accepted")
	}
}
