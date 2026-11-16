package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// Empty snapshots represent absent tasks. History is stored in the same
// transaction as the change, so a failed write can never consume an undo step.
type taskChange struct {
	ID     string `json:"id"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type operationMove struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type taskUndo struct {
	Label      string          `json:"label"`
	Tasks      []taskChange    `json:"tasks"`
	Operations []operationMove `json:"operations,omitempty"`
}

func recordTaskUndo(tx *sql.Tx, label string, changes []taskChange, moves []operationMove) error {
	changed := make([]taskChange, 0, len(changes))
	for _, change := range changes {
		if change.Before != change.After {
			changed = append(changed, change)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	raw, err := json.Marshal(taskUndo{Label: label, Tasks: changed, Operations: moves})
	if err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO task_undo(data) VALUES(?)", string(raw)); err != nil {
		return err
	}
	// Bound history without expiring it on a refresh or app restart.
	_, err = tx.Exec("DELETE FROM task_undo WHERE id NOT IN (SELECT id FROM task_undo ORDER BY id DESC LIMIT 100)")
	return err
}

func taskNotRunning(tx *sql.Tx, id, action string) error {
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM operations WHERE json_extract(data, '$.taskId')=? AND json_extract(data, '$.status')='running'", id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("stop the running agent before %s", action)
	}
	return nil
}

// Reverse only the fields this action changed. Session links and other fields
// updated by an agent in the meantime must survive an undo.
func reverseTaskChange(change taskChange, current string) (string, error) {
	conflict := errors.New("this task changed since that action; undo would overwrite newer changes")
	if change.Before == "" || change.After == "" || current == "" {
		if current != change.After {
			return "", conflict
		}
		return change.Before, nil
	}
	var before, after, now map[string]json.RawMessage
	for _, item := range []struct {
		raw string
		out *map[string]json.RawMessage
	}{{change.Before, &before}, {change.After, &after}, {current, &now}} {
		if err := json.Unmarshal([]byte(item.raw), item.out); err != nil {
			return "", err
		}
	}
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	for key := range keys {
		if reflect.DeepEqual(before[key], after[key]) {
			continue
		}
		if !reflect.DeepEqual(now[key], after[key]) {
			if key == "updatedAt" {
				continue
			}
			return "", conflict
		}
		if value, ok := before[key]; ok {
			now[key] = value
		} else {
			delete(now, key)
		}
	}
	// Preserve the exact original snapshot when there were no concurrent changes.
	if reflect.DeepEqual(now, before) {
		return change.Before, nil
	}
	raw, err := json.Marshal(now)
	return string(raw), err
}

func (s *Store) UndoTasks() (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var id int64
	var raw string
	if err := tx.QueryRow("SELECT id,data FROM task_undo ORDER BY id DESC LIMIT 1").Scan(&id, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("no task changes to undo")
		}
		return "", err
	}
	var undo taskUndo
	if err := json.Unmarshal([]byte(raw), &undo); err != nil {
		return "", err
	}
	for _, change := range undo.Tasks {
		if err := taskNotRunning(tx, change.ID, "undoing this task change"); err != nil {
			return "", err
		}
		var current string
		err := tx.QueryRow("SELECT data FROM tasks WHERE id=?", change.ID).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		restored, err := reverseTaskChange(change, current)
		if err != nil {
			return "", err
		}
		if restored == "" {
			_, err = tx.Exec("DELETE FROM tasks WHERE id=?", change.ID)
		} else {
			_, err = tx.Exec("INSERT INTO tasks(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", change.ID, restored)
		}
		if err != nil {
			return "", err
		}
		eventTask := restored
		if eventTask == "" {
			eventTask = change.After
		}
		if _, err := tx.Exec("INSERT INTO events(task_id,title,action,at) VALUES(?,json_extract(?, '$.title'),?,?)", change.ID, eventTask, "undid "+undo.Label, time.Now().Format(time.RFC3339Nano)); err != nil {
			return "", err
		}
	}
	for _, move := range undo.Operations {
		result, err := tx.Exec("UPDATE operations SET data=json_set(data, '$.taskId', ?) WHERE id=? AND json_extract(data, '$.taskId')=? AND json_extract(data, '$.status')!='running'", move.From, move.ID, move.To)
		if err != nil {
			return "", err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return "", err
		}
		if count != 1 {
			return "", errors.New("run history changed since the merge; cannot undo it")
		}
	}
	if _, err := tx.Exec("DELETE FROM task_undo WHERE id=?", id); err != nil {
		return "", err
	}
	return undo.Label, tx.Commit()
}
