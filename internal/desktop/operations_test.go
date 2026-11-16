package desktop

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
	"github.com/adaptive-scale/listodo/internal/store"
)

func operationServer(t *testing.T, script string) (*Server, string) {
	t.Helper()
	binDir, workspace := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.stopOperations(); db.Close() })
	return server, workspace
}
func operationRequest(server *Server, method, path string, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("X-Listodo-Token", server.token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	return w
}
func awaitOperation(t *testing.T, server *Server, id string) model.Operation {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		op, err := server.store.Operation(id)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status != "running" {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("operation did not finish")
	return model.Operation{}
}
func TestTaskOperationPersistsOutputAndLinksTheSession(t *testing.T) {
	server, workspace := operationServer(t, `/bin/cat > prompt
printf '%s\n' '{"type":"thread.started","thread_id":"session-123"}' '{"type":"item.completed","item":{"type":"agent_message","text":"Implemented and tested."}}' '{"type":"turn.completed"}'
`)
	task, err := server.store.SaveTask(model.Task{Title: "Implement the task", Notes: "Keep the tests passing", Status: "todo", Priority: 1})
	if err != nil {
		t.Fatal(err)
	}
	response := operationRequest(server, "POST", "/api/operations", map[string]string{"taskId": task.ID, "provider": "codex", "directory": workspace})
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	var op model.Operation
	if err := json.Unmarshal(response.Body.Bytes(), &op); err != nil {
		t.Fatal(err)
	}
	final := awaitOperation(t, server, op.ID)
	if final.Status != "succeeded" || final.SessionID != "session-123" || !strings.Contains(final.Output, "Implemented and tested.") {
		t.Fatalf("invalid result: %+v", final)
	}
	server.runWG.Wait()
	updated, _ := server.store.Task(task.ID)
	if updated.Directory != op.Directory {
		t.Fatal("run folder was not saved on the task")
	}
	if updated.Status != "working" || len(updated.SessionIDs) != 1 || updated.SessionIDs[0] != "codex:session-123" {
		t.Fatalf("task lifecycle or session link incorrect: %+v", updated)
	}
	if !updated.CreatedAt.Equal(task.CreatedAt) {
		t.Fatal("creation date changed")
	}
	list := operationRequest(server, "GET", "/api/operations", nil)
	if strings.Contains(list.Body.String(), "Implemented and tested.") {
		t.Fatal("list must not include full logs")
	}
	detail := operationRequest(server, "GET", "/api/operations/"+op.ID, nil)
	if !strings.Contains(detail.Body.String(), "Implemented and tested.") {
		t.Fatal("detail omitted output")
	}
	prompt, _ := os.ReadFile(filepath.Join(workspace, "prompt"))
	if string(prompt) != "Implement the task\n\nKeep the tests passing" {
		t.Fatal("wrong prompt")
	}
	// A later run can omit the folder and reuse the task's saved association.
	response = operationRequest(server, "POST", "/api/operations", map[string]string{"taskId": task.ID, "provider": "codex"})
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	var next model.Operation
	if err := json.Unmarshal(response.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if next.Directory != op.Directory {
		t.Fatal("later run did not reuse the task folder")
	}
	if final := awaitOperation(t, server, next.ID); final.Status != "succeeded" {
		t.Fatal("later run failed")
	}
}
func TestOperationValidationDuplicatePreventionAndStop(t *testing.T) {
	server, workspace := operationServer(t, "/bin/sleep 30\n")
	task, _ := server.store.SaveTask(model.Task{Title: "Long task", Status: "todo", Priority: 1})
	for _, body := range []map[string]string{
		{"taskId": task.ID, "provider": "unknown", "directory": workspace},
		{"taskId": task.ID, "provider": "codex", "directory": "relative/path"},
		{"taskId": "missing", "provider": "codex", "directory": workspace},
	} {
		if r := operationRequest(server, "POST", "/api/operations", body); r.Code != http.StatusBadRequest {
			t.Fatal("invalid launch accepted")
		}
	}
	payload := map[string]string{"taskId": task.ID, "provider": "codex", "directory": workspace}
	r := operationRequest(server, "POST", "/api/operations", payload)
	var op model.Operation
	json.Unmarshal(r.Body.Bytes(), &op)
	if op.ID == "" {
		t.Fatal(r.Body.String())
	}
	if r := operationRequest(server, "POST", "/api/operations", payload); r.Code != http.StatusBadRequest {
		t.Fatal("duplicate run accepted")
	}
	if r := operationRequest(server, "DELETE", "/api/tasks/"+task.ID, nil); r.Code != http.StatusBadRequest {
		t.Fatal("running task deleted")
	}
	operationRequest(server, "POST", "/api/operations/"+op.ID+"/cancel", map[string]string{})
	if final := awaitOperation(t, server, op.ID); final.Status != "cancelled" {
		t.Fatal(final.Status)
	}
}
func TestOperationRecoveryAndAuthorization(t *testing.T) {
	server, workspace := operationServer(t, "exit 1\n")
	task, _ := server.store.SaveTask(model.Task{Title: "Interrupted task", Status: "todo", Priority: 1})
	op := model.Operation{ID: model.ID(), TaskID: task.ID, Title: task.Title, Directory: workspace, Provider: "codex", Status: "running", OwnerPID: -1, StartedAt: time.Now()}
	op.Output, op.Prompt = "Output before interruption", "Original instructions"
	if err := server.store.BeginOperation(op); err != nil {
		t.Fatal(err)
	}
	if err := server.recoverOperations(); err != nil {
		t.Fatal(err)
	}
	stored, _ := server.store.Operation(op.ID)
	if stored.Status != "interrupted" || stored.FinishedAt == nil {
		t.Fatal("stale run not recovered")
	}
	if stored.Output != op.Output || stored.Prompt != op.Prompt {
		t.Fatal("recovery lost the full run history")
	}
	// Running operations owned by another live window must keep their status.
	op.ID, op.OwnerPID = model.ID(), os.Getpid()
	if err := server.store.BeginOperation(op); err != nil {
		t.Fatal(err)
	}
	if err := server.recoverOperations(); err != nil {
		t.Fatal(err)
	}
	if active, err := server.store.Operation(op.ID); err != nil || active.Status != "running" {
		t.Fatal("recovery interrupted an operation owned by a live process")
	}
	for _, path := range []string{"/api/operations", "/api/operations/" + op.ID} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatal("unauthenticated operation data exposed")
		}
	}
}

func TestContinueOperationKeepsSessionAndOutput(t *testing.T) {
	for _, status := range []string{"succeeded", "failed", "cancelled", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			server, workspace := operationServer(t, `/bin/cat > prompt
printf '%s\n' '{"type":"thread.started","thread_id":"session-context"}' '{"type":"item.completed","item":{"type":"agent_message","text":"Follow-up complete."}}' '{"type":"turn.completed"}'
`)
			task, _ := server.store.SaveTask(model.Task{Title: "Continue this task", Status: "todo"})
			op := model.Operation{ID: model.ID(), TaskID: task.ID, Turn: 1, Title: task.Title, Prompt: "Original task", Directory: workspace,
				Provider: "codex", Status: "running", SessionID: "session-context", Output: "Earlier output\n", StartedAt: time.Now().Add(-time.Hour)}
			if status == "interrupted" {
				op.Turn = 0 // Older run records have no turn field.
			}
			if err := server.store.BeginOperation(op); err != nil {
				t.Fatal(err)
			}
			op.Status = status
			if err := server.store.SaveOperation(op); err != nil {
				t.Fatal(err)
			}
			notifications := make(chan model.Operation, 3)
			server.notifyFinished = func(finished model.Operation) { notifications <- finished }
			for turn := 1; turn <= 2; turn++ {
				payload := map[string]any{"prompt": "Please test these changes.", "turn": turn}
				response := operationRequest(server, "POST", "/api/operations/"+op.ID+"/continue", payload)
				if response.Code != http.StatusAccepted {
					t.Fatal(response.Body.String())
				}
				final := awaitOperation(t, server, op.ID)
				server.runWG.Wait()
				if final.Status != "succeeded" || final.Turn != turn+1 || final.SessionID != op.SessionID || final.Prompt != op.Prompt || !final.StartedAt.Equal(op.StartedAt) {
					t.Fatalf("session context was not preserved: %+v", final)
				}
				if !strings.Contains(final.Output, "Earlier output") || !strings.Contains(final.Output, "> Please test these changes.") || !strings.Contains(final.Output, "Follow-up complete.") {
					t.Fatal("continuation did not retain output and prompt history")
				}
				if stale := operationRequest(server, "POST", "/api/operations/"+op.ID+"/continue", payload); stale.Code != http.StatusBadRequest {
					t.Fatal("a repeated prompt from a stale turn was accepted")
				}
			}
			if len(notifications) != 2 {
				t.Fatal("each completed turn must notify exactly once")
			}
			prompt, _ := os.ReadFile(filepath.Join(workspace, "prompt"))
			if string(prompt) != "Please test these changes." {
				t.Fatal("the agent did not receive the follow-up prompt")
			}
		})
	}
}

