# Canvas CLI — Canvas LMS for AI Agents

English | [简体中文](README.zh-CN.md)

[![CI](https://github.com/hhe48203-ctrl/canvas-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/hhe48203-ctrl/canvas-cli/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/hhe48203-ctrl/canvas-cli)](https://github.com/hhe48203-ctrl/canvas-cli/blob/main/go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Canvas CLI gives AI agents a controlled interface to Canvas LMS. Agents can
inspect courses and assignments, download course files, prepare submissions,
work through Classic Quizzes with user-provided answers, and access more than
1,100 Canvas REST API operations.

Commands return structured JSON or YAML. Read operations run directly; writes
such as uploads and submissions require `--confirm`, so an agent can show the
exact action before executing it.

## Install

Give the repository URL to an agent:

```text
Install Canvas CLI from https://github.com/hhe48203-ctrl/canvas-cli.
Use the Go version declared by the repository, install the canvas binary in a
user-writable directory on PATH, and verify it with canvas --help. Do not use
sudo or change my shell configuration without asking. I will set the Canvas
token myself; do not ask me to paste it into chat.
```

Or install it manually:

```bash
git clone https://github.com/hhe48203-ctrl/canvas-cli.git
cd canvas-cli
go build -o canvas .
mkdir -p "$HOME/.local/bin"
install -m 0755 canvas "$HOME/.local/bin/canvas"
```

Canvas CLI requires the Go version declared in [`go.mod`](go.mod).

## Configure

Create a Canvas access token in **Account → Settings → Approved Integrations**,
then keep it in an environment variable:

```bash
export CANVAS_BASE_URL="https://canvas.example.edu"
export CANVAS_API_TOKEN="your-token"

canvas auth status
canvas auth set-url "https://canvas.example.edu"
```

`CANVAS_API_TOKEN` is never written by the CLI. Do not put it in prompts, source
files, command arguments, or commits.

## Agent Skill

Add a `SKILL.md` like the following to teach an agent the safe workflow:

```markdown
---
name: canvas-lms
description: Use Canvas LMS through the canvas CLI for courses, assignments, files, submissions, and quizzes.
---

# Canvas LMS

Use `canvas` instead of browser automation when working with Canvas.

Rules:
- Run `canvas auth status` before the first request.
- Prefer `--json`; use returned IDs instead of guessing them.
- Read assignment or quiz details before preparing an answer.
- Show the target and payload before any write.
- Add `--confirm` only after the user explicitly approves the write.
- Never request or display `CANVAS_API_TOKEN`.

Useful reads:
    canvas courses list --all-pages --json
    canvas assignments list COURSE_ID --all-pages --json
    canvas assignments show COURSE_ID ASSIGNMENT_ID --json
    canvas files list COURSE_ID --all-pages --json

Submission after approval:
    canvas assignments submit COURSE_ID ASSIGNMENT_ID \
      --file answer.pdf --confirm --json

For other endpoints:
    canvas api search KEYWORD
    canvas api describe OPERATION_ID
    canvas api invoke OPERATION_ID --json
```

Example request:

> Find my active biology course, list assignments due this week, and prepare the
> next submission. Show me the course, assignment, and files before submitting.

## Common commands

```bash
# Courses and assignments
canvas courses list --all-pages --json
canvas assignments list COURSE_ID --all-pages --json
canvas assignments show COURSE_ID ASSIGNMENT_ID --json

# Files
canvas files list COURSE_ID --all-pages --json
canvas files download FILE_ID --destination ./lecture.pdf

# Text, URL, or multi-file submissions
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --text "My response" --confirm --json
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --url "https://example.com/work" --confirm --json
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --file answer.pdf --file appendix.pdf --confirm --json

# Classic Quizzes
canvas quizzes list COURSE_ID --all-pages --json
canvas quizzes start COURSE_ID QUIZ_ID --confirm --json
canvas quizzes questions SUBMISSION_ID --all-pages --json
canvas quizzes answer SUBMISSION_ID \
  --answers-file answers.json --confirm --json
canvas quizzes complete COURSE_ID QUIZ_ID SUBMISSION_ID \
  --attempt ATTEMPT --validation-token TOKEN --confirm --json
```

Use `canvas --help` or `canvas <command> --help` for all flags and examples.

## Full Canvas API

The generated catalog covers more than 1,100 operations from the official
Canvas REST API documentation:

```bash
canvas api search modules
canvas api describe context_modules_api.create
canvas api invoke courses.list \
  --query enrollment_type=student --all-pages --json
canvas api invoke METHOD /api/v1/example \
  --query key=value --body request.json --confirm --json
```

`api describe` shows the method, path, and parameters. `api invoke` accepts
repeatable `--path`, `--query`, `--header`, and `--form` values, plus `--body`
and `--all-pages`; write methods require `--confirm`.

The authoritative API reference is the
[Instructure Developer Documentation](https://developerdocs.instructure.com/services/canvas).

## Output and safety

- `--json` and `--yaml` return stable success or error envelopes.
- `--all-pages` follows Canvas pagination links without reconstructing them.
- File uploads stream data and complete Canvas' multi-step upload flow.
- Mutating operations require `--confirm`.
- Quiz answers must come from the user; the CLI does not solve or guess them.
- Canvas permissions and institutional policies still apply.

## Local usage logs

Usage logging is **on by default** and stays on your computer; nothing is
uploaded. Disable it in the agent's environment with:

```bash
export CANVAS_USAGE_LOG=0
```

Unset the variable to re-enable it. Each completed CLI invocation appends one
JSONL record, including argument and execution failures. Pagination and file
transfers produce one command summary. Help and completion have their own
`kind` so they can be excluded from usage statistics.

| Fields | Meaning |
| --- | --- |
| `time`, `version` | UTC completion time; module version, Git revision, or `devel` when unavailable |
| `kind`, `command` | `command`, `help`, or `completion`; registered command path |
| `operation_id` | Recognized catalog ID for `api invoke` / `api describe`, when available |
| `duration_ms`, `exit_code` | Command duration and exit code (0 or 1) |
| `error_kind` | On failure: `arguments`, `confirmation_required`, `configuration`, `http`, `network`, `io`, or `execution` |
| `http_status` | Last received HTTP response status, when available; a later network/local failure may still leave this status present |

Only these fields are stored. Logs exclude argument values, raw URLs, tokens,
file paths, request/response bodies, and original error messages. Unknown
operation IDs are omitted. Logging failures do not change stdout, stderr, or
the command's exit code. Forced termination may leave no record, and command
success does not establish that an agent completed the user's task.

Daily files are named `YYYY-MM-DD.jsonl` in the system user cache directory:

- macOS: `~/Library/Caches/canvas-cli/logs`
- Linux: `${XDG_CACHE_HOME:-$HOME/.cache}/canvas-cli/logs`
- Windows: `%LocalAppData%\canvas-cli\logs`

Directories use mode `0700` and files `0600` on Unix. Each logged invocation
cleans up files older than the current UTC day and six preceding days. Each
daily file has a 10 MiB soft limit: new records are skipped after the limit is
reached, with small overshoots possible from concurrent writes. Logging resumes
in a new file the next UTC day. Disabling logging does not delete existing logs.

After running commands, use `jq` to summarize call counts, failure rates (0–1),
and average duration, excluding help and completion:

```bash
case "$(uname -s)" in
  Darwin) canvas_log_dir="$HOME/Library/Caches/canvas-cli/logs" ;;
  *) canvas_log_dir="${XDG_CACHE_HOME:-$HOME/.cache}/canvas-cli/logs" ;;
esac

jq -s '
  map(select(.kind == "command"))
  | group_by([.command, .operation_id])
  | map({
      command: .[0].command,
      operation_id: .[0].operation_id,
      calls: length,
      failure_rate: ((map(select(.exit_code != 0)) | length) / length),
      avg_duration_ms: ((map(.duration_ms) | add) / length)
    })
  | sort_by(-.calls)
' "$canvas_log_dir"/*.jsonl
```

## Development

```bash
go test ./...
go build ./...
scripts/update-api-catalog.sh
```

The script refreshes the embedded catalog from Instructure's official API
documentation. See [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution
workflow.

## License

[MIT](LICENSE)
