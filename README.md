# Canvas CLI for University Courses

English | [简体中文](README.zh-CN.md)

[![CI](https://github.com/hhe48203-ctrl/canvas-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/hhe48203-ctrl/canvas-cli/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/hhe48203-ctrl/canvas-cli)](https://github.com/hhe48203-ctrl/canvas-cli/blob/main/go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A command-line client for Canvas LMS in university and higher-education courses, built for students, instructors, teaching staff, and AI agents. It provides high-level course workflows, stable structured output, and a searchable catalog of Canvas REST API operations.

> [!IMPORTANT]
> This is an early-stage, independent community project for university course use. It is not developed, endorsed, or supported by Instructure. Before submitting an assignment, starting or completing a quiz, or performing another write operation, verify the target Canvas LMS instance, course, and parameters.

Canvas CLI targets academic workflows in university Canvas LMS courses and is designed for two kinds of users:

- students, instructors, and teaching staff who want short commands for courses, assignments, files, and quizzes;
- AI agents that need predictable JSON/YAML output, structured errors, explicit write confirmation, and access to less common Canvas endpoints.

The authoritative Canvas API documentation is available from the [Instructure Developer Documentation](https://developerdocs.instructure.com/services/canvas). Canvas CLI authenticates with an OAuth2 access token in the `Authorization: Bearer <token>` header.

## Highlights

- High-level commands for university courses, assignments, course files, and Classic Quizzes.
- More than 1,100 generated Canvas REST API operations that can be searched, described, and invoked.
- Automatic traversal of Canvas pagination through opaque `Link` headers.
- JSON, YAML, and terminal table output with stable success and error envelopes.
- Canvas' three-step file upload workflow, including authenticated completion redirects.
- Explicit `--confirm` required for write operations.
- Detailed help and copyable examples at every command level.

## Installation

Canvas CLI requires Go 1.26 or newer.

```bash
git clone https://github.com/hhe48203-ctrl/canvas-cli.git
cd canvas-cli
go build -o canvas .
sudo install -m 0755 canvas /usr/local/bin/canvas
```

To install without writing to a system directory:

```bash
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/canvas" .
canvas --help
```

During development, run the CLI directly:

```bash
go run . --help
```

## Configuration

Set the Canvas instance URL and access token:

```bash
export CANVAS_BASE_URL="https://school.instructure.com"
export CANVAS_API_TOKEN="your-access-token"
```

The base URL can also be saved locally:

```bash
canvas auth set-url https://school.instructure.com
```

The access token is read only from `CANVAS_API_TOKEN`; it is not written to the configuration file. Treat the token like a password: grant the minimum required permissions, never put it in command arguments, URLs, logs, or commits, and revoke it immediately if it is exposed.

Verify authentication:

```bash
canvas auth status
canvas me
```

## Common workflows

### Courses and assignments

```bash
canvas courses list
canvas courses list --all-pages
canvas courses list \
  --query enrollment_type=student \
  --query 'include[]=term'
canvas courses show COURSE_ID

canvas assignments list COURSE_ID --all-pages
canvas assignments show COURSE_ID ASSIGNMENT_ID
```

### Files

```bash
canvas files list COURSE_ID
canvas files download FILE_ID --destination ./lecture.pdf
canvas files upload COURSE_ID ./notes.pdf --confirm
```

### Assignment submissions

Every submission requires explicit confirmation:

```bash
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --file homework.pdf --confirm

canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --text '<p>Assignment content</p>' --confirm

canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --url https://example.com/report --confirm
```

File submissions first complete the Canvas upload workflow and then submit the returned file ID to the assignment. See the official [Submissions API](https://developerdocs.instructure.com/services/canvas/resources/submissions) and [File Upload documentation](https://developerdocs.instructure.com/services/canvas/basics/file.file_uploads).

### Classic Quizzes

```bash
canvas quizzes list COURSE_ID
canvas quizzes show COURSE_ID QUIZ_ID
canvas quizzes start COURSE_ID QUIZ_ID --confirm
canvas quizzes questions SUBMISSION_ID
canvas quizzes answer SUBMISSION_ID \
  --answers-file answers.json --confirm
canvas quizzes complete COURSE_ID QUIZ_ID SUBMISSION_ID \
  --attempt 1 --validation-token TOKEN --confirm
```

Example answer file:

```json
{
  "attempt": 1,
  "validation_token": "token-from-start-response",
  "quiz_questions": [
    {"id": 101, "answer": "A"},
    {"id": 102, "answer": ["choice-1", "choice-3"]}
  ]
}
```

These commands target Classic Quizzes. New Quizzes is a separate Canvas service with its own API lifecycle.

## Discovering and invoking the REST API

The bundled catalog covers the Canvas LMS REST endpoints present in the official documentation when the catalog was generated. Search the catalog, inspect an operation, and then invoke it:

```bash
canvas api search modules
canvas api describe context_modules_api.create
canvas api list --json
```

High-level aliases provide stable operation IDs for common calls:

```bash
canvas api describe courses.list
canvas api invoke courses.list \
  --query enrollment_type=student \
  --all-pages
```

Any endpoint can also be called with an explicit method and path:

```bash
canvas api invoke GET /api/v1/courses

canvas api invoke GET /api/v1/courses/{course_id} \
  --path course_id=123

canvas api invoke GET /api/v1/calendar_events \
  --query 'context_codes[]=course_123' \
  --query 'context_codes[]=course_456'
```

Canvas commonly accepts nested form fields for POST and PUT requests:

```bash
canvas api invoke POST /api/v1/courses/123/pages \
  --form 'wiki_page[title]=Overview' \
  --form 'wiki_page[body]=<p>Hello</p>' \
  --confirm
```

Use a raw body file or stdin for JSON and other encoded content:

```bash
canvas api invoke PUT /api/v1/courses/123 \
  --body request.json \
  --content-type application/json \
  --confirm

printf '%s' '{"query":"{ course(id: \"123\") { name } }"}' | \
  canvas api invoke POST /api/graphql \
  --body - --content-type application/json --confirm
```

Generic invocation flags:

- `--path name=value` replaces an operation path placeholder.
- `--query name=value` adds a query parameter and can be repeated for arrays.
- `--form name=value` adds a bracket-style Canvas form field.
- `--body FILE` sends a raw body; `--body -` reads stdin.
- `--header name=value` adds a request header.
- `--all-pages` follows Canvas `Link` headers and combines JSON-array pages.
- `--include-headers` includes the HTTP status, response headers, page count, and data.
- `--confirm` authorizes a POST, PUT, PATCH, or DELETE request.

## Help

Every command includes purpose, argument, flag, safety, and example information:

```bash
canvas --help
canvas api --help
canvas api invoke --help
canvas api search modules
canvas api describe context_modules_api.create
canvas assignments submit --help
canvas quizzes answer --help
canvas files download --help
```

In terminal mode, `api describe` shows parameter location, requirement status, type, default or allowed values, description, request-body content types, authentication scope, response metadata, and a runnable invocation skeleton. Use `--json` or `--yaml` for the same information in a structured form.

## Output

```bash
canvas courses list --output table
canvas courses list --json
canvas courses list --yaml
```

Behavior:

- an interactive terminal defaults to table output;
- a pipe or redirect defaults to JSON;
- `--json`, `--yaml`, or `--output` explicitly selects the format;
- successful results use an `{ "ok": true, "data": ... }` envelope;
- JSON/YAML errors use an `{ "ok": false, "error": ... }` envelope on stderr.

Example agent pipeline:

```bash
canvas courses list --all-pages --json | jq '.data[] | {id, name}'
```

## API catalog generation

The checked-in operation catalog is generated from Instructure's official online REST API documentation:

```bash
scripts/update-api-catalog.sh
go test ./...
```

The script converts `all_resources.html` to OpenAPI metadata and then generates `internal/api/generated.go`. If a Canvas source checkout has produced an official OpenAPI document with `bundle exec rake doc:openapi`, use it directly:

```bash
scripts/generate-openapi.sh path/to/canvas.openapi.yaml
go test ./...
```

The OpenAPI generator retains operation groups, descriptions, path/query/body parameters, required status, types, enums, defaults, content types, responses, and authentication scopes. It also resolves path-level parameters and component references.

“Complete” refers to the Canvas LMS REST API snapshot used to generate the catalog. Canvas Studio, the Data Access Platform, New Quizzes, and other separately versioned products use different base URLs, authentication rules, or lifecycles.

## Development

```bash
gofmt -w .
go test ./...
go vet ./...
go build -o canvas .
go run . api invoke --help
```

Tests use local `httptest` servers. They do not connect to a real Canvas instance, submit assignments, or complete quizzes.

Repository layout:

```text
.
├── cmd/                 # Cobra commands and high-level workflows
├── internal/api/        # Generated operation catalog and metadata
├── internal/canvas/     # HTTP, auth, pagination, downloads, and uploads
├── internal/config/     # Base URL and token configuration
├── internal/output/     # JSON, YAML, table, and envelope rendering
├── scripts/             # Catalog generation scripts
├── tools/               # Documentation and OpenAPI generators
└── .github/workflows/   # Continuous integration
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution and catalog-update workflow. Never include real access tokens, student information, course content, or institution-private data in commits, fixtures, issues, or logs.

## Scope and safety boundaries

- The project targets Canvas LMS deployments used for university and higher-education courses.
- API access remains limited by the authenticated user's Canvas role, token scopes, and course permissions.
- The CLI does not bypass access codes, IP restrictions, time limits, attempt limits, or other Canvas controls.
- Quiz commands only transmit answers explicitly supplied by the user or calling agent; they do not generate or guess answers.
- High-level commands cover common workflows; the generated catalog and raw invoker provide broader REST API reach.

## License

Canvas CLI is released under the [MIT License](LICENSE).

Canvas, Canvas LMS, and Instructure are trademarks of their respective owners. This project is not affiliated with or endorsed by Instructure.