func TestOperationCompletionNotifications(t *testing.T) {
	for _, kind := range []string{"success", "failure", "start-failure", "stop", "shutdown"} {
		t.Run(kind, func(t *testing.T) {
			script := "printf '%s\\n' '{\"type\":\"turn.completed\"}'\n"
			if kind == "failure" {
				script = "exit 1\n"
			} else if kind == "stop" || kind == "shutdown" {
				script = "/bin/sleep 30\n"
			}
			server, workspace := operationServer(t, script)
			if kind == "start-failure" {
				if err := os.WriteFile(filepath.Join(os.Getenv("PATH"), "codex"), []byte("#!/missing-listodo-test-interpreter\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			notifications := make(chan model.Operation, 2)
			server.notifyFinished = func(operation model.Operation) { notifications <- operation }
			task, _ := server.store.SaveTask(model.Task{Title: "Notify about this task", Status: "todo"})
			response := operationRequest(server, "POST", "/api/operations", map[string]string{"taskId": task.ID, "provider": "codex", "directory": workspace})
			var op model.Operation
			if err := json.Unmarshal(response.Body.Bytes(), &op); err != nil || op.ID == "" {
				t.Fatal(response.Body.String())
			}
			if kind == "stop" {
				operationRequest(server, "POST", "/api/operations/"+op.ID+"/cancel", nil)
			} else if kind == "shutdown" {
				server.stopOperations()
			}
			final := awaitOperation(t, server, op.ID)
			server.runWG.Wait()
			if kind == "stop" || kind == "shutdown" {
				if len(notifications) != 0 {
					t.Fatal("stopping or quitting must not produce completion alerts")
				}
			} else {
				if len(notifications) != 1 {
					t.Fatal("a completed or failed run must notify once")
				}
				notification := <-notifications
				if notification.ID != final.ID || notification.Status != final.Status || notification.Title != task.Title {
					t.Fatal("notification must identify the finished run and its outcome")
				}
			}
		})
	}
}
