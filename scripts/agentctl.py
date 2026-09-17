#!/usr/bin/env python3
"""Validate, gate, and synchronize the repository's agent workflow policy."""

from __future__ import annotations

import argparse
import fnmatch
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from typing import Iterable


def load_json(path: str | Path) -> dict:
    with Path(path).open(encoding="utf-8") as handle:
        value = json.load(handle)
    if not isinstance(value, dict):
        raise ValueError(f"Expected a JSON object in {path}")
    return value


def validate_policy(policy: dict) -> None:
    if policy.get("schema_version") != 1:
        raise ValueError("Unsupported agent policy schema_version")
    pull_requests = policy.get("pull_requests")
    auto_merge = policy.get("auto_merge")
    queue = policy.get("queue")
    labels = policy.get("labels")
    if not all(isinstance(value, dict) for value in (pull_requests, auto_merge, queue)):
        raise ValueError("Policy must define queue, pull_requests, and auto_merge objects")
    if not isinstance(labels, list) or not labels:
        raise ValueError("Policy must define at least one label")
    names = [entry.get("name") for entry in labels if isinstance(entry, dict)]
    if len(names) != len(labels) or any(not name for name in names):
        raise ValueError("Every policy label needs a non-empty name")
    if len(set(names)) != len(names):
        raise ValueError("Policy label names must be unique")
    allowed_risks = auto_merge.get("allowed_risk_labels")
    if (
        not isinstance(allowed_risks, list)
        or not allowed_risks
        or any(not isinstance(name, str) or not name for name in allowed_risks)
        or len(set(allowed_risks)) != len(allowed_risks)
    ):
        raise ValueError("auto_merge.allowed_risk_labels must be a non-empty unique string list")
    required_checks = auto_merge.get("required_checks")
    if (
        not isinstance(required_checks, list)
        or not required_checks
        or any(not isinstance(name, str) or not name for name in required_checks)
        or len(set(required_checks)) != len(required_checks)
    ):
        raise ValueError("auto_merge.required_checks must be a non-empty unique string list")
    required_evidence = auto_merge.get("required_evidence_items")
    if (
        not isinstance(required_evidence, list)
        or not required_evidence
        or any(not isinstance(item, str) or not item for item in required_evidence)
        or len(set(required_evidence)) != len(required_evidence)
    ):
        raise ValueError(
            "auto_merge.required_evidence_items must be a non-empty unique string list"
        )
    runtime_paths = auto_merge.get("runtime_evidence_paths")
    if not isinstance(runtime_paths, list) or any(
        not isinstance(pattern, str) or not pattern for pattern in runtime_paths
    ):
        raise ValueError("auto_merge.runtime_evidence_paths must be a string list")
    referenced = {
        queue.get("ready_label"),
        queue.get("claimed_label"),
        queue.get("blocked_label"),
        pull_requests.get("agent_label"),
        pull_requests.get("auto_merge_label"),
        pull_requests.get("human_decision_label"),
        *allowed_risks,
        *pull_requests.get("risk_labels", []),
    }
    missing = sorted(name for name in referenced if name and name not in names)
    if missing:
        raise ValueError(f"Policy references labels that are not declared: {', '.join(missing)}")


def label_names(event: dict) -> set[str]:
    pull_request = event.get("pull_request") or {}
    return {
        label["name"]
        for label in pull_request.get("labels", [])
        if isinstance(label, dict) and isinstance(label.get("name"), str)
    }


def missing_headings(body: str, headings: Iterable[str]) -> list[str]:
    lines = {line.strip() for line in body.splitlines()}
    return [heading for heading in headings if heading not in lines]


def protected_path_matches(paths: Iterable[str], patterns: Iterable[str]) -> list[str]:
    matches = []
    for path in paths:
        if any(fnmatch.fnmatchcase(path, pattern) for pattern in patterns):
            matches.append(path)
    return sorted(set(matches))


