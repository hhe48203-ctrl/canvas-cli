#!/usr/bin/env python3
"""Validate and synchronize the repository's agent workflow policy."""

from __future__ import annotations

import argparse
import fnmatch
import json
import os
from pathlib import Path
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
    labels = policy.get("labels")
    if not isinstance(pull_requests, dict) or not isinstance(auto_merge, dict):
        raise ValueError("Policy must define pull_requests and auto_merge objects")
    if not isinstance(labels, list) or not labels:
        raise ValueError("Policy must define at least one label")
    names = [entry.get("name") for entry in labels if isinstance(entry, dict)]
    if len(names) != len(labels) or any(not name for name in names):
        raise ValueError("Every policy label needs a non-empty name")
    if len(set(names)) != len(names):
        raise ValueError("Policy label names must be unique")
    referenced = {
        pull_requests.get("agent_label"),
        pull_requests.get("auto_merge_label"),
        pull_requests.get("human_decision_label"),
        auto_merge.get("allowed_risk_label"),
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
        if selected_risks != [policy["auto_merge"]["allowed_risk_label"]]:
            errors.append(
                f"{auto_label} requires exactly {policy['auto_merge']['allowed_risk_label']}"
            )
        if human_label in labels:
            errors.append(f"{auto_label} cannot be combined with {human_label}")
        if pull_request.get("draft"):
            errors.append(f"{auto_label} cannot be used on a draft PR")
        protected = protected_path_matches(paths, policy["auto_merge"]["protected_paths"])
        if protected:
            errors.append("Automatic merge is blocked by protected paths: " + ", ".join(protected))

    return errors


def git_changed_paths(base: str, head: str) -> list[str]:
    completed = subprocess.run(
        ["git", "diff", "--name-only", "--diff-filter=ACDMRTUXB", f"{base}...{head}"],
        check=True,
        text=True,
        stdout=subprocess.PIPE,
    )
    return [line for line in completed.stdout.splitlines() if line]


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
