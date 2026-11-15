package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adaptive-scale/listodo/internal/model"
)

func TestRejectsUnknownTaskMatches(t *testing.T) {
	p := Proposal{Matches: []Match{{TaskID: "invented", SessionID: "s", SuggestedStatus: "done"}}}
	if Validate(p, []model.Task{{ID: "real"}}, []model.Session{{ID: "s"}}) == nil {
		t.Fatal("unknown task accepted")
	}
}

func TestCodexInvocationAndStructuredResponse(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	args := filepath.Join(dir, "args")
	input := filepath.Join(dir, "input")
	response := filepath.Join(dir, "response")
	if err := os.WriteFile(response, []byte(`{"summary":"One task for today.","tasks":[{"title":"Ship desktop app","notes":"Verify persistence","project":"Listodo","dueDate":"2026-11-13","priority":2}],"matches":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LISTODO_TEST_ARGS\"\n/bin/cat > \"$LISTODO_TEST_INPUT\"\n/bin/cat \"$LISTODO_TEST_RESPONSE\"\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("LISTODO_TEST_ARGS", args)
	t.Setenv("LISTODO_TEST_INPUT", input)
	t.Setenv("LISTODO_TEST_RESPONSE", response)
	p, err := Organize(context.Background(), "codex", "Build the app", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 1 || p.Tasks[0].Title != "Ship desktop app" {
		t.Fatalf("bad response: %+v", p)
	}
	argBytes, _ := os.ReadFile(args)
	for _, want := range []string{"--sandbox\nread-only", "--ignore-user-config", "--ephemeral", "--output-schema"} {
		if !strings.Contains(string(argBytes), want) {
			t.Fatalf("missing argument %s", want)
		}
	}
	prompt, _ := os.ReadFile(input)
	if !strings.Contains(string(prompt), "Build the app") {
		t.Fatal("user instructions not sent")
	}
}

func TestCancelledOrganizerStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Organize(ctx, "codex", "Test", nil, nil); err == nil {
		t.Fatal("cancelled run succeeded")
	}
}
