package desktop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/adaptive-scale/listodo/internal/harness"
	"github.com/adaptive-scale/listodo/internal/model"
)

const operationOutputLimit = 128 << 10

type liveOperation struct {
	mu        sync.Mutex
	operation model.Operation
	cancel    context.CancelFunc
	lastSave  time.Time
}

func (run *liveOperation) snapshot() model.Operation {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.operation
}

func (s *Server) recoverOperations() error {
	operations, err := s.store.Operations()
	if err != nil {
		return err
	}
	for _, op := range operations {
		if op.Status == "running" && !harness.ProcessAlive(op.OwnerPID) {
			now := time.Now()
			op.Status, op.Error = "interrupted", "Listodo closed before this run finished."
			op.FinishedAt, op.UpdatedAt = &now, now
			if err := s.store.SaveOperation(op); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) operationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/operations", func(w http.ResponseWriter, r *http.Request) {
		operations, err := s.store.Operations()
		if err != nil {
			s.respond(w, nil, err)
			return
		}
		for i, op := range operations {
			s.mu.Lock()
			run := s.runs[op.ID]
			s.mu.Unlock()
			if run != nil {
				operations[i] = run.snapshot()
			}
			operations[i].Output, operations[i].Prompt, operations[i].OwnerPID = "", "", 0
		}
		s.respond(w, operations, nil)
	})
	mux.HandleFunc("GET /api/operations/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		run := s.runs[r.PathValue("id")]
		s.mu.Unlock()
		if run != nil {
			s.respond(w, run.snapshot(), nil)
			return
		}
		op, err := s.store.Operation(r.PathValue("id"))
		s.respond(w, op, err)
	})
	mux.HandleFunc("POST /api/operations", s.startOperation)
	mux.HandleFunc("POST /api/operations/{id}/continue", s.continueOperation)
	mux.HandleFunc("POST /api/operations/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		run := s.runs[r.PathValue("id")]
		s.mu.Unlock()
		if run == nil {
			op, err := s.store.Operation(r.PathValue("id"))
			if err == nil && op.Status == "running" {
				err = errors.New("this run belongs to another Listodo window; stop it there")
			}
			s.respond(w, op, err)
			return
		}
		run.cancel()
		s.respond(w, map[string]bool{"ok": true}, nil)
	})
	mux.HandleFunc("POST /api/folders/choose", func(w http.ResponseWriter, r *http.Request) {
		if !s.Native {
			s.respond(w, nil, errors.New("enter a project folder path in the browser"))
			return
		}
		path, err := chooseFolder()
		s.respond(w, map[string]string{"path": path}, err)
	})
}

func (s *Server) startOperation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TaskID    string `json:"taskId"`
		Provider  string `json:"provider"`
		Directory string `json:"directory"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	task, err := s.store.Task(input.TaskID)
	if err != nil {
		s.respond(w, nil, errors.New("task no longer exists"))
		return
	}
	if task.Status == "done" {
		s.respond(w, nil, errors.New("reopen this task before running an agent"))
		return
	}
	if strings.TrimSpace(input.Directory) == "" {
		input.Directory = task.Directory
	}
	prompt := task.Title
	if task.Notes != "" {
		prompt += "\n\n" + task.Notes
	}
	s.launchOperation(w, model.Operation{ID: model.ID(), Turn: 1, TaskID: task.ID, Title: task.Title, Prompt: prompt, Provider: input.Provider, Directory: input.Directory}, prompt, false)
}

func (s *Server) continueOperation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Prompt string `json:"prompt"`
		Turn   int    `json:"turn"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" || len(prompt) > 30000 {
		s.respond(w, nil, errors.New("write a prompt of 1–30,000 characters"))
		return
	}
	op, err := s.store.Operation(r.PathValue("id"))
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	if op.Status == "running" {
		s.respond(w, nil, errors.New("wait for this run to finish or stop it before sending another prompt"))
		return
	}
	if op.Turn == 0 {
		op.Turn = 1
	}
	if input.Turn != op.Turn {
		s.respond(w, nil, errors.New("this session has changed; refresh before sending another prompt"))
		return
	}
	op.Turn++
	op.Output = outputTail(op.Output + "\n> " + prompt + "\n\n")
	s.launchOperation(w, op, prompt, true)
}

