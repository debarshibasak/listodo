package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db   *sql.DB
	Path string
}

func DefaultDir() (string, error) {
	if dir := os.Getenv("LISTODO_DATA_DIR"); dir != "" {
		return dir, nil
	}
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "Listodo"), err
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path, err := filepath.Abs(filepath.Join(dir, "listodo.db"))
	if err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=FULL"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS settings (id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY, task_id TEXT NOT NULL, title TEXT NOT NULL, action TEXT NOT NULL, at TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS operations (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	CREATE UNIQUE INDEX IF NOT EXISTS one_running_operation_per_task ON operations(json_extract(data, '$.taskId')) WHERE json_extract(data, '$.status') = 'running';
	PRAGMA user_version=1;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	// Restore folder associations saved before tasks had their own directory field.
	_, err = db.Exec(`UPDATE tasks SET data = json_set(data, '$.directory', COALESCE(
		(SELECT json_extract(operations.data, '$.directory') FROM operations
		 WHERE json_extract(operations.data, '$.taskId') = tasks.id
		   AND COALESCE(json_extract(operations.data, '$.directory'), '') != ''
		 ORDER BY json_extract(operations.data, '$.startedAt') DESC LIMIT 1),
		(SELECT json_extract(sessions.data, '$.directory') FROM sessions
		 JOIN json_each(tasks.data, '$.sessionIds') AS linked ON linked.value = sessions.id
		 WHERE COALESCE(json_extract(sessions.data, '$.directory'), '') != ''
		 ORDER BY json_extract(sessions.data, '$.updatedAt') DESC LIMIT 1),
		'')) WHERE json_type(data, '$.directory') IS NULL`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, Path: path}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Tasks() ([]model.Task, error) {
	rows, err := s.db.Query("SELECT data FROM tasks")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []model.Task{}
	for rows.Next() {
		var raw string
		var t model.Task
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &t); err != nil {
			return nil, err
		}
		t.StartDate = t.StartDay()
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *Store) SaveTask(t model.Task) (model.Task, error) {
	return s.saveTask(t, nil)
}

// TaskEdit contains only fields changed by the task editor. An omitted status
// preserves status updates made by agent runs while the editor is open.
type TaskEdit struct {
	Title    string  `json:"title"`
	Notes    string  `json:"notes"`
	Priority int     `json:"priority"`
	Status   *string `json:"status,omitempty"`
	DueDate  *string `json:"dueDate,omitempty"`
}

func (s *Store) EditTask(id string, edit TaskEdit) (model.Task, error) {
	if id == "" {
		return model.Task{}, errors.New("choose an existing task to edit")
	}
	return s.saveTask(model.Task{ID: id}, &edit)
}

