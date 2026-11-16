package store

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/adaptive-scale/listodo/internal/model"
)

// ReorderTasks permutes the selected tasks' slots, preserving every hidden task.
func (s *Store) ReorderTasks(ids []string) error {
	if len(ids) < 2 || len(ids) > 10000 {
		return errors.New("choose at least two tasks to reorder")
	}
	selected := map[string]bool{}
	for _, id := range ids {
		if selected[id] {
			return errors.New("a task appears twice in the requested order")
		}
		selected[id] = true
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query("SELECT data FROM tasks")
	if err != nil {
		return err
	}
	var tasks []model.Task
	byID := map[string]model.Task{}
	before := map[string]string{}
	for rows.Next() {
		var raw string
		var task model.Task
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(raw), &task); err != nil {
			rows.Close()
			return err
		}
		tasks = append(tasks, task)
		byID[task.ID] = task
		before[task.ID] = raw
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := byID[id]; !ok {
			return errors.New("a task was deleted; refresh and try again")
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Order != tasks[j].Order {
			return tasks[i].Order < tasks[j].Order
		}
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}
		return tasks[i].ID < tasks[j].ID
	})
	next := 0
	var changes []taskChange
	for i, task := range tasks {
		if selected[task.ID] {
			task = byID[ids[next]]
			next++
		}
		task.Order = int64(i + 1)
		raw, err := json.Marshal(task)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE tasks SET data=? WHERE id=?", string(raw), task.ID); err != nil {
			return err
		}
		changes = append(changes, taskChange{ID: task.ID, Before: before[task.ID], After: string(raw)})
	}
	if err := recordTaskUndo(tx, "reorder tasks", changes, nil); err != nil {
		return err
	}
	return tx.Commit()
}
