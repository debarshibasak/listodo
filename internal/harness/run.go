package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type RunEvent struct {
	Text      string
	SessionID string
}

type Execution struct {
	cmd         *exec.Cmd
	stdout      *lineWriter
	stderr      *lineWriter
	mu          sync.Mutex
	completed   bool
	resultError string
}

// PrepareRun never executes a shell or puts task text into command arguments.
func PrepareRun(ctx context.Context, provider, directory, prompt string, emit func(RunEvent)) (*Execution, string, error) {
	return prepareRun(ctx, provider, directory, prompt, "", emit)
}

func PrepareContinuation(ctx context.Context, provider, directory, sessionID, prompt string, emit func(RunEvent)) (*Execution, string, error) {
	if sessionID == "" || len(sessionID) > 200 || strings.HasPrefix(sessionID, "-") || strings.ContainsAny(sessionID, "\x00\r\n") {
		return nil, "", errors.New("this run has no valid session to continue")
	}
	return prepareRun(ctx, provider, directory, prompt, sessionID, emit)
}

func prepareRun(ctx context.Context, provider, directory, prompt, sessionID string, emit func(RunEvent)) (*Execution, string, error) {
	bin, err := Find(provider)
	if err != nil {
		return nil, "", err
	}
	directory = strings.TrimSpace(directory)
	if directory == "~" || strings.HasPrefix(directory, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", err
		}
		directory = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(directory, "~"), "/"))
	}
	if !filepath.IsAbs(directory) {
		return nil, "", errors.New("choose an absolute project folder path")
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, "", fmt.Errorf("project folder: %w", err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() {
		return nil, "", errors.New("project location must be a folder")
	}
	var args []string
	if provider == "codex" {
		args = []string{"-a", "never", "exec", "--sandbox", "workspace-write", "--color", "never"}
		if sessionID != "" {
			args = append(args, "resume", "--skip-git-repo-check", "--json", sessionID, "-")
		} else {
			args = append(args, "--skip-git-repo-check", "--json", "-")
		}
	} else {
		args = []string{"-p", "--verbose", "--output-format", "stream-json", "--permission-mode", "acceptEdits"}
		if sessionID != "" {
			args = append(args, "--resume", sessionID)
		}
	}
	execution := &Execution{}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(bin)+":"+os.Getenv("PATH")+":/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin")
	cmd.WaitDelay = 2 * time.Second
	configureProcess(cmd)
	execution.cmd = cmd
	execution.stdout = &lineWriter{onLine: func(line string) {
		event, completed, failure := parseRunEvent(provider, line)
		if completed {
			execution.mu.Lock()
			execution.completed = true
			execution.resultError = failure
			execution.mu.Unlock()
		}
		if event.Text != "" || event.SessionID != "" {
			emit(event)
		}
	}}
	execution.stderr = &lineWriter{onLine: func(line string) { emit(RunEvent{Text: line}) }}
	cmd.Stdout = execution.stdout
	cmd.Stderr = execution.stderr
	return execution, directory, nil
}

func (e *Execution) Start() error { return e.cmd.Start() }
func (e *Execution) Wait() error {
	err := e.cmd.Wait()
	e.stdout.flush()
	e.stderr.flush()
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.resultError != "" {
		return errors.New(e.resultError)
	}
	if !e.completed {
		return errors.New("agent exited without a completed result; check the output and CLI sign-in")
	}
	return nil
}

// CLI records can be large. Bound each pending line while continuing to drain the pipe.
type lineWriter struct {
	buffer   []byte
	dropping bool
	onLine   func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
		part := p
		if end >= 0 {
			part = p[:end]
		}
		if !w.dropping {
			if len(w.buffer)+len(part) > 1<<20 {
				w.buffer = nil
				w.dropping = true
				w.onLine("[An oversized agent event was omitted.]")
			} else {
				w.buffer = append(w.buffer, part...)
			}
		}
		if end < 0 {
			break
		}
		if !w.dropping {
			w.flush()
		}
		w.dropping = false
		p = p[end+1:]
	}
	return n, nil
}
func (w *lineWriter) flush() {
	if len(w.buffer) > 0 {
		w.onLine(strings.TrimSpace(string(w.buffer)))
		w.buffer = nil
	}
}

func parseRunEvent(provider, line string) (RunEvent, bool, string) {
	var row struct {
		Type      string            `json:"type"`
		Subtype   string            `json:"subtype"`
		ThreadID  string            `json:"thread_id"`
		SessionID string            `json:"session_id"`
		Result    string            `json:"result"`
		IsError   bool              `json:"is_error"`
		Errors    []string          `json:"errors"`
		Denials   []json.RawMessage `json:"permission_denials"`
		Error     struct {
			Message string `json:"message"`
		} `json:"error"`
		Message json.RawMessage `json:"message"`
		Item    struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Command string `json:"command"`
			Output  string `json:"aggregated_output"`
			Changes []struct {
				Path string `json:"path"`
				Kind string `json:"kind"`
			} `json:"changes"`
		} `json:"item"`
	}
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		return RunEvent{Text: line}, false, ""
	}
	event := RunEvent{SessionID: row.SessionID}
	if provider == "codex" {
		event.SessionID = row.ThreadID
		switch row.Type {
		case "turn.completed":
			return event, true, ""
		case "turn.failed":
			failure := row.Error.Message
			if failure == "" {
				failure = "Codex could not finish this run."
			}
			event.Text = failure
			return event, true, failure
		case "error":
			_ = json.Unmarshal(row.Message, &event.Text)
		case "item.started":
			if row.Item.Command != "" {
				event.Text = "$ " + row.Item.Command
			}
		case "item.completed":
			event.Text = row.Item.Text
			if row.Item.Output != "" {
				event.Text = row.Item.Output
			}
			for _, change := range row.Item.Changes {
				event.Text += "\n" + change.Kind + " " + change.Path
			}
		}
	} else {
		switch row.Type {
		case "result":
			event.Text = row.Result
			if row.IsError {
				failure := strings.TrimSpace(row.Result + "\n" + strings.Join(row.Errors, "\n"))
				if failure == "" {
					failure = "Claude Code could not finish this run."
				}
				return event, true, failure
			}
			if len(row.Denials) > 0 {
				return event, true, "Some tools require permission. Review the output and your Claude Code permissions before retrying."
			}
			return event, true, ""
		case "assistant", "user":
			var message struct {
				Content []struct {
					Type    string          `json:"type"`
					Text    string          `json:"text"`
					Name    string          `json:"name"`
					Content json.RawMessage `json:"content"`
					Input   struct {
						Command  string `json:"command"`
						FilePath string `json:"file_path"`
					} `json:"input"`
				} `json:"content"`
			}
			if json.Unmarshal(row.Message, &message) == nil {
				var parts []string
				for _, block := range message.Content {
					if block.Text != "" {
						parts = append(parts, block.Text)
					}
					if block.Type == "tool_use" {
						parts = append(parts, strings.TrimSpace(block.Name+" "+block.Input.Command+" "+block.Input.FilePath))
					}
					if block.Type == "tool_result" {
						var content string
						if json.Unmarshal(block.Content, &content) == nil && content != "" {
							parts = append(parts, content)
						}
					}
				}
				event.Text = strings.Join(parts, "\n")
			}
		case "system":
			if row.Subtype == "permission_denied" {
				event.Text = "A tool requires permission; review your Claude Code permissions."
			}
		}
	}
	return event, false, ""
}