def validate_pull_request(policy: dict, event: dict, paths: Iterable[str]) -> list[str]:
    validate_policy(policy)
    pull_request = event.get("pull_request")
    if not isinstance(pull_request, dict):
        return ["Event does not contain a pull_request object"]

    settings = policy["pull_requests"]
    labels = label_names(event)
    agent_label = settings["agent_label"]
    auto_label = settings["auto_merge_label"]
    human_label = settings["human_decision_label"]
    agent_authored = agent_label in labels
    auto_merge = auto_label in labels

    if not agent_authored and not auto_merge:
        return []

    errors: list[str] = []
    selected_risks = sorted(labels.intersection(settings["risk_labels"]))
    if agent_authored:
        if len(selected_risks) != 1:
            errors.append(
                "Agent-authored PRs need exactly one risk label; found "
                + (", ".join(selected_risks) if selected_risks else "none")
            )
        missing = missing_headings(
            pull_request.get("body") or "", settings["required_body_headings"]
        )
        if missing:
            errors.append("PR body is missing required headings: " + ", ".join(missing))

    if auto_merge:
        if not agent_authored:
            errors.append(f"{auto_label} requires {agent_label}")
        allowed_risks = policy["auto_merge"]["allowed_risk_labels"]
        if len(selected_risks) != 1 or selected_risks[0] not in allowed_risks:
            errors.append(
                f"{auto_label} requires exactly one allowed risk label: "
                + ", ".join(allowed_risks)
            )
        if human_label in labels:
            errors.append(f"{auto_label} cannot be combined with {human_label}")
        if pull_request.get("draft"):
            errors.append(f"{auto_label} cannot be used on a draft PR")
        protected = protected_path_matches(paths, policy["auto_merge"]["protected_paths"])
        if protected:
            errors.append("Automatic merge is blocked by protected paths: " + ", ".join(protected))

    return errors


def check_identifier(check: dict) -> str:
    explicit = check.get("identifier")
    if isinstance(explicit, str):
        return explicit
    name = check.get("name") or check.get("context") or ""
    workflow = check.get("workflowName") or ""
    return f"{workflow} / {name}" if workflow and name else name


def latest_checks(checks: Iterable[dict]) -> dict[str, dict]:
    latest: dict[str, dict] = {}
    for check in checks:
        if not isinstance(check, dict):
            continue
        identifier = check_identifier(check)
        if not identifier:
            continue
        check_id = check.get("id")
        timestamp = check.get("startedAt") or check.get("completedAt") or ""
        order_key = (2, check_id) if isinstance(check_id, int) else (1, timestamp)
        if not check_id and not timestamp:
            order_key = (0, 0)
        current = latest.get(identifier)
        current_id = current.get("id") if current else None
        current_timestamp = (
            current.get("startedAt") or current.get("completedAt") or ""
            if current
            else ""
        )
        current_key = (
            (2, current_id)
            if isinstance(current_id, int)
            else ((1, current_timestamp) if current_timestamp else (0, 0))
        )
        if current is None or order_key > current_key:
            latest[identifier] = check
        elif order_key == current_key == (0, 0):
            latest[identifier] = {"identifier": identifier, "orderingUnknown": True}
    return latest


def check_succeeded(check: dict) -> bool:
    if check.get("orderingUnknown"):
        return False
    if check.get("__typename") == "StatusContext":
        return check.get("state") == "SUCCESS"
    return check.get("status") == "COMPLETED" and check.get("conclusion") == "SUCCESS"


def labels_from_item(item: dict) -> set[str]:
    return {
        label["name"]
        for label in item.get("labels", [])
        if isinstance(label, dict) and isinstance(label.get("name"), str)
    }


