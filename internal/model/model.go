package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Notes       string     `json:"notes"`
	Project     string     `json:"project"`
	Directory   string     `json:"directory"`
	StartDate   string     `json:"startDate"`
	DueDate     string     `json:"dueDate"`
	Priority    int        `json:"priority"`
	Status      string     `json:"status"`
	Order       int64      `json:"order"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	SessionIDs  []string   `json:"sessionIds"`
}

type Session struct {
	ID         string    `json:"id"`
	Harness    string    `json:"harness"`
	Title      string    `json:"title"`
	Project    string    `json:"project"`
	Directory  string    `json:"directory"`
	SourcePath string    `json:"sourcePath"`
	Summary    string    `json:"summary"`
	StartedAt  time.Time `json:"startedAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Status     string    `json:"status"`
	Messages   int       `json:"messages"`
}

// Operation is a task execution started by Listodo, separate from imported sessions.
type Operation struct {
	ID         string     `json:"id"`
	Turn       int        `json:"turn,omitempty"`
	TaskID     string     `json:"taskId"`
	Title      string     `json:"title"`
	Prompt     string     `json:"prompt,omitempty"`
	Provider   string     `json:"provider"`
	Directory  string     `json:"directory"`
	Status     string     `json:"status"`
	SessionID  string     `json:"sessionId"`
	Output     string     `json:"output,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	OwnerPID   int        `json:"ownerPid,omitempty"`
}

type Settings struct {
	Harness       string `json:"harness"`
	CodexRoot     string `json:"codexRoot"`
	ClaudeRoot    string `json:"claudeRoot"`
	CodexEnabled  bool   `json:"codexEnabled"`
	ClaudeEnabled bool   `json:"claudeEnabled"`
	AutoSync      bool   `json:"autoSync"`
}

type Event struct {
	ID     int64
	TaskID string
	Title  string
	Action string
	At     time.Time
}

func ID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func Day(t time.Time) string { return t.Local().Format("2006-01-02") }

// StartDay retains the creation calendar date, including for older saved tasks.
func (t Task) StartDay() string {
	if t.StartDate != "" {
		return t.StartDate
	}
	if t.CreatedAt.IsZero() {
		return ""
	}
	return t.CreatedAt.Format("2006-01-02")
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.Title) == "" {
		return errors.New("give your task a title")
	}
	if len(t.Title) > 500 || len(t.Notes) > 30000 || len(t.Project) > 200 {
		return errors.New("task text is too long")
	}
	if t.Priority < 0 || t.Priority > 3 {
		return errors.New("priority must be between 0 and 3")
	}
	if t.Status != "todo" && t.Status != "working" && t.Status != "done" {
		return errors.New("unknown task status")
	}
	if t.DueDate != "" {
		if _, err := time.Parse("2006-01-02", t.DueDate); err != nil {
			return errors.New("use YYYY-MM-DD for the due date")
		}
	}
	return nil
}

// Calendar dates, rather than 24-hour durations, keep escalation correct across DST.
func (t Task) OverdueDays(now time.Time) int {
	if t.Status == "done" || t.DueDate == "" {
		return 0
	}
	due, err := time.Parse("2006-01-02", t.DueDate)
	if err != nil {
		return 0
	}
	today, _ := time.Parse("2006-01-02", Day(now))
	days := int(today.Sub(due).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func (t Task) EffectivePriority(now time.Time) int {
	p := t.Priority + t.OverdueDays(now)
	if p > 3 {
		return 3
	}
	return p
}

func PriorityName(p int) string {
	if p < 0 || p > 3 {
		return "Normal"
	}
	return []string{"Low", "Normal", "High", "Urgent"}[p]
}

func (t Task) PriorityReason(now time.Time) string {
	days := t.OverdueDays(now)
	if days == 0 {
		return PriorityName(t.Priority)
	}
	return fmt.Sprintf("%s · %dd overdue · originally %s", PriorityName(t.EffectivePriority(now)), days, strings.ToLower(PriorityName(t.Priority)))
}

func (t Task) IsToday(now time.Time) bool {
	if t.Status == "done" {
		return t.CompletedAt != nil && Day(*t.CompletedAt) == Day(now)
	}
	return t.Status == "working" || (t.DueDate != "" && t.DueDate <= Day(now)) ||
		(t.DueDate == "" && t.StartDay() != "" && t.StartDay() <= Day(now))
}

func (s Session) IsActive(now time.Time) bool {
	return s.Status == "working" && now.Sub(s.UpdatedAt) >= 0 && now.Sub(s.UpdatedAt) < 3*time.Minute
}
