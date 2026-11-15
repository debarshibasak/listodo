package model

import (
	"testing"
	"time"
)

func TestPriorityEscalatesByCalendarDay(t *testing.T) {
	now := time.Date(2026, 11, 13, 0, 10, 0, 0, time.Local)
	for _, tc := range []struct {
		due        string
		base, want int
	}{{"2026-11-13", 1, 1}, {"2026-11-12", 1, 2}, {"2026-11-11", 1, 3}, {"2026-10-01", 0, 3}, {"2026-11-14", 2, 2}, {"", 0, 0}} {
		task := Task{DueDate: tc.due, Priority: tc.base, Status: "todo"}
		if got := task.EffectivePriority(now); got != tc.want {
			t.Errorf("due %s: got %d, want %d", tc.due, got, tc.want)
		}
	}
	task := Task{DueDate: "2026-01-01", Priority: 0, Status: "done"}
	if task.EffectivePriority(now) != 0 {
		t.Fatal("completed tasks must not escalate")
	}
}

func TestTodayUsesCompletionDate(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	task := Task{Status: "done", DueDate: Day(now), CompletedAt: &yesterday}
	if task.IsToday(now) {
		t.Fatal("yesterday's completions should not count today")
	}
	task.CompletedAt = &now
	if !task.IsToday(now) {
		t.Fatal("today's completion missing")
	}
	task = Task{Status: "working"}
	if !task.IsToday(now) {
		t.Fatal("unscheduled work should appear today")
	}
}

func TestCreationDayStartsTasksWithoutADeadline(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.Local)
	created := now.AddDate(0, 0, -2)
	task := Task{Title: "Write a task", Status: "todo", Priority: 1, CreatedAt: created}
	if !task.IsToday(now) || task.StartDay() != "2026-11-11" {
		t.Fatal("open tasks should remain visible from their creation date")
	}
	if task.EffectivePriority(now) != 1 {
		t.Fatal("a start date must not create an overdue deadline")
	}
	task.DueDate = "2026-11-14"
	if task.IsToday(now) {
		t.Fatal("existing future schedules must be preserved")
	}
	// A stored creation day must survive a later timezone change.
	task.CreatedAt = time.Date(2026, 11, 12, 0, 15, 0, 0, time.FixedZone("UTC+14", 14*3600))
	if task.StartDay() != "2026-11-12" {
		t.Fatal("start date shifted to another timezone")
	}
}
