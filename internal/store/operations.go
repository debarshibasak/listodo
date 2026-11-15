package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
)

func (s *Store) Task(id string) (model.Task, error) {
	var task model.Task
	var raw string
	if err := s.db.QueryRow("SELECT data FROM tasks WHERE id=?", id).Scan(&raw); err != nil {
		return task, err
	}
	err := json.Unmarshal([]byte(raw), &task)
	task.StartDate = task.StartDay()
	return task, err
}

func (s *Store) Operations() ([]model.Operation, error) {
	rows, err := s.db.Query("SELECT data FROM operations ORDER BY json_extract(data, '$.startedAt') DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Operation{}
	for rows.Next() {
		var raw string
		var op model.Operation
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &op); err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, rows.Err()
}

func (s *Store) Operation(id string) (model.Operation, error) {
	var op model.Operation
	var raw string
	if err := s.db.QueryRow("SELECT data FROM operations WHERE id=?", id).Scan(&raw); err != nil {
		return op, err
	}
	err := json.Unmarshal([]byte(raw), &op)
	return op, err
}

func (s *Store) SaveOperation(op model.Operation) error {
	raw, err := json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE operations SET data=? WHERE id=?", string(raw), op.ID)
	return err
}

// BeginOperation atomically reserves the task and starts its work status.
func (s *Store) BeginOperation(op model.Operation) error {
	return s.beginOperation(op, false)
}

func (s *Store) ContinueOperation(op model.Operation) error {
	return s.beginOperation(op, true)
}

func (s *Store) beginOperation(op model.Operation, continuing bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if continuing {
		if err := tx.QueryRow("SELECT data FROM operations WHERE id=?", op.ID).Scan(&raw); err != nil {
			return err
		}
		var previous model.Operation
		if err := json.Unmarshal([]byte(raw), &previous); err != nil {
			return err
		}
		turn := previous.Turn
		if turn == 0 {
			turn = 1
		}
		if previous.Status == "running" || turn+1 != op.Turn || previous.TaskID != op.TaskID || previous.SessionID != op.SessionID {
			return errors.New("this session has changed; refresh before sending another prompt")
		}
	}
	if err := tx.QueryRow("SELECT data FROM tasks WHERE id=?", op.TaskID).Scan(&raw); err != nil {
		return err
	}
	var task model.Task
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		return err
	}
	if task.Status == "done" {
		return errors.New("reopen this task before running an agent")
	}
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM operations WHERE json_extract(data, '$.taskId')=? AND json_extract(data, '$.status')='running'", task.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("an agent is already running this task")
	}
	data, err := json.Marshal(op)
	if err != nil {
		return err
	}
	query := "INSERT INTO operations(data,id) VALUES(?,?)"
	if continuing {
		query = "UPDATE operations SET data=? WHERE id=?"
	}
	if _, err := tx.Exec(query, string(data), op.ID); err != nil {
		return err
	}
	task.Status = "working"
	task.Directory = op.Directory
	task.UpdatedAt = time.Now()
	data, err = json.Marshal(task)
	if err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE tasks SET data=? WHERE id=?", string(data), task.ID); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO events(task_id,title,action,at) VALUES(?,?,?,?)", task.ID, task.Title, "working", task.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

// Link only the new session, preserving concurrent edits and completion changes.
func (s *Store) LinkTaskSession(taskID, sessionID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	err = tx.QueryRow("SELECT data FROM tasks WHERE id=?", taskID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} // History survives task deletion.
	if err != nil {
		return err
	}
	var task model.Task
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		return err
	}
	for _, id := range task.SessionIDs {
		if id == sessionID {
			return nil
		}
	}
	task.SessionIDs = append(task.SessionIDs, sessionID)
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE tasks SET data=? WHERE id=?", string(data), taskID); err != nil {
		return err
	}
	return tx.Commit()
}