def validate_merge_gate(
    policy: dict, pull_request: dict, review_threads: dict, expected_head: str
) -> list[str]:
    """Return every reason the current remote PR head must not be merged."""
    validate_policy(policy)
    files = [
        item["path"]
        for item in pull_request.get("files", [])
        if isinstance(item, dict) and isinstance(item.get("path"), str)
    ]
    event = {
        "pull_request": {
            "body": pull_request.get("body") or "",
            "draft": bool(pull_request.get("isDraft")),
            "labels": pull_request.get("labels", []),
        }
    }
    errors = validate_pull_request(policy, event, files)
    current_labels = labels_from_item(pull_request)
    for required_label in (
        policy["pull_requests"]["agent_label"],
        policy["pull_requests"]["auto_merge_label"],
    ):
        if required_label not in current_labels:
            errors.append(f"Merge gate requires {required_label}")
    if policy["queue"]["blocked_label"] in current_labels:
        errors.append(f"Merge gate rejects {policy['queue']['blocked_label']}")

    if pull_request.get("state") != "OPEN":
        errors.append("Pull request is not open")
    if pull_request.get("baseRefName") != policy.get("default_branch"):
        errors.append(f"Pull request must target {policy.get('default_branch')}")
    if pull_request.get("headRefOid") != expected_head:
        errors.append("Pull request head does not match --expected-head")
    if pull_request.get("mergeable") != "MERGEABLE":
        errors.append("Pull request is not currently mergeable")
    if pull_request.get("mergeStateStatus") != "CLEAN":
        errors.append("Pull request merge state is not CLEAN")
    if pull_request.get("reviewDecision") == "CHANGES_REQUESTED" or any(
        review.get("state") == "CHANGES_REQUESTED"
        for review in pull_request.get("latestReviews", [])
        if isinstance(review, dict)
    ):
        errors.append("A review is requesting changes")
    if pull_request.get("reviewRequests"):
        errors.append("A requested reviewer has not completed review")

    body = event["pull_request"]["body"]
    if re.search(r"(?im)^\s*[-*+]\s+\[\s\]\s+", body):
        errors.append("PR body contains unresolved evidence checkboxes")
    checked_items = [
        match.group(1).strip()
        for match in re.finditer(r"(?im)^\s*[-*+]\s+\[[x]\]\s+(.+)$", body)
    ]
    missing_evidence = [
        required
        for required in policy["auto_merge"]["required_evidence_items"]
        if not any(item.startswith(required) for item in checked_items)
    ]
    if missing_evidence:
        errors.append("Required evidence items are missing: " + ", ".join(missing_evidence))
    body_lines = {line.strip() for line in body.splitlines()}
    if "- Independent review: passed" not in body_lines:
        errors.append("Independent review attestation is not passed")
    if f"- Reviewed head: `{expected_head}`" not in body_lines:
        errors.append("Independent review does not attest the expected head")
    finding_lines = [
        line.removeprefix("- Findings:").strip()
        for line in body_lines
        if line.startswith("- Findings:")
    ]
    if len(finding_lines) != 1 or not (
        finding_lines[0].lower() == "none"
        or finding_lines[0].lower().startswith("resolved")
    ):
        errors.append("Independent review findings are not resolved")

    runtime_matches = protected_path_matches(
        files, policy["auto_merge"]["runtime_evidence_paths"]
    )
    if runtime_matches:
        if "- Runtime evidence: verified" not in body_lines:
            errors.append("Required runtime evidence is not verified")
        if f"- Runtime head: `{expected_head}`" not in body_lines:
            errors.append("Runtime evidence does not attest the expected head")
        for prefix in ("- Runtime environment:", "- Runtime fixture and result:"):
            values = [
                line.removeprefix(prefix).strip()
                for line in body_lines
                if line.startswith(prefix)
            ]
            if len(values) != 1 or any(
                marker in values[0].lower()
                for marker in (
                    "pending",
                    "n/a",
                    "not available",
                    "unavailable",
                    "not required",
                    "not needed",
                )
            ) or not values[0]:
                errors.append(f"Runtime evidence field is incomplete: {prefix}")

    active_issue_labels = {
        policy["queue"]["ready_label"],
        policy["queue"]["claimed_label"],
    }
    linked_issues = pull_request.get("closingIssuesReferences", [])
    if pull_request.get("closingIssuesReferencesIncomplete"):
        errors.append("Linked issue result is incomplete")
    if len(linked_issues) != 1:
        errors.append("PR must close exactly one issue")
    if any(issue.get("labelsIncomplete") for issue in linked_issues if isinstance(issue, dict)):
        errors.append("Linked issue label result is incomplete")
    blocking_issue_labels = {
        policy["queue"]["blocked_label"],
        policy["pull_requests"]["human_decision_label"],
    }
    qualifying_issues = [
        issue
        for issue in linked_issues
        if isinstance(issue, dict)
        and issue.get("state") == "OPEN"
        and issue.get("repository") == pull_request.get("repository")
        and labels_from_item(issue).intersection(active_issue_labels)
        and not labels_from_item(issue).intersection(blocking_issue_labels)
    ]
    if len(qualifying_issues) != 1:
        errors.append("PR must close exactly one eligible issue in the same repository")
    if any(
        isinstance(issue, dict)
        and issue.get("state") == "OPEN"
        and labels_from_item(issue).intersection(blocking_issue_labels)
        for issue in linked_issues
    ):
        errors.append("A linked issue is blocked or requires a human decision")

    checks = latest_checks(pull_request.get("statusCheckRollup", []))
    missing_checks = [
        name for name in policy["auto_merge"]["required_checks"] if name not in checks
    ]
    if missing_checks:
        errors.append("Required checks are missing: " + ", ".join(missing_checks))
    unsuccessful_checks = sorted(
        name for name, check in checks.items() if not check_succeeded(check)
    )
    if unsuccessful_checks:
        errors.append("Checks are not successful: " + ", ".join(unsuccessful_checks))

    if review_threads.get("pageInfo", {}).get("hasNextPage"):
        errors.append("Review thread result is incomplete")
    unresolved_threads = [
        thread
        for thread in review_threads.get("nodes", [])
        if isinstance(thread, dict) and not thread.get("isResolved")
    ]
    if unresolved_threads:
        errors.append(f"PR has {len(unresolved_threads)} unresolved review thread(s)")
    return errors


