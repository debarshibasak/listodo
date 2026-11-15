// Package desktop serves the shadcn interface and the existing local Go services.
package desktop

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/adaptive-scale/listodo/internal/harness"
	"github.com/adaptive-scale/listodo/internal/model"
	"github.com/adaptive-scale/listodo/internal/sessions"
	"github.com/adaptive-scale/listodo/internal/store"
)

//go:embed all:dist
var assets embed.FS

type Server struct {
	Native         bool
	store          *store.Store
	token          string
	mu             sync.Mutex
	organizeCancel context.CancelFunc
	runs           map[string]*liveOperation
	runWG          sync.WaitGroup
	closing        bool
	notifyFinished func(model.Operation)
}

type snapshot struct {
	Tasks    []model.Task    `json:"tasks"`
	Sessions []model.Session `json:"sessions"`
	Settings model.Settings  `json:"settings"`
	Events   []model.Event   `json:"events"`
	Database string          `json:"database"`
}

func NewServer(s *store.Store) (*Server, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, err
	}
	server := &Server{store: s, token: hex.EncodeToString(key[:]), runs: map[string]*liveOperation{}}
	server.notifyFinished = func(operation model.Operation) {
		if server.Native {
			notifyRunFinished(operation)
		}
	}
	if err := server.recoverOperations(); err != nil {
		return nil, err
	}
	return server, nil
}

