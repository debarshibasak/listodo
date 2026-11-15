# Listodo

![Listodo poster: monochrome typography beside a minimal task list, with the headline “Local tasks. Agent-powered work.”](assets/listodo-poster.png)

A local desktop task tracker built with **Go, SQLite, React, and shadcn/ui**. The macOS app uses the system’s native WKWebView, with no Electron or hosted backend. The interface uses Geist typography, Lucide icons, a monochrome palette, and a minimal task view.

## Run

Requires Go 1.23+, Node.js 20.19+, npm, and a C compiler (on macOS, install the Xcode Command Line Tools). The native interface requires macOS 13.3+ for its modern WebKit/CSS support. `make run` installs the frontend’s locked dependencies, builds its assets, and opens the native app.

```sh
make run
```

Build a macOS application you can open from Finder:

```sh
make package
open build/Listodo.app
```

The bundle is signed locally for development. Distribution signing and notarization are not configured. The native shell currently targets macOS; the same interface can run in a browser using `build/listodo --serve`. All assets, including fonts and the app icon, are bundled locally. The same monochrome logo is used in the sidebar, browser favicon, and macOS Dock/Finder. To regenerate the native icon from `frontend/public/favicon.svg` on macOS, run `node scripts/generate-icon.mjs` (requires the frontend dependencies and Playwright Chromium).

## What it does

- **Today:** open tasks from their start date onward, scheduled work due today, and tasks completed today.
- **Task capture:** one Markdown text box. Plain text creates a task; top-level `- [ ]` and `- [x]` lines create individual open or completed tasks. Additional lines are kept as Markdown notes. The creation day becomes the start date automatically and remains fixed across edits. New tasks have no implicit deadline; existing projects, priorities, dates, and session links are preserved.
- **Upcoming calendar:** click a date to add tasks with that due date, including Markdown checklist batches. Drag existing tasks from Unscheduled onto a day, move them between days to reschedule, or drop them back into Unscheduled to remove the date. Navigate months or return to Today, search and filter by status, and switch to List for future work. The task editor's Due date field also schedules, reschedules, and clears dates; changes to existing tasks save automatically.
- **Completed:** completed tasks, with search and status filters.
- **Automatic escalation:** every overdue calendar day raises effective priority one level, capped at Urgent. The original priority stays intact. Completed tasks do not escalate.
- **Run with agent:** choose **Codex** or **Claude Code** from a task’s **… → Run with agent** menu, choose a project folder, and start the task. The folder is saved on the task and remembered across edits and restarts. The task title and Markdown notes are sent to the CLI using your existing sign-in.
- **View operations:** lists only task runs launched by Listodo, with live output, provider, working folder, session ID, status, and a Stop action. Output follows new messages automatically; scroll up to pause following and return to the bottom to resume. Run history is saved locally. Tasks move to In progress when launched; you review the result and mark them complete. Quitting Listodo stops its active runs.
- **Run notifications:** finished and failed turns show a persistent in-app alert with **View run**. The packaged macOS app also sends a desktop notification when it is in the background; allow notifications when prompted on your first run. Clicking a desktop notification opens that operation. Historical runs and runs you explicitly stop do not generate completion alerts.
- **Manual ordering:** drag the handle beside a task to rearrange its status group, or focus the handle and press **Alt+↑ / Alt+↓**. Order survives edits and restarts; search-hidden tasks keep their positions. Only the task list scrolls, leaving the heading, filters, and capture field fixed.
- **Activity:** import local Codex and Claude Code transcripts, group them by repository/project, and attach sessions to tasks as evidence. Sessions live in the Activity page; there is no sidebar feed.
- **Organizer:** turn a description into proposed tasks and compare today's sessions with the existing list. Codex is the default; Claude is selectable. Review each task or suggested status before applying it.
- **History:** record task creation, updates, status changes, and deletion separately from session activity.
- **Persistence:** save every change in SQLite, including tasks, completion timestamps, session summaries, links, and integration settings. Export a JSON snapshot from the sidebar.

Press **⌘N** to create a task and **⌘K** or **⌘F** to search. Press Enter in the quick-add field to add a task. Paste multiple lines to open the Markdown editor, then press **⌘Enter** to save. Click its title to edit the title and notes in the same text box, or use its **…** menu. Completion checkboxes and Start/Pause buttons save immediately. Status tabs and the Display menu filter and sort the list. The sidebar can be collapsed, and narrower windows prioritize the task list.

Use the editor’s expand button to fill the window’s width and height without losing your draft; click it again to restore the compact view. Text scrolls inside the editor while the save controls stay at the bottom. Associated project folders appear above the footer. The flag menu sets a task’s priority to Low, Normal, High, or Urgent; when creating several tasks from a checklist, the selected priority applies to each one.

Edits to existing tasks save automatically after a short pause, including priority changes. The footer shows **Saving…**, **Saved**, or **Not saved**. You can keep typing while a save runs. Closing the editor or pressing **⌘Enter** saves the latest changes before closing; a save failure keeps your draft open for retry. New tasks and checklist batches are created with the save button or **⌘Enter**.

The task editor renders Markdown as editable headings, bold/italic/strikethrough text, links, images, nested lists, checklists, quotes, code blocks, tables, and horizontal rules. Paste Markdown to format it, or type shortcuts such as `## `, `**bold**`, and `- [ ] `. The **Markdown** button switches to the source. Tasks and agent prompts remain stored as Markdown.

