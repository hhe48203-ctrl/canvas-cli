# Agentic development for Canvas CLI

This repository uses GitHub as the shared control plane between the maintainer,
Grok Bot, and Codex. The goal is not unattended code generation. The goal is to
automate evidence gathering, implementation, verification, review follow-up,
and low-risk merging while keeping product, architecture, compatibility, and
security decisions with the maintainer.

## Responsibilities

### Maintainer

- Decides product direction, public CLI/output contracts, architecture,
  credential/security boundaries, dependencies, and release policy.
- Approves an issue for implementation by resolving its decision notes and
  adding `agent:ready`.
- Reviews every medium/high-risk PR and any PR with `decision:human`.

### Grok Bot: outer loop

- Watches issues, PRs, CI, and release activity.
- Deduplicates and improves reports, asks for missing reproduction evidence,
  proposes risk labels, and marks only sufficiently specified issues ready.
- Independently reviews PR evidence and checks whether CI/review findings are
  current and legitimate.
- Stays quiet when nothing changed. It does not edit branches or merge unless
  the PR already satisfies the repository's explicit auto-merge policy.

### Codex: inner loop

- Claims one ready issue, works in an isolated branch/worktree, implements and
  tests it, self-reviews, and opens a draft PR.
- Uses subagents for bounded read-heavy exploration, test analysis, and
  adversarial review while one primary agent owns the edits.
- Guards the PR until it is merge-ready. It may merge only a policy-compliant
  low-risk PR carrying `agent:auto-merge`.

Project-scoped roles live in `.codex/agents/`: `repo_explorer` maps code and
history, `test_analyst` designs regression/protocol proof, and
`adversarial_reviewer` challenges the final diff. They are read-only; the
primary Codex agent is the sole writer and integrates their evidence.

## State machine

```text
new issue
  -> needs evidence / decision:human / duplicate / closed
  -> agent:ready
  -> agent:in-progress
  -> draft PR (agent:codex + one risk label)
  -> CI and independent review
  -> human review (medium/high or decision:human)
     OR agent:auto-merge (low risk only)
  -> merged -> issue closed -> retrospective if a gate escaped
```

An automation must process at most one implementation issue per run. A claimed
issue with an active branch or PR is never claimed again.

## Risk model

- `risk:low`: small, reversible maintenance or a well-bounded bug fix with a
  direct regression test and no protected path. Eligible for auto-merge after
  independent review.
- `risk:medium`: meaningful behavior, cross-package work, performance, or a
  change whose runtime evidence is indirect. Human PR review required.
- `risk:high`: credentials, confirmation, output compatibility, architecture,
  dependencies, releases, or another broad blast radius. Human decision before
  implementation and human merge.

`.agentic/policy.json` is the machine-readable source for labels, protected
paths, and required evidence. `scripts/agentctl.py` validates agent PRs and can
idempotently synchronize the label set:

```bash
python3 scripts/agentctl.py sync-labels \
  --policy .agentic/policy.json \
  --repo hhe48203-ctrl/canvas-cli
```

## Grok Bot instruction seed

Use this as the durable instruction for a Canvas CLI portfolio/triage bot:

> Treat GitHub as the source of truth for hhe48203-ctrl/canvas-cli. You own the
> outer loop: triage issues, deduplicate them, demand redacted reproduction and
> acceptance evidence, classify risk using AGENTS.md and .agentic/policy.json,
> and watch PR CI/reviews. Never request or expose Canvas tokens or real course,
> student, or submission data. Do not write code or push branches. Add
> agent:ready only when the scope is implementable and every product,
> architecture, compatibility, security, dependency, or release decision is
> already resolved; otherwise use decision:human and ask one focused question.
> Stay silent when state is unchanged. An agent:auto-merge recommendation is
> allowed only for a non-draft risk:low PR with no protected paths, green current
> checks, complete evidence, and no unresolved consequential review finding.

## Codex scheduled-task seed

Run this prompt against the saved local project in an isolated worktree:

> Use the repository's canvas-maintainer skill. Inspect open issues labeled
> agent:ready, exclude any issue already claimed or linked to an open PR, and
> select at most one highest-priority bounded issue. Claim it with
> agent:in-progress, implement it in the worktree, run the required verification,
> and open a draft PR using the template with agent:codex and exactly one risk
> label. Stop at a design proposal and apply decision:human if a required
> decision is not already recorded. Never push directly to main, weaken a test,
> expose Canvas data, or merge medium/high-risk work. If there is no actionable
> issue or the queue is unchanged, make no changes and report nothing.

Test the prompt manually before scheduling it. Start with one run per day; add a
separate PR-guardian schedule only after several implementation runs behave
predictably.

## Retrospectives

When an agent creates avoidable rework, capture the mechanism, not a vague
warning. Prefer, in order: a regression test, an `agentctl`/CI rule, a generator
invariant, a focused skill edit, then an `AGENTS.md` rule. Remove obsolete rules
when code or automation makes them redundant.