// Listen binds only to loopback. A new random token protects every API request.
func (s *Server) Listen(address string) (string, func(), error) {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return "", nil, err
	}
	if address, ok := l.Addr().(*net.TCPAddr); !ok || !address.IP.IsLoopback() {
		_ = l.Close()
		return "", nil, errors.New("Listodo must listen on a loopback address")
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(l) }()
	return "http://" + l.Addr().String(), func() {
		s.mu.Lock()
		if s.organizeCancel != nil {
			s.organizeCancel()
		}
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		s.stopOperations()
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.operationRoutes(mux)
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.snapshot()
		s.respond(w, v, err)
	})
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		// Lifecycle timestamps belong to the store, not the web client.
		var input struct {
			ID         string   `json:"id"`
			Title      string   `json:"title"`
			Notes      string   `json:"notes"`
			Project    string   `json:"project"`
			Directory  string   `json:"directory"`
			DueDate    string   `json:"dueDate"`
			Priority   int      `json:"priority"`
			Status     string   `json:"status"`
			SessionIDs []string `json:"sessionIds"`
		}
		if !s.decode(w, r, &input) {
			return
		}
		t := model.Task{ID: input.ID, Title: input.Title, Notes: input.Notes, Project: input.Project, Directory: input.Directory, DueDate: input.DueDate, Priority: input.Priority, Status: input.Status, SessionIDs: input.SessionIDs}
		v, err := s.store.SaveTask(t)
		s.respond(w, v, err)
	})
	mux.HandleFunc("DELETE /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.respond(w, map[string]bool{"ok": true}, s.store.DeleteTask(r.PathValue("id")))
	})
	mux.HandleFunc("PATCH /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		var edit store.TaskEdit
		if !s.decode(w, r, &edit) {
			return
		}
		task, err := s.store.EditTask(r.PathValue("id"), edit)
		s.respond(w, task, err)
	})
	mux.HandleFunc("POST /api/tasks/reorder", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			TaskIDs []string `json:"taskIds"`
		}
		if !s.decode(w, r, &input) {
			return
		}
		s.respond(w, map[string]bool{"ok": true}, s.store.ReorderTasks(input.TaskIDs))
	})
	mux.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var v model.Settings
		if !s.decode(w, r, &v) {
			return
		}
		for _, root := range []struct {
			path    string
			enabled bool
		}{{v.CodexRoot, v.CodexEnabled}, {v.ClaudeRoot, v.ClaudeEnabled}} {
			if root.enabled {
				info, err := os.Stat(sessions.Expand(root.path))
				if err != nil {
					s.respond(w, nil, fmt.Errorf("session folder: %w", err))
					return
				}
				if !info.IsDir() {
					s.respond(w, nil, errors.New("session location must be a folder"))
					return
				}
			}
		}
		s.respond(w, v, s.store.SaveSettings(v))
	})
	mux.HandleFunc("POST /api/sync", func(w http.ResponseWriter, r *http.Request) {
		settings, err := s.store.Settings()
		if err != nil {
			s.respond(w, nil, err)
			return
		}
		report := sessions.Scan(settings, time.Now())
		s.respond(w, map[string]any{"count": len(report.Sessions), "warnings": report.Warnings}, s.store.SaveSessions(report.Sessions))
	})
	mux.HandleFunc("POST /api/organize", s.organize)
	mux.HandleFunc("POST /api/organize/cancel", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		if s.organizeCancel != nil {
			s.organizeCancel()
		}
		s.mu.Unlock()
		s.respond(w, map[string]bool{"ok": true}, nil)
	})
	mux.HandleFunc("GET /api/export", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.snapshot()
		if err != nil {
			s.respond(w, nil, err)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="listodo-backup.json"`)
		s.respond(w, struct {
			Version    int             `json:"version"`
			ExportedAt time.Time       `json:"exportedAt"`
			Tasks      []model.Task    `json:"tasks"`
			Sessions   []model.Session `json:"sessions"`
			Settings   model.Settings  `json:"settings"`
			Events     []model.Event   `json:"events"`
		}{1, time.Now(), v.Tasks, v.Sessions, v.Settings, v.Events}, nil)
	})
	mux.HandleFunc("POST /api/export/save", func(w http.ResponseWriter, r *http.Request) {
		if !s.Native {
			s.respond(w, nil, errors.New("native export requires the desktop app"))
			return
		}
		v, err := s.snapshot()
		if err != nil {
			s.respond(w, nil, err)
			return
		}
		data, err := json.MarshalIndent(map[string]any{"version": 1, "exportedAt": time.Now(), "tasks": v.Tasks, "sessions": v.Sessions, "settings": v.Settings, "events": v.Events}, "", "  ")
		if err != nil {
			s.respond(w, nil, err)
			return
		}
		saved, err := saveBackup(data)
		s.respond(w, map[string]bool{"saved": saved}, err)
	})
	files, _ := fs.Sub(assets, "dist")
	static := http.FileServer(http.FS(files))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			index, err := fs.ReadFile(files, "index.html")
			if err != nil {
				http.Error(w, "Frontend unavailable", 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(strings.Replace(string(index), "<head>", `<head><meta name="listodo-token" content="`+s.token+`">`, 1)))
			return
		}
		static.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https: http:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Listodo-Token")), []byte(s.token)) != 1 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) snapshot() (snapshot, error) {
	v := snapshot{Database: s.store.Path}
	var err error
	if v.Tasks, err = s.store.Tasks(); err != nil {
		return v, err
	}
	if v.Sessions, err = s.store.Sessions(); err != nil {
		return v, err
	}
	if v.Settings, err = s.store.Settings(); err != nil {
		return v, err
	}
	if v.Events, err = s.store.Events(""); err != nil {
		return v, err
	}
	sort.Slice(v.Sessions, func(i, j int) bool { return v.Sessions[i].UpdatedAt.After(v.Sessions[j].UpdatedAt) })
	return v, nil
}

func (s *Server) organize(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Provider     string `json:"provider"`
		Instructions string `json:"instructions"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	s.mu.Lock()
	if s.organizeCancel != nil {
		s.mu.Unlock()
		s.respond(w, nil, errors.New("organizer is already running"))
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	s.organizeCancel = cancel
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); s.organizeCancel = nil; s.mu.Unlock() }()
	v, err := s.snapshot()
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	if strings.TrimSpace(input.Instructions) == "" && len(v.Tasks) == 0 {
		s.respond(w, nil, errors.New("describe your plans so the organizer has something to work with"))
		return
	}
	recent := []model.Session{}
	for _, session := range v.Sessions {
		if model.Day(session.UpdatedAt) == model.Day(time.Now()) {
			recent = append(recent, session)
		}
	}
	proposal, err := harness.Organize(ctx, input.Provider, input.Instructions, v.Tasks, recent)
	s.respond(w, proposal, err)
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		s.respond(w, nil, fmt.Errorf("invalid request: %w", err))
		return false
	}
	return true
}
func (s *Server) respond(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}