def gh_value(arguments: list[str]) -> object:
    completed = subprocess.run(
        ["gh", *arguments],
        check=True,
        text=True,
        stdout=subprocess.PIPE,
    )
    return json.loads(completed.stdout)


def gh_json(arguments: list[str]) -> dict:
    value = gh_value(arguments)
    if not isinstance(value, dict):
        raise ValueError("GitHub CLI returned a non-object JSON value")
    return value


def gh_paginated_array(endpoint: str) -> list[dict]:
    pages = gh_value(["api", "--paginate", "--slurp", endpoint])
    if not isinstance(pages, list) or any(not isinstance(page, list) for page in pages):
        raise ValueError(f"GitHub CLI returned an invalid paginated result for {endpoint}")
    return [item for page in pages for item in page if isinstance(item, dict)]


def latest_decisive_reviews(reviews: Iterable[dict]) -> list[dict]:
    latest: dict[str, dict] = {}
    for review in reviews:
        if not isinstance(review, dict) or review.get("state") not in {
            "APPROVED",
            "CHANGES_REQUESTED",
            "DISMISSED",
        }:
            continue
        author = review.get("user", {}).get("login")
        if not isinstance(author, str):
            continue
        current = latest.get(author)
        current_key = (
            (current.get("submitted_at") or "", current.get("id") or 0)
            if current
            else ("", 0)
        )
        review_key = (review.get("submitted_at") or "", review.get("id") or 0)
        if current is None or review_key >= current_key:
            latest[author] = review
    return [{"state": review.get("state")} for review in latest.values()]


