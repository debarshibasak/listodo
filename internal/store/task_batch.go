package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
)

func selectedTasks(tx *sql.Tx, ids []string, minimum int) ([]model.Task, []taskChange, error) {
	if len(ids) < minimum || len(ids) > 10000 {
		return nil, nil, fmt.Errorf("select between %d and 10000 tasks", minimum)
	}
	seen := map[string]bool{}
	tasks := make([]model.Task, 0, len(ids))
	changes := make([]taskChange, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			return nil, nil, errors.New("select each task only once")
		}
		seen[id] = true
		var raw string
		if err := tx.QueryRow("SELECT data FROM tasks WHERE id=?", id).Scan(&raw); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil, errors.New("a selected task was deleted; refresh and try again")
			}
			return nil, nil, err
		}
		var task model.Task
		if err := json.Unmarshal([]byte(raw), &task); err != nil {
			return nil, nil, err
		}
		tasks = append(tasks, task)
		changes = append(changes, taskChange{ID: id, Before: raw, After: raw})
	}
	return tasks, changes, nil
}

func writeBatchTask(tx *sql.Tx, task model.Task, action string, now time.Time) (string, error) {
	raw, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec("UPDATE tasks SET data=? WHERE id=?", string(raw), task.ID); err != nil {
		return "", err
	}
	_, err = tx.Exec("INSERT INTO events(task_id,title,action,at) VALUES(?,?,?,?)", task.ID, task.Title, action, now.Format(time.RFC3339Nano))
	return string(raw), err
}

// CloseTasks completes the selection atomically and records one undo step.
func (s *Store) CloseTasks(ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tasks, changes, err := selectedTasks(tx, ids, 1)
	if err != nil {
		return err
	}
	now := time.Now()
	for i, task := range tasks {
		if task.Status == "done" {
			continue
		}
		task.Status, task.CompletedAt, task.UpdatedAt = "done", &now, now
		changes[i].After, err = writeBatchTask(tx, task, "done", now)
		if err != nil {
			return err
		}
	}
	if err := recordTaskUndo(tx, "close tasks", changes, nil); err != nil {
		return err
	}
	return tx.Commit()
}

// MergeTasks keeps the first selected task's identity and title. Other titles
// become note headings; sessions and run history stay attached to the survivor.
func (s *Store) MergeTasks(ids []string) (model.Task, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return model.Task{}, err
	}
	defer tx.Rollback()
	tasks, changes, err := selectedTasks(tx, ids, 2)
	if err != nil {
		return model.Task{}, err
	}
	for _, task := range tasks {
		if err := taskNotRunning(tx, task.ID, "merging these tasks"); err != nil {
			return model.Task{}, err
		}
	}
	merged := tasks[0]
	merged.SessionIDs = nil
	seenSessions := map[string]bool{}
	allDone, working := true, false
	for i, task := range tasks {
		allDone = allDone && task.Status == "done"
		working = working || task.Status == "working"
		if task.Priority > merged.Priority {
			merged.Priority = task.Priority
		}
		if task.DueDate != "" && (merged.DueDate == "" || task.DueDate < merged.DueDate) {
			merged.DueDate = task.DueDate
		}
		if merged.Project == "" {
			merged.Project = task.Project
		}
		if merged.Directory == "" {
			merged.Directory = task.Directory
		}
		for _, id := range task.SessionIDs {
			if !seenSessions[id] {
				merged.SessionIDs = append(merged.SessionIDs, id)
				seenSessions[id] = true
			}
		}
		if i == 0 {
			continue
		}
		section := "## " + task.Title
		// Preserve differing folder/project associations in the combined notes.
		if task.Project != "" && task.Project != merged.Project {
			section += "\n\nProject: " + task.Project
		}
		if task.Directory != "" && task.Directory != merged.Directory {
			section += "\n\nFolder: " + task.Directory
		}
		if task.Notes != "" {
			section += "\n\n" + task.Notes
		}
		if merged.Notes != "" {
			merged.Notes += "\n\n---\n\n"
		}
		merged.Notes += section
	}
	if !allDone {
		merged.Status, merged.CompletedAt = "todo", nil
		if working {
			merged.Status = "working"
		}
	}
	merged.UpdatedAt = time.Now()
	if err := merged.Validate(); err != nil {
		return model.Task{}, fmt.Errorf("cannot merge tasks: %w", err)
	}
	var moves []operationMove
	for _, task := range tasks[1:] {
		rows, err := tx.Query("SELECT id FROM operations WHERE json_extract(data, '$.taskId')=?", task.ID)
		if err != nil {
			return model.Task{}, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return model.Task{}, err
			}
			moves = append(moves, operationMove{ID: id, From: task.ID, To: merged.ID})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return model.Task{}, err
		}
		if _, err := tx.Exec("UPDATE operations SET data=json_set(data, '$.taskId', ?) WHERE json_extract(data, '$.taskId')=?", merged.ID, task.ID); err != nil {
			return model.Task{}, err
		}
	}
	changes[0].After, err = writeBatchTask(tx, merged, "merged", merged.UpdatedAt)
	if err != nil {
		return model.Task{}, err
	}
	for i, task := range tasks[1:] {
		if _, err := tx.Exec("DELETE FROM tasks WHERE id=?", task.ID); err != nil {
			return model.Task{}, err
		}
		if _, err := tx.Exec("INSERT INTO events(task_id,title,action,at) VALUES(?,?,?,?)", task.ID, task.Title, "merged into "+merged.ID, merged.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
			return model.Task{}, err
		}
		changes[i+1].After = ""
	}
	if err := recordTaskUndo(tx, "merge tasks", changes, moves); err != nil {
		return model.Task{}, err
	}
	return merged, tx.Commit()
}
