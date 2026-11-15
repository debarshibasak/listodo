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