def fetch_merge_gate_state(repository: str, number: int) -> tuple[dict, dict]:
    if repository.count("/") != 1:
        raise ValueError("--repo must use OWNER/REPO")
    owner, name = repository.split("/", 1)
    fields = ",".join(
        (
            "baseRefName",
            "body",
            "headRefOid",
            "isDraft",
            "mergeable",
            "mergeStateStatus",
            "reviewDecision",
            "state",
            "url",
        )
    )
    pull_request = gh_json(
        ["pr", "view", str(number), "--repo", repository, "--json", fields]
    )
    rest_pull_request = gh_json(["api", f"repos/{repository}/pulls/{number}"])
    pull_request["repository"] = repository
    pull_request["labels"] = [
        {"name": label["name"]}
        for label in rest_pull_request.get("labels", [])
        if isinstance(label, dict) and isinstance(label.get("name"), str)
    ]
    pull_request["reviewRequests"] = [
        *rest_pull_request.get("requested_reviewers", []),
        *rest_pull_request.get("requested_teams", []),
    ]

    file_items = gh_paginated_array(
        f"repos/{repository}/pulls/{number}/files?per_page=100"
    )
    if rest_pull_request.get("changed_files") != len(file_items):
        raise ValueError("Pull-request file result is incomplete")
    if any(not isinstance(item.get("filename"), str) for item in file_items):
        raise ValueError("Pull-request file result contains an unnamed file")
    file_paths: list[str] = []
    for item in file_items:
        previous = item.get("previous_filename")
        current = item.get("filename")
        if isinstance(previous, str):
            file_paths.append(previous)
        if isinstance(current, str):
            file_paths.append(current)
    pull_request["files"] = [{"path": path} for path in file_paths]

    head = pull_request.get("headRefOid")
    check_pages = gh_value(
        [
            "api",
            "--paginate",
            "--slurp",
            f"repos/{repository}/commits/{head}/check-runs?per_page=100&filter=all",
        ]
    )
    if not isinstance(check_pages, list) or any(
        not isinstance(page, dict) or not isinstance(page.get("check_runs"), list)
        for page in check_pages
    ):
        raise ValueError("GitHub CLI returned an invalid check-runs result")
    check_run_count = sum(len(page["check_runs"]) for page in check_pages)
    expected_check_runs = max(
        (page.get("total_count", check_run_count) for page in check_pages),
        default=0,
    )
    if check_run_count < expected_check_runs:
        raise ValueError("Check-run result is incomplete")
    checks: list[dict] = []
    for page in check_pages:
        for check in page["check_runs"]:
            if not isinstance(check, dict):
                continue
            check_name = check.get("name")
            app = check.get("app") or {}
            app_slug = app.get("slug") or app.get("name") or "unknown-app"
            identifier = f"{app_slug} / {check_name}"
            if app_slug == "github-actions":
                job_match = re.search(r"/job/(\d+)(?:$|[/?#])", check.get("details_url") or "")
                if not job_match:
                    raise ValueError("GitHub Actions check is missing a job identifier")
                job = gh_json(
                    ["api", f"repos/{repository}/actions/jobs/{job_match.group(1)}"]
                )
                if job.get("head_sha") != head:
                    raise ValueError("GitHub Actions job does not match the PR head")
                workflow_name = job.get("workflow_name")
                if not isinstance(workflow_name, str) or not workflow_name:
                    raise ValueError("GitHub Actions job is missing its workflow name")
                identifier = f"{workflow_name} / {check_name}"
            checks.append(
                {
            "__typename": "CheckRun",
            "id": check.get("id"),
            "identifier": identifier,
            "name": check_name,
            "status": str(check.get("status") or "").upper(),
            "conclusion": (
                str(check["conclusion"]).upper() if check.get("conclusion") else None
            ),
            "startedAt": check.get("started_at") or "",
            "completedAt": check.get("completed_at") or "",
                }
            )
    statuses = gh_paginated_array(
        f"repos/{repository}/commits/{head}/statuses?per_page=100"
    )
    checks.extend(
        {
            "__typename": "StatusContext",
            "id": status.get("id"),
            "context": status.get("context"),
            "state": str(status.get("state") or "").upper(),
            "startedAt": status.get("created_at") or "",
            "completedAt": status.get("updated_at") or "",
        }
        for status in statuses
    )
    pull_request["statusCheckRollup"] = checks

    reviews = gh_paginated_array(
        f"repos/{repository}/pulls/{number}/reviews?per_page=100"
    )
    pull_request["latestReviews"] = latest_decisive_reviews(reviews)

    query = """
query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      closingIssuesReferences(first: 10) {
        nodes {
          number
          state
          repository { nameWithOwner }
          labels(first: 100) {
            nodes { name }
            pageInfo { hasNextPage }
          }
        }
        pageInfo { hasNextPage }
      }
      reviewThreads(first: 100) {
        nodes { isResolved }
        pageInfo { hasNextPage }
      }
    }
  }
}
"""
    response = gh_json(
        [
            "api",
            "graphql",
            "-f",
            f"query={query}",
            "-f",
            f"owner={owner}",
            "-f",
            f"name={name}",
            "-F",
            f"number={number}",
        ]
    )
    remote_pull_request = response["data"]["repository"]["pullRequest"]
    closing_issues = remote_pull_request["closingIssuesReferences"]
    pull_request["closingIssuesReferences"] = [
        {
            "number": issue["number"],
            "state": issue["state"],
            "repository": issue["repository"]["nameWithOwner"],
            "labels": issue.get("labels", {}).get("nodes", []),
            "labelsIncomplete": issue.get("labels", {})
            .get("pageInfo", {})
            .get("hasNextPage", False),
        }
        for issue in closing_issues.get("nodes", [])
    ]
    pull_request["closingIssuesReferencesIncomplete"] = closing_issues.get(
        "pageInfo", {}
    ).get("hasNextPage", False)
    review_threads = remote_pull_request["reviewThreads"]
    return pull_request, review_threads


