package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTaskExecutionUsesStdinDirectoryAndPermissionModes(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			binDir, workspace := t.TempDir(), t.TempDir()
			response := `{"type":"thread.started","thread_id":"codex-session"}
{"type":"item.completed","item":{"type":"agent_message","text":"Updated the file."}}
{"type":"turn.completed"}`
			if provider == "claude" {
				response = `{"type":"system","subtype":"init","session_id":"claude-session"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Updated the file."}]}}
{"type":"result","is_error":false,"result":"Done","session_id":"claude-session"}`
			}
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > args\n/bin/cat > prompt\nprintf '%s' '" + response + "'\n"
			if err := os.WriteFile(filepath.Join(binDir, provider), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir)
			prompt := "Task `touch unexpected` $(touch unexpected) 'quoted'\n\nMarkdown details"
			var events []RunEvent
			run, dir, err := PrepareRun(context.Background(), provider, workspace, prompt, func(event RunEvent) { events = append(events, event) })
			if err != nil {
				t.Fatal(err)
			}
			resolved, _ := filepath.EvalSymlinks(workspace)
			if dir != resolved {
				t.Fatal("wrong working folder")
			}
			if err := run.Start(); err != nil {
				t.Fatal(err)
			}
			if err := run.Wait(); err != nil {
				t.Fatal(err)
			}
			captured, _ := os.ReadFile(filepath.Join(workspace, "prompt"))
			if string(captured) != prompt {
				t.Fatal("task prompt changed")
			}
			if _, err := os.Stat(filepath.Join(workspace, "unexpected")); !os.IsNotExist(err) {
				t.Fatal("task text was executed as shell")
			}
			args, _ := os.ReadFile(filepath.Join(workspace, "args"))
			expected := "--sandbox\nworkspace-write"
			if provider == "claude" {
				expected = "--permission-mode\nacceptEdits"
			}
			if !strings.Contains(string(args), expected) || strings.Contains(string(args), "bypass") {
				t.Fatalf("wrong permission mode: %s", args)
			}
			var session string
			for _, event := range events {
				if event.SessionID != "" {
					session = event.SessionID
				}
			}
			if session != provider+"-session" {
				t.Fatalf("session missing: %+v", events)
			}
		})
	}
}

func TestAgentErrorsAndFragmentedOutput(t *testing.T) {
	for _, tc := range []struct{ provider, line string }{
		{"codex", `{"type":"turn.failed","error":{"message":"Sign in first"}}`},
		{"claude", `{"type":"result","is_error":true,"result":"Sign in first"}`},
		{"claude", `{"type":"result","is_error":false,"permission_denials":[{}]}`},
	} {
		_, completed, failure := parseRunEvent(tc.provider, tc.line)
		if !completed || failure == "" {
			t.Fatalf("agent failure was lost: %s", tc.line)
		}
	}
	var lines []string
	w := lineWriter{onLine: func(line string) { lines = append(lines, line) }}
	w.Write([]byte("fir"))
	w.Write([]byte("st\nsecond\nlast"))
	w.flush()
	if strings.Join(lines, "|") != "first|second|last" {
		t.Fatal(lines)
	}
	w.Write([]byte(strings.Repeat("x", (1<<20)+1)))
	w.Write([]byte("\nafter oversized event\n"))
	if lines[len(lines)-1] != "after oversized event" {
		t.Fatal("stream did not recover from a large event")
	}
}

func TestContinuationUsesSameSessionAndPermissionModes(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			binDir, workspace := t.TempDir(), t.TempDir()
			response := `{"type":"turn.completed"}`
			if provider == "claude" {
				response = `{"type":"result","is_error":false,"result":"Done"}`
			}
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > args\n/bin/cat > prompt\nprintf '%s' '" + response + "'\n"
			if err := os.WriteFile(filepath.Join(binDir, provider), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir)
			prompt := "Now test the changes. $(touch unexpected) `touch unexpected`"
			run, _, err := PrepareContinuation(context.Background(), provider, workspace, "saved-session", prompt, func(RunEvent) {})
			if err != nil {
				t.Fatal(err)
			}
			if err := run.Start(); err != nil {
				t.Fatal(err)
			}
			if err := run.Wait(); err != nil {
				t.Fatal(err)
			}
			args, _ := os.ReadFile(filepath.Join(workspace, "args"))
			captured, _ := os.ReadFile(filepath.Join(workspace, "prompt"))
			resume, permissions := "resume\n--skip-git-repo-check\n--json\nsaved-session\n-", "--sandbox\nworkspace-write"
			if provider == "claude" {
				resume, permissions = "--resume\nsaved-session", "--permission-mode\nacceptEdits"
			}
			if !strings.Contains(string(args), resume) || !strings.Contains(string(args), permissions) || strings.Contains(string(args), "bypass") {
				t.Fatalf("wrong continuation arguments: %s", args)
			}
			if string(captured) != prompt || strings.Contains(string(args), prompt) {
				t.Fatal("follow-up prompt must be delivered unchanged through stdin")
			}
			if _, err := os.Stat(filepath.Join(workspace, "unexpected")); !os.IsNotExist(err) {
				t.Fatal("prompt was interpreted as shell code")
			}
		})
	}
}

func TestCancelAgentStopsItsProcessGroup(t *testing.T) {
	binDir, workspace := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\n/bin/sleep 30 &\nprintf '%s' \"$!\" > child\nwait\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run, _, err := PrepareRun(ctx, "codex", workspace, "test", func(RunEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(filepath.Join(workspace, "child")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	cancel()
	if err := run.Wait(); err == nil {
		t.Fatal("cancelled agent succeeded")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("agent did not stop promptly")
	}
}
