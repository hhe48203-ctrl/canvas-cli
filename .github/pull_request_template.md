## Outcome

<!-- What user-visible or maintenance outcome does this PR achieve? -->

Closes #

## Evidence

<!-- Include failing-before/passing-after evidence where practical. Mark every
item complete, including N/A with an explanation; unchecked items block agent
merge. -->

- [ ] Targeted test or reproduction
- [ ] `test -z "$(gofmt -l .)"`
- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `go test -race ./...`
- [ ] `go build -o /tmp/canvas-cli-verify .`

## Risk and decisions

<!-- Apply exactly one risk label to agent-authored PRs. -->

- Risk: low / medium / high
- Human decision required: no / yes — explain
- Compatibility or security impact: none / explain
- Generated files: none / explain source and generator

## Independent review

<!-- A read-only reviewer must inspect the final commit. Replace pending and the
placeholder SHA only after every consequential finding is resolved. -->

- Independent review: pending
- Reviewed head: `HEAD_SHA`
- Findings: pending

## Review notes

<!-- Point reviewers to the highest-risk lines and any proof that is still missing. -->