def parse_name_status_z(output: str) -> list[str]:
    tokens = output.split("\0")
    if tokens and not tokens[-1]:
        tokens.pop()
    paths: list[str] = []
    index = 0
    while index < len(tokens):
        status = tokens[index]
        index += 1
        path_count = 2 if status[:1] in {"R", "C"} else 1
        if index + path_count > len(tokens):
            raise ValueError("Malformed git --name-status -z output")
        paths.extend(tokens[index : index + path_count])
        index += path_count
    return paths


def git_changed_paths(base: str, head: str) -> list[str]:
    completed = subprocess.run(
        [
            "git",
            "diff",
            "--name-status",
            "-z",
            "--diff-filter=ACDMRTUXB",
            f"{base}...{head}",
        ],
        check=True,
        text=True,
        stdout=subprocess.PIPE,
    )
    return parse_name_status_z(completed.stdout)


def sync_labels(policy: dict, repository: str) -> None:
    validate_policy(policy)
    for label in policy["labels"]:
        subprocess.run(
            [
                "gh",
                "label",
                "create",
                label["name"],
                "--repo",
                repository,
                "--color",
                label["color"],
                "--description",
                label["description"],
                "--force",
            ],
            check=True,
        )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    validate = subparsers.add_parser("validate-pr", help="Validate an agent-authored PR event")
    validate.add_argument("--policy", required=True)
    validate.add_argument("--event", required=True)
    validate.add_argument("--base", required=True)
    validate.add_argument("--head", required=True)

    sync = subparsers.add_parser("sync-labels", help="Create or update declared GitHub labels")
    sync.add_argument("--policy", required=True)
    sync.add_argument("--repo", required=True)

    classify = subparsers.add_parser("classify", help="Report protected paths for a proposed change")
    classify.add_argument("--policy", required=True)
    classify.add_argument("paths", nargs="+")

    gate = subparsers.add_parser(
        "merge-gate", help="Validate the current remote PR state before an agent merge"
    )
    gate.add_argument("--policy", required=True)
    gate.add_argument("--repo", required=True)
    gate.add_argument("--pr", required=True, type=int)
    gate.add_argument("--expected-head", required=True)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    policy = load_json(args.policy)

    if args.command == "sync-labels":
        sync_labels(policy, args.repo)
        return 0
    if args.command == "classify":
        validate_policy(policy)
        matches = protected_path_matches(args.paths, policy["auto_merge"]["protected_paths"])
        print(json.dumps({"protected": bool(matches), "matches": matches}, indent=2))
        return 0

    if args.command == "merge-gate":
        pull_request, review_threads = fetch_merge_gate_state(args.repo, args.pr)
        errors = validate_merge_gate(policy, pull_request, review_threads, args.expected_head)
        if not errors:
            print(
                json.dumps(
                    {
                        "eligible": True,
                        "head": pull_request["headRefOid"],
                        "pull_request": pull_request["url"],
                    },
                    indent=2,
                )
            )
            return 0
        for error in errors:
            print(f"error: {error}", file=sys.stderr)
        return 1

    event = load_json(args.event)
    paths = git_changed_paths(args.base, args.head)
    errors = validate_pull_request(policy, event, paths)
    if not errors:
        print("Agent policy passed")
        return 0
    for error in errors:
        if os.environ.get("GITHUB_ACTIONS") == "true":
            print(f"::error title=Agent policy::{error}")
        else:
            print(f"error: {error}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
