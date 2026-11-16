package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
)

func transcript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCodexTranscriptIgnoresInjectedInstructions(t *testing.T) {
	now := time.Now()
	stamp := now.Format(time.RFC3339Nano)
	body := fmt.Sprintf(`{"type":"session_meta","timestamp":%q,"payload":{"id":"abc","cwd":"/work/listodo"}}
{"type":"response_item","timestamp":%q,"payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>ignore this</environment_context>"}]}}
{"type":"event_msg","timestamp":%q,"payload":{"type":"task_started"}}
{"type":"response_item","timestamp":%q,"payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Build task persistence"}]}}
{"type":"response_item","timestamp":%q,"payload":{"type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"SQLite persistence is implemented and tested."}]}}
{"type":"event_msg","timestamp":%q,"payload":{"type":"task_complete"}}
{"partial":`, stamp, stamp, stamp, stamp, stamp, stamp)
	path := transcript(t, t.TempDir(), "rollout.jsonl", body)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseFile(path, "codex", now)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "codex:abc" || s.Project != "listodo" || s.Title != "Build task persistence" || s.Summary != "SQLite persistence is implemented and tested." || s.IsActive(now) {
		t.Fatalf("bad session: %+v", s)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("import modified the transcript")
	}
}

func TestClaudeActiveTurnAndStaleTurn(t *testing.T) {
	now := time.Now()
	body := fmt.Sprintf(`{"type":"user","timestamp":%q,"cwd":"/work/listodo","message":{"role":"user","content":"Fix login flow"}}
{"type":"assistant","timestamp":%q,"message":{"role":"assistant","stop_reason":"tool_use","content":[{"type":"text","text":"Working on the login flow"},{"type":"tool_use","name":"Bash","input":{"command":"secret command"}}]}}`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	path := transcript(t, t.TempDir(), "session.jsonl", body)
	s, err := ParseFile(path, "claude", now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Fix login flow" || s.Summary != "Working on the login flow" || !s.IsActive(now) {
		t.Fatalf("bad session: %+v", s)
	}
	s, err = ParseFile(path, "claude", now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if s.IsActive(now.Add(5 * time.Minute)) {
		t.Fatal("stale unfinished session shown as active")
	}
}

func TestScanReportsMissingSources(t *testing.T) {
	r := Scan(model.Settings{CodexEnabled: true, CodexRoot: filepath.Join(t.TempDir(), "missing")}, time.Now())
	if len(r.Warnings) != 1 || len(r.Sessions) != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestScannerImportsOnlyChangedFilesAndEvictsUnusedFingerprints(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	body := func(id, title string) string {
		return fmt.Sprintf("{\"type\":\"session_meta\",\"payload\":{\"id\":%q}}\n{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":%q}}\n", id, title)
	}
	first := transcript(t, dir, "one.jsonl", body("one", "First task"))
	second := transcript(t, dir, "two.jsonl", body("two", "Other task"))
	settings := model.Settings{CodexEnabled: true, CodexRoot: dir}
	var scanner Scanner
	if r := scanner.Scan(settings, now); r.Count != 2 || len(r.Sessions) != 2 {
		t.Fatalf("first scan: %+v", r)
	}
	if r := scanner.Scan(settings, now); r.Count != 2 || len(r.Sessions) != 0 || r.Files != 2 {
		t.Fatalf("unchanged scan: %+v", r)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	transcript(t, dir, "one.jsonl", body("one", "A longer task title"))
	// A size change must be detected even with unchanged modification time.
	if err := os.Chtimes(first, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if r := scanner.Scan(settings, now); r.Count != 2 || len(r.Sessions) != 1 || r.Sessions[0].Title != "A longer task title" {
		t.Fatalf("changed scan: %+v", r)
	}
	// A same-size edit must also be detected through its modification time.
	transcript(t, dir, "one.jsonl", body("one", "A newer task title!"))
	if err := os.Chtimes(first, now, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if r := scanner.Scan(settings, now); len(r.Sessions) != 1 || r.Sessions[0].Title != "A newer task title!" {
		t.Fatalf("same-size edit: %+v", r)
	}
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if r := scanner.Scan(settings, now); r.Count != 1 || len(scanner.files) != 1 {
		t.Fatalf("deleted file retained: %+v", r)
	}
	scanner.Reset()
	if r := scanner.Scan(settings, now); len(r.Sessions) != 1 {
		t.Fatal("reset must retry the import")
	}
	settings.CodexEnabled = false
	scanner.Scan(settings, now)
	if len(scanner.files) != 0 {
		t.Fatal("disabled source retained fingerprints")
	}
}

func TestScannerExpiresAnUnchangedActiveTurn(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	transcript(t, dir, "active.jsonl", fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"role":"user","content":"Keep working"}}`, now.Format(time.RFC3339Nano)))
	settings := model.Settings{ClaudeEnabled: true, ClaudeRoot: dir}
	var scanner Scanner
	first := scanner.Scan(settings, now)
	if len(first.Sessions) != 1 || !first.Sessions[0].IsActive(now) {
		t.Fatalf("missing active turn: %+v", first)
	}
	if r := scanner.Scan(settings, now.Add(2*time.Minute)); len(r.Sessions) != 0 {
		t.Fatal("active turn reparsed before expiry")
	}
	if r := scanner.Scan(settings, now.Add(3*time.Minute)); len(r.Sessions) != 1 || r.Sessions[0].Status != "idle" {
		t.Fatalf("unchanged active turn did not expire: %+v", r)
	}
	if r := scanner.Scan(settings, now.Add(4*time.Minute)); len(r.Sessions) != 0 {
		t.Fatal("idle turn reparsed")
	}
}

func TestScannerImportsDuplicateWhenTheWinningFileDisappears(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for i, title := range []string{"Original task", "Latest task"} {
		path := transcript(t, dir, fmt.Sprintf("%d.jsonl", i), fmt.Sprintf(`{"type":"session_meta","payload":{"id":"shared"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":%q}}`, title))
		modified := now.Add(time.Duration(i-2) * time.Minute)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	settings := model.Settings{CodexEnabled: true, CodexRoot: dir}
	var scanner Scanner
	if r := scanner.Scan(settings, now); r.Count != 1 || len(r.Sessions) != 1 || r.Sessions[0].Title != "Latest task" {
		t.Fatalf("wrong duplicate selected: %+v", r)
	}
	if r := scanner.Scan(settings, now); r.Count != 1 || len(r.Sessions) != 0 {
		t.Fatalf("unchanged duplicate imported: %+v", r)
	}
	if err := os.Remove(filepath.Join(dir, "1.jsonl")); err != nil {
		t.Fatal(err)
	}
	if r := scanner.Scan(settings, now); r.Count != 1 || len(r.Sessions) != 1 || r.Sessions[0].Title != "Original task" {
		t.Fatalf("replacement duplicate was not imported: %+v", r)
	}
}