func (s *Store) saveTask(t model.Task, edit *TaskEdit) (model.Task, error) {
	if edit == nil {
		t.Title = strings.TrimSpace(t.Title)
		t.Project = strings.TrimSpace(t.Project)
		t.Directory = strings.TrimSpace(t.Directory)
		if err := t.Validate(); err != nil {
			return t, err
		}
	}
	now := time.Now()
	action := "created"
	tx, err := s.db.Begin()
	if err != nil {
		return t, err
	}
	defer tx.Rollback()
	if t.ID == "" {
		if err := tx.QueryRow("SELECT COALESCE(MAX(CAST(json_extract(data, '$.order') AS INTEGER)),0)+1 FROM tasks").Scan(&t.Order); err != nil {
			return t, err
		}
		t.ID = model.ID()
		t.CreatedAt = now
		t.StartDate = model.Day(now)
		t.CompletedAt = nil
	} else {
		var raw string
		if err := tx.QueryRow("SELECT data FROM tasks WHERE id=?", t.ID).Scan(&raw); err != nil {
			return t, err
		}
		var old model.Task
		if err := json.Unmarshal([]byte(raw), &old); err != nil {
			return t, err
		}
		if edit != nil {
			t = old
			t.Title, t.Notes, t.Priority = strings.TrimSpace(edit.Title), edit.Notes, edit.Priority
			if edit.Status != nil {
				t.Status = *edit.Status
			}
			if edit.DueDate != nil {
				t.DueDate = *edit.DueDate
			}
			if t.Title == old.Title && t.Notes == old.Notes && t.Priority == old.Priority && t.Status == old.Status && t.DueDate == old.DueDate {
				return old, nil
			}
		}
		t.CreatedAt = old.CreatedAt
		t.Order = old.Order
		t.StartDate = old.StartDay()
		t.CompletedAt = old.CompletedAt
		action = "updated"
		if old.Status != t.Status {
			action = t.Status
		}
	}
	if edit != nil {
		if err := t.Validate(); err != nil {
			return t, err
		}
	}
	if t.Directory == "" && len(t.SessionIDs) > 0 {
		linked, err := json.Marshal(t.SessionIDs)
		if err != nil {
			return t, err
		}
		err = tx.QueryRow(`SELECT COALESCE((
			SELECT json_extract(sessions.data, '$.directory') FROM sessions
			JOIN json_each(?) AS linked ON linked.value = sessions.id
			WHERE COALESCE(json_extract(sessions.data, '$.directory'), '') != ''
			ORDER BY json_extract(sessions.data, '$.updatedAt') DESC LIMIT 1), '')`, string(linked)).Scan(&t.Directory)
		if err != nil {
			return t, err
		}
	}
	if t.Status == "done" && t.CompletedAt == nil {
		t.CompletedAt = &now
		action = "done"
	}
	if t.Status != "done" {
		t.CompletedAt = nil
	}
	t.UpdatedAt = now
	raw, err := json.Marshal(t)
	if err != nil {
		return t, err
	}
	if _, err = tx.Exec("INSERT INTO tasks(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", t.ID, string(raw)); err != nil {
		return t, err
	}
	if _, err = tx.Exec("INSERT INTO events(task_id,title,action,at) VALUES(?,?,?,?)", t.ID, t.Title, action, now.Format(time.RFC3339Nano)); err != nil {
		return t, err
	}
	return t, tx.Commit()
}

func (s *Store) DeleteTask(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var running int
	if err := tx.QueryRow("SELECT count(*) FROM operations WHERE json_extract(data, '$.taskId')=? AND json_extract(data, '$.status')='running'", id).Scan(&running); err != nil {
		return err
	}
	if running > 0 {
		return errors.New("stop the running agent before deleting this task")
	}
	var raw string
	if err := tx.QueryRow("SELECT data FROM tasks WHERE id=?", id).Scan(&raw); err != nil {
		return err
	}
	var t model.Task
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM tasks WHERE id=?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO events(task_id,title,action,at) VALUES(?,?,?,?)", id, t.Title, "deleted", time.Now().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Sessions() ([]model.Session, error) {
	rows, err := s.db.Query("SELECT data FROM sessions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Session{}
	for rows.Next() {
		var raw string
		var v model.Session
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (s *Store) SaveSessions(sessions []model.Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare("INSERT INTO sessions(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, v := range sessions {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(v.ID, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Settings() (model.Settings, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return model.Settings{}, err
	}
	v := model.Settings{Harness: "codex", CodexRoot: filepath.Join(home, ".codex", "sessions"), ClaudeRoot: filepath.Join(home, ".claude", "projects"), CodexEnabled: true, ClaudeEnabled: true, AutoSync: true}
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		v.CodexRoot = filepath.Join(codexHome, "sessions")
	}
	var raw string
	err = s.db.QueryRow("SELECT data FROM settings WHERE id=1").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	err = json.Unmarshal([]byte(raw), &v)
	return v, err
}

func (s *Store) SaveSettings(v model.Settings) error {
	if v.Harness != "codex" && v.Harness != "claude" {
		return errors.New("choose Codex or Claude")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO settings(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(raw))
	return err
}

func (s *Store) Events(day string) ([]model.Event, error) {
	rows, err := s.db.Query("SELECT id,task_id,title,action,at FROM events ORDER BY id DESC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Event{}
	for rows.Next() {
		var v model.Event
		var at string
		if err := rows.Scan(&v.ID, &v.TaskID, &v.Title, &v.Action, &at); err != nil {
			return nil, err
		}
		var err error
		v.At, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, fmt.Errorf("event timestamp: %w", err)
		}
		if day == "" || model.Day(v.At) == day {
			result = append(result, v)
		}
	}
	return result, rows.Err()
}
