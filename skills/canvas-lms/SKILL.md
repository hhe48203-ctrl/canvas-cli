---
name: canvas-lms
description: Use Canvas LMS through the canvas CLI to inspect coursework and prepare or execute approved submissions without handling access tokens.
---

# Canvas LMS

Use `canvas` rather than browser automation for Canvas work. Return structured
output with `--json` (or `--yaml` when requested) and use IDs returned by Canvas
instead of guessing them.

## Safe workflow

- Run `canvas auth status` before the first request when credentials are expected
  to be configured.
- Read course, assignment, file, or quiz details before preparing an answer or
  submission. Quiz answers must come from the user; do not solve or guess them.
- Preview a write before sending it. Use `--dry-run` for `api invoke` and
  `assignments submit` when available; otherwise show the target and payload.
- Add `--confirm` only after the user explicitly approves the exact write.
- Never request, display, echo, or persist `CANVAS_API_TOKEN`. Let the CLI use
  the user's configured environment.

## Common reads

```bash
canvas courses list --all-pages --json
canvas assignments list COURSE_ID --all-pages --json
canvas assignments show COURSE_ID ASSIGNMENT_ID --json
canvas files list COURSE_ID --all-pages --json
```

## Submissions

Preview first, then repeat the same request with `--confirm` only after user
approval:

```bash
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --file answer.pdf --dry-run --json
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --file answer.pdf --confirm --json
```

## Other Canvas API endpoints

Search the bundled catalog, inspect the selected operation, then invoke it.
Use a dry run before a write and never include a token in a command:
Replace `COURSE_ID` with an ID returned by a previous Canvas response.

```bash
canvas api search modules --json
canvas api describe context_modules_api.index --json
canvas api invoke context_modules_api.index --path course_id=COURSE_ID --dry-run --json
```
