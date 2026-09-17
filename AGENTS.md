# Canvas CLI agent guidance

## Mission

Maintain Canvas CLI as a predictable, scriptable boundary between Canvas LMS and
people or agents. Prefer stable structured behavior, explicit write approval,
and compatibility over convenience that makes automation ambiguous.

## Repository map

- `cmd/`: Cobra commands and CLI-facing behavior.
- `internal/canvas/`: HTTP transport, pagination, uploads, and Canvas boundary.
- `internal/output/`: stable JSON/YAML success and error envelopes.
- `internal/config/`: local configuration and credential-adjacent behavior.
- `internal/api/registry.go`: generated-catalog lookup behavior.
- `internal/api/generated.go`: generated output; change its generator or source,
  not this file by hand.
- `tools/`: API catalog generators and their fixtures.
- `skills/canvas-lms/`: end-user skill for operating the installed CLI.
- `.agents/skills/canvas-maintainer/`: repository maintenance workflow.
- `.codex/agents/`: read-only exploration, test-design, and adversarial-review
  subagent roles; the primary agent remains the only writer.

`progress.md` is a historical local work log. Do not update it unless the user
explicitly asks for that file to be maintained.

## Non-negotiable behavior

- Never print, persist, request, or commit `CANVAS_API_TOKEN` or real course,
  student, submission, or institution data.
- Keep mutating Canvas operations behind an exact preview and explicit
  `--confirm`. A new mutation must not bypass this boundary.
- Preserve stable JSON/YAML envelopes and machine-readable failures. Human text
  may improve, but automation must not need to scrape it.
- Treat Canvas IDs as lossless identifiers. Do not narrow them through floating
  point or platform-sized integer conversions.
- Keep bearer credentials on the configured origin across redirects.
- Tests must use local fixtures or `httptest`; they must not contact a real
  Canvas instance.
- Catalog changes are generated. Update the relevant generator and prove the
  fixture pipeline before accepting generated churn.

## Working method

1. Start from an issue or write a short outcome and acceptance criteria before
   editing. Read `.agentic/policy.json` for risk and queue rules.
2. Work on a branch or isolated worktree. Do not push directly to `main`.
3. For non-trivial work, delegate exploration to `repo_explorer`, test analysis
   to `test_analyst`, and final challenge review to `adversarial_reviewer`. One
   primary agent owns the final edits in a worktree; do not let parallel writers
   race on the same files.
4. Reproduce bugs before changing production code. Add a focused failing test
   first when the repository already has a cheap test seam.
5. Make the smallest coherent change that solves the root cause. Do not preserve
   a legacy path unless compatibility is intentional and tested.
6. Run the relevant fast checks while iterating and the full gate before a PR.
7. Review the complete diff against the issue, then open a draft PR with the
   repository template and correct risk labels.

## Human decision checkpoints

Add `decision:human` and stop at a reviewable design or draft PR when a change
affects any of the following, unless the issue already records the decision and
has `agent:ready`:

- command names, flags, output schemas, or compatibility promises;
- architecture or ownership across packages;
- authentication, credential handling, redirects, confirmation, or privacy;
- a new runtime dependency or external service;
- release/distribution policy or support matrix;
- a user-visible feature rather than implementation of already-approved scope.

Agents may fully implement an already-approved decision. They must not silently
make a new product or architecture decision merely to unblock themselves.

## Verification

Fast, targeted checks are encouraged during implementation. Before declaring a
change ready, run from the repository root:

```bash
test -z "$(gofmt -l .)"
go test ./...
go vet ./...
go test -race ./...
go build -o /tmp/canvas-cli-verify .
```

For catalog changes, also run the relevant generator fixture tests and inspect
the generated diff. For CLI behavior, run the affected help or command path and
check both human and structured output when applicable.

## Pull requests and autonomy

- `agent:ready` means the issue is sufficiently specified for implementation.
- An agent-authored PR carries `agent:codex` and exactly one risk label.
- `agent:auto-merge` is allowed only with `risk:low`, no `decision:human`, no
  protected paths from `.agentic/policy.json`, green required checks, and an
  independent review with no unresolved consequential finding.
- Medium/high-risk changes remain draft or review-required even when tests pass.
- A failed or flaky gate is work to investigate, not a reason to weaken the
  check. Document a genuine infrastructure flake before retrying it.

## Code review rules

- Flag any mutation that can execute without the established preview/confirm
  contract.
- Flag output that is no longer a single parseable JSON/YAML envelope.
- Flag token/header forwarding across origins or secrets entering arguments,
  logs, fixtures, errors, or generated files.
- Flag hand edits to `internal/api/generated.go` without the generating source
  and verification.
- Flag retries of non-idempotent operations or unbounded reads/uploads.
- Require observable regression coverage for behavior changes; do not accept a
  test that only mirrors implementation details.
