// Package sessions reads local harness transcripts without changing them.
// Transcript formats are best-effort adapters, not public provider APIs.
package sessions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
)

type Report struct {
	Sessions []model.Session
	Warnings []string
	Files    int
}
type candidate struct {
	path, harness string
	modified      time.Time
}

func Expand(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path
}

func Scan(settings model.Settings, now time.Time) Report {
	report := Report{Sessions: []model.Session{}}
	files := []candidate{}
	roots := []struct {
		path, harness string
		enabled       bool
	}{{settings.CodexRoot, "codex", settings.CodexEnabled}, {settings.ClaudeRoot, "claude", settings.ClaudeEnabled}}
	for _, root := range roots {
		if !root.enabled {
			continue
		}
		base := Expand(root.path)
		if _, err := os.Stat(base); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %v", root.harness, err))
			continue
		}
		visited := 0
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			visited++
			if visited > 50000 {
				return fmt.Errorf("directory is too large; choose a narrower session folder")
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if now.Sub(info.ModTime()) > 14*24*time.Hour {
				return nil
			}
			files = append(files, candidate{path, root.harness, info.ModTime()})
			return nil
		})
		if err != nil {
			report.Warnings = append(report.Warnings, root.harness+": "+err.Error())
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified.After(files[j].modified) })
	if len(files) > 400 {
		files = files[:400]
		report.Warnings = append(report.Warnings, "Import limited to the 400 most recently modified transcripts.")
	}
	seen := map[string]bool{}
	for _, f := range files {
		v, err := ParseFile(f.path, f.harness, now)
		report.Files++
		if err != nil {
			report.Warnings = append(report.Warnings, filepath.Base(f.path)+": "+err.Error())
			continue
		}
		if v.Title == "" || seen[v.ID] {
			continue
		}
		seen[v.ID] = true
		report.Sessions = append(report.Sessions, v)
	}
	sort.Slice(report.Sessions, func(i, j int) bool { return report.Sessions[i].UpdatedAt.After(report.Sessions[j].UpdatedAt) })
	return report
}

func text(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	blocks, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := []string{}
	for _, b := range blocks {
		if m, ok := b.(map[string]any); ok {
			kind, _ := m["type"].(string)
			if kind == "text" || kind == "input_text" || kind == "output_text" {
				if s, ok := m["text"].(string); ok {
					parts = append(parts, s)
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

func str(m map[string]any, key string) string { s, _ := m[key].(string); return s }
func clean(s string) string {
	// Keep actual task descriptions; discard injected environment/instruction blocks.
	for _, tag := range []string{"environment_context", "instructions", "system-reminder", "user_instructions", "INSTRUCTIONS"} {
		for {
			start := strings.Index(s, "<"+tag+">")
			if start < 0 {
				break
			}
			end := strings.Index(s[start:], "</"+tag+">")
			if end < 0 {
				return ""
			}
			s = s[:start] + s[start+end+len(tag)+3:]
		}
	}
	if strings.HasPrefix(strings.TrimSpace(s), "# AGENTS.md instructions") {
		return ""
	}
	return strings.TrimSpace(s)
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func ParseFile(path, harness string, now time.Time) (model.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.Session{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return model.Session{}, err
	}
	// Read the beginning for metadata and the tail for recent activity. Large tool
	// output is never loaded wholesale, and partial JSONL writes are ignored.
	var reader io.Reader = f
	if info.Size() > 4<<20 {
		head := io.NewSectionReader(f, 0, 512<<10)
		tail := io.NewSectionReader(f, info.Size()-(3<<20), 3<<20)
		reader = io.MultiReader(head, strings.NewReader("\n"), tail)
	}
	v := model.Session{ID: harness + ":" + strings.TrimSuffix(filepath.Base(path), ".jsonl"), Harness: harness, SourcePath: path, Status: "idle"}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		var row map[string]any
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			continue
		}
		kind := str(row, "type")
		payload, _ := row["payload"].(map[string]any)
		stamp, _ := time.Parse(time.RFC3339Nano, str(row, "timestamp"))
		if kind == "session_meta" {
			if id := str(payload, "id"); id != "" {
				v.ID = harness + ":" + id
			}
			v.Directory = str(payload, "cwd")
			v.StartedAt = stamp
		}
		if v.Directory == "" {
			v.Directory = str(row, "cwd")
		}
		var role, body string
		activity := false
		if harness == "codex" {
			if kind == "response_item" && str(payload, "type") == "message" {
				role = str(payload, "role")
				body = text(payload["content"])
			}
			if kind == "event_msg" {
				event := str(payload, "type")
				switch event {
				case "task_started", "turn_started":
					v.Status = "working"
					activity = true
				case "task_complete", "task_completed", "turn_complete", "turn_completed", "turn_aborted":
					v.Status = "idle"
					activity = true
				}
			}
		} else {
			if kind == "user" || kind == "assistant" {
				message, _ := row["message"].(map[string]any)
				role = str(message, "role")
				body = text(message["content"])
				activity = true
				if role == "user" {
					v.Status = "working"
				} else if str(message, "stop_reason") == "end_turn" {
					v.Status = "idle"
				}
			}
			if kind == "system" && str(row, "subtype") == "turn_duration" {
				v.Status = "idle"
				activity = true
			}
		}
		if role == "user" || role == "assistant" {
			activity = true
			v.Messages++
			body = clean(body)
			if role == "user" && v.Title == "" && body != "" {
				v.Title = clip(strings.Join(strings.Fields(body), " "), 160)
			}
			if role == "assistant" && body != "" && (harness == "claude" || str(payload, "phase") == "final_answer") {
				v.Summary = clip(body, 4000)
			}
		}
		if activity && !stamp.IsZero() {
			if v.StartedAt.IsZero() {
				v.StartedAt = stamp
			}
			if stamp.After(v.UpdatedAt) {
				v.UpdatedAt = stamp
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return v, fmt.Errorf("transcript contains an oversized record: %w", err)
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = info.ModTime()
	}
	if v.StartedAt.IsZero() {
		v.StartedAt = v.UpdatedAt
	}
	if !v.IsActive(now) {
		v.Status = "idle"
	}
	v.Project = filepath.Base(v.Directory)
	if v.Directory == "" || v.Project == "." {
		v.Project = "Other sessions"
	}
	return v, nil
}