func (s *Server) launchOperation(w http.ResponseWriter, operation model.Operation, prompt string, continuing bool) {
	// A background context lets runs continue when the user leaves the page.
	ctx, cancel := context.WithCancel(context.Background())
	run := &liveOperation{cancel: cancel}
	emit := func(event harness.RunEvent) {
		run.mu.Lock()
		defer run.mu.Unlock()
		if event.SessionID != "" {
			run.operation.SessionID = event.SessionID
		}
		if text := strings.TrimSpace(event.Text); text != "" {
			run.operation.Output = outputTail(run.operation.Output + text + "\n")
		}
		run.operation.UpdatedAt = time.Now()
		if time.Since(run.lastSave) >= 500*time.Millisecond {
			if err := s.store.SaveOperation(run.operation); err != nil {
				log.Printf("save operation progress: %v", err)
			}
			run.lastSave = time.Now()
		}
	}
	var execution *harness.Execution
	var directory string
	var err error
	if continuing {
		execution, directory, err = harness.PrepareContinuation(ctx, operation.Provider, operation.Directory, operation.SessionID, prompt, emit)
	} else {
		execution, directory, err = harness.PrepareRun(ctx, operation.Provider, operation.Directory, prompt, emit)
	}
	if err != nil {
		cancel()
		s.respond(w, nil, err)
		return
	}
	now := time.Now()
	operation.Directory, operation.Status, operation.Error = directory, "running", ""
	operation.UpdatedAt, operation.FinishedAt, operation.OwnerPID = now, nil, os.Getpid()
	if operation.StartedAt.IsZero() {
		operation.StartedAt = now
	}
	run.operation = operation
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		cancel()
		s.respond(w, nil, errors.New("Listodo is closing"))
		return
	}
	begin := s.store.BeginOperation
	if continuing {
		begin = s.store.ContinueOperation
	}
	if err := begin(run.operation); err != nil {
		s.mu.Unlock()
		cancel()
		s.respond(w, nil, err)
		return
	}
	if s.Native {
		prepareRunNotifications()
	}
	// Start while holding the server lock so shutdown cannot miss a launched process.
	if err := execution.Start(); err != nil {
		now := time.Now()
		run.operation.Status, run.operation.Error = "failed", fmt.Sprintf("could not start agent: %v", err)
		run.operation.FinishedAt, run.operation.UpdatedAt = &now, now
		if saveErr := s.store.SaveOperation(run.operation); saveErr != nil {
			log.Printf("save failed operation: %v", saveErr)
		}
		s.mu.Unlock()
		cancel()
		s.operationFinished(run.operation)
		s.respond(w, run.operation, nil)
		return
	}
	s.runs[run.operation.ID] = run
	s.runWG.Add(1)
	s.mu.Unlock()
	go s.waitOperation(ctx, run, execution)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	s.respond(w, run.snapshot(), nil)
}

func outputTail(output string) string {
	if len(output) <= operationOutputLimit {
		return output
	}
	output = output[len(output)-operationOutputLimit:]
	for len(output) > 0 && !utf8.RuneStart(output[0]) {
		output = output[1:]
	}
	return "[Earlier output omitted]\n" + output
}

func (s *Server) waitOperation(ctx context.Context, run *liveOperation, execution *harness.Execution) {
	defer s.runWG.Done()
	err := execution.Wait()
	run.mu.Lock()
	now := time.Now()
	op := &run.operation
	op.FinishedAt, op.UpdatedAt = &now, now
	switch {
	case ctx.Err() != nil:
		op.Status = "cancelled"
	case err != nil:
		op.Status, op.Error = "failed", err.Error()
	default:
		op.Status = "succeeded"
	}
	if err := s.store.SaveOperation(*op); err != nil {
		log.Printf("save finished operation: %v", err)
	}
	if op.SessionID != "" {
		if err := s.store.LinkTaskSession(op.TaskID, op.Provider+":"+op.SessionID); err != nil {
			log.Printf("link operation session: %v", err)
		}
	}
	finished := *op
	run.mu.Unlock()
	run.cancel()
	s.mu.Lock()
	if s.runs[run.operation.ID] == run {
		delete(s.runs, run.operation.ID)
	}
	s.mu.Unlock()
	s.operationFinished(finished)
}

func (s *Server) operationFinished(operation model.Operation) {
	if operation.Status != "succeeded" && operation.Status != "failed" {
		return
	}
	s.mu.Lock()
	closing, notify := s.closing, s.notifyFinished
	s.mu.Unlock()
	if !closing && notify != nil {
		notify(operation)
	}
}

func (s *Server) stopOperations() {
	s.mu.Lock()
	s.closing = true
	for _, run := range s.runs {
		run.cancel()
	}
	s.mu.Unlock()
	s.runWG.Wait()
}