Arrow keys and Shift-selection move through the whole document. **Tab / Shift+Tab** indent or outdent list items and move between table cells; Tab inserts spaces in code blocks. **⌘B / ⌘I** format text, **⌘Z / ⇧⌘Z** undo and redo, and **⇧⌘X** toggles the current checklist item. **⌘-click** a link to open it in your browser. Use Ctrl in place of ⌘ on Windows/Linux.

The frontend talks to Go through a loopback-only HTTP server on a random port. Each launch creates a fresh API token; every API request must supply it. The existing database format and data folder are unchanged.

## Running task agents

Task runs use the installed CLIs and the folder you select. Codex uses `exec --json` with the `workspace-write` sandbox and noninteractive approvals; Claude Code uses `-p --output-format stream-json --permission-mode acceptEdits`. Existing CLI permissions still apply, and permission denials appear in the operation output. No permission-bypass flags are enabled. Configure command permissions in the CLI if a task needs tools it cannot run.

Runs continue while you navigate between pages. Closing the app stops its processes; unfinished records from an interrupted app are marked Interrupted on the next launch. Failed runs can be started again from the task menu, creating a new history entry. Only one agent can run a given task at a time. Operation history retains the task prompt and the latest 128 KiB of output even if the task is later deleted.

When a run finishes or is stopped, a **`>`** prompt appears below its output. Type a follow-up and press **Enter** to continue the same Codex or Claude session, in the same folder, with the conversation context and output history preserved. Each completed turn can notify you again. This requires a recorded session ID; if the CLI exited before creating a session, start a new run from the task. Reopen completed tasks before continuing their sessions.

## Local data

On macOS the database lives at:

```text
~/Library/Application Support/Listodo/listodo.db
```

The directory uses the OS configuration directory on other platforms. Override it with `LISTODO_DATA_DIR` or `build/listodo --data-dir /your/folder`. SQLite uses WAL journaling, full synchronous writes, and transactions; the database file is private to your user. There is no telemetry.

To make a complete restorable backup, quit Listodo and copy the entire data folder, including any `listodo.db-wal` and `listodo.db-shm` files. Restore that folder while the app is closed. The JSON export is for inspection and portability; an import UI is not included.

## Harness integration

Install and sign in to either CLI in your terminal, then choose it in **Integrations**. Listodo uses the CLI's saved authentication and does not read credential files itself. A logged-in ChatGPT desktop app alone is not a Codex CLI installation.

- [Codex non-interactive execution](https://developers.openai.com/codex/noninteractive/): invoked using `codex exec`, a read-only sandbox, ephemeral execution, and a JSON output schema. User configuration/rules are disabled for organizer runs; authentication remains available. No model is hardcoded.
- [Claude Code programmatic execution](https://code.claude.com/docs/en/headless): invoked in print mode with structured output, restricted settings, no built-in tools, and no MCP servers. Uses the installed CLI's existing authentication. Recent CLI versions supporting these flags are required.

The app checks `PATH`, `~/.local/bin`, `/opt/homebrew/bin`, `/usr/local/bin`, and `~/.npm-global/bin` to find the CLI, including when launched from Finder. Missing authentication, timeouts, and malformed suggestions are shown as errors. Runs can be cancelled and time out after three minutes.

**Importing sessions is local and does not call a model.** Clicking **Prepare suggestions** sends task descriptions and up to 60 of today's imported session excerpts through the selected CLI to its provider. The call uses your existing plan or API billing. Suggestions never change tasks until you apply them.

Default transcript folders are `~/.codex/sessions` (or `$CODEX_HOME/sessions`) and `~/.claude/projects`. Both can be changed or disabled. Auto-sync runs once a minute while the app is open. It pauses while you edit integration settings.

The current adapters support **Codex and Claude Code**. Transcript formats are not public stable APIs, so parsing is best effort. Each scan reads the 400 most recently modified JSONL files from the past 14 days, bounded to the beginning and tail of large files. Previous imports remain in SQLite. Tool outputs and credential files are not included in organizer input. A session marked "Recently active" means a recent unfinished turn was observed; it does not confirm that a process is alive. Session completion is separate from task completion.

## Development

```sh
make test
make build
make test-ui
```

Go tests cover restart persistence, completion history, date-based priority, transcript parsing, CLI invocation, suggestion validation, API authorization, export, and task changes. Runner tests use fixture CLI executables to verify prompt delivery, working folders, streaming events, cancellation, and failure handling without making paid model calls. Browser tests exercise the actual shadcn interface: creation, start, completion, reopening, editing, deletion, search, filters, navigation, backup downloads, responsive layout, organizer review, agent launch/stop, operation output, persisted drag order, and fixed scrolling across tabs. Organizer tests use fixtures rather than calling a paid model.

The browser tests use a temporary database and write empty and populated screenshots to `build/preview-empty.png` and `build/preview.png`. Install Playwright’s Chromium once if it is not already cached:

```sh
cd frontend
npx playwright install chromium
npm test
```

For frontend development, start the Go server on a fixed local port, then start Vite in another terminal:

```sh
go run ./cmd/listodo --serve --listen 127.0.0.1:34115
npm --prefix frontend run dev
```

Vite proxies the API and injects the server’s per-launch token into the development page. Set `LISTODO_API_URL` if using another local port. Stop the development server before running browser tests, which reserve port 34115.

The entry point is `cmd/listodo`. The new frontend lives in `frontend/src`; shadcn components and their license are included. `internal/desktop` embeds the built assets, serves the API, and hosts the native macOS window. The existing Fyne interface remains in `internal/ui` as legacy source. Core services remain in `internal/model`, `internal/store`, `internal/sessions`, and `internal/harness`. Add another harness by implementing a transcript adapter in `internal/sessions` and, if needed, an organizer command in `internal/harness`.
