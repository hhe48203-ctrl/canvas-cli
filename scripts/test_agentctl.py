import copy
import unittest
from unittest import mock

import agentctl


POLICY = {
    "schema_version": 1,
    "default_branch": "main",
    "queue": {
        "ready_label": "agent:ready",
        "claimed_label": "agent:in-progress",
        "blocked_label": "agent:blocked",
    },
    "pull_requests": {
        "agent_label": "agent:codex",
        "auto_merge_label": "agent:auto-merge",
        "human_decision_label": "decision:human",
        "risk_labels": ["risk:low", "risk:medium", "risk:high"],
        "required_body_headings": [
            "## Outcome",
            "## Evidence",
            "## Risk and decisions",
            "## Independent review",
        ],
    },
    "auto_merge": {
        "allowed_risk_labels": ["risk:low", "risk:medium"],
        "required_checks": ["CI / test", "Agent policy / validate"],
        "required_evidence_items": ["Targeted test", "Full gate"],
        "runtime_evidence_paths": [],
        "protected_paths": [".github/workflows/**", "go.mod"],
    },
    "labels": [
        {"name": name, "color": "000000", "description": name}
        for name in (
            "agent:ready",
            "agent:in-progress",
            "agent:blocked",
            "agent:codex",
            "agent:auto-merge",
            "decision:human",
            "risk:low",
            "risk:medium",
            "risk:high",
        )
    ],
}


def event(*labels, draft=False, body=None):
    return {
        "pull_request": {
            "draft": draft,
            "body": body
            or (
                "## Outcome\nDone\n\n## Evidence\nTests\n\n"
                "## Risk and decisions\nLow\n\n## Independent review\nPending\n"
            ),
            "labels": [{"name": label} for label in labels],
        }
    }


def remote_pull_request(**overrides):
    value = {
        "baseRefName": "main",
        "body": (
            "## Outcome\nDone\n\n## Evidence\n- [x] Targeted test\n"
            "- [x] Full gate\n\n## Risk and decisions\nMedium\n\n"
            "## Independent review\n- Independent review: passed\n"
            "- Reviewed head: `abc123`\n- Findings: none\n"
        ),
        "closingIssuesReferences": [
            {
                "state": "OPEN",
                "repository": "owner/repo",
                "labels": [{"name": "agent:in-progress"}],
            }
        ],
        "files": [{"path": "cmd/root.go"}],
        "headRefOid": "abc123",
        "repository": "owner/repo",
        "isDraft": False,
        "labels": [
            {"name": "agent:codex"},
            {"name": "agent:auto-merge"},
            {"name": "risk:medium"},
        ],
        "latestReviews": [],
        "mergeable": "MERGEABLE",
        "mergeStateStatus": "CLEAN",
        "reviewDecision": "APPROVED",
        "reviewRequests": [],
        "state": "OPEN",
        "statusCheckRollup": [
            {
                "__typename": "CheckRun",
                "id": 1,
                "workflowName": "CI",
                "name": "test",
                "status": "COMPLETED",
                "conclusion": "SUCCESS",
                "startedAt": "2026-01-01T00:00:00Z",
            },
            {
                "__typename": "CheckRun",
                "id": 3,
                "workflowName": "Agent policy",
                "name": "validate",
                "status": "COMPLETED",
                "conclusion": "SUCCESS",
                "startedAt": "2026-01-01T00:00:00Z",
            },
        ],
        "url": "https://example.test/pull/1",
    }
    value.update(overrides)
    return value


EMPTY_THREADS = {"nodes": [], "pageInfo": {"hasNextPage": False}}


class AgentPolicyTests(unittest.TestCase):
    def test_human_pull_request_is_not_forced_into_agent_protocol(self):
        self.assertEqual(agentctl.validate_pull_request(POLICY, event(), []), [])

    def test_agent_pull_request_needs_exactly_one_risk(self):
        errors = agentctl.validate_pull_request(POLICY, event("agent:codex"), [])
        self.assertTrue(any("exactly one risk" in error for error in errors))
        errors = agentctl.validate_pull_request(
            POLICY, event("agent:codex", "risk:low", "risk:medium"), []
        )
        self.assertTrue(any("exactly one risk" in error for error in errors))

    def test_agent_pull_request_needs_evidence_headings(self):
        errors = agentctl.validate_pull_request(
            POLICY, event("agent:codex", "risk:low", body="## Outcome\nDone"), []
        )
        self.assertTrue(any("missing required headings" in error for error in errors))

    def test_low_risk_auto_merge_passes(self):
        errors = agentctl.validate_pull_request(
            POLICY, event("agent:codex", "agent:auto-merge", "risk:low"), ["cmd/root.go"]
        )
        self.assertEqual(errors, [])

    def test_medium_risk_auto_merge_passes(self):
        errors = agentctl.validate_pull_request(
            POLICY,
            event("agent:codex", "agent:auto-merge", "risk:medium"),
            ["cmd/root.go"],
        )
        self.assertEqual(errors, [])

    def test_auto_merge_rejects_draft_human_decision_and_high_risk(self):
        errors = agentctl.validate_pull_request(
            POLICY,
            event(
                "agent:codex",
                "agent:auto-merge",
                "decision:human",
                "risk:high",
                draft=True,
            ),
            [],
        )
        self.assertTrue(any("allowed risk label" in error for error in errors))
        self.assertTrue(any("decision:human" in error for error in errors))
        self.assertTrue(any("draft" in error for error in errors))

    def test_auto_merge_rejects_protected_path(self):
        errors = agentctl.validate_pull_request(
            POLICY,
            event("agent:codex", "agent:auto-merge", "risk:low"),
            [".github/workflows/ci.yml"],
        )
        self.assertTrue(any("protected paths" in error for error in errors))

    def test_auto_merge_requires_agent_label(self):
        errors = agentctl.validate_pull_request(
            POLICY, event("agent:auto-merge", "risk:low"), []
        )
        self.assertTrue(any("requires agent:codex" in error for error in errors))

    def test_policy_rejects_undeclared_referenced_label(self):
        policy = copy.deepcopy(POLICY)
        policy["pull_requests"]["agent_label"] = "agent:missing"
        with self.assertRaisesRegex(ValueError, "not declared"):
            agentctl.validate_policy(policy)

    def test_merge_gate_allows_current_green_medium_risk_pr(self):
        self.assertEqual(
            agentctl.validate_merge_gate(
                POLICY, remote_pull_request(), EMPTY_THREADS, "abc123"
            ),
            [],
        )

    def test_merge_gate_requires_agent_and_auto_merge_labels(self):
        pull_request = remote_pull_request(labels=[{"name": "risk:medium"}])
        errors = agentctl.validate_merge_gate(
            POLICY, pull_request, EMPTY_THREADS, "abc123"
        )
        self.assertTrue(any("requires agent:codex" in error for error in errors))
        self.assertTrue(any("requires agent:auto-merge" in error for error in errors))

        blocked = remote_pull_request()
        blocked["labels"].append({"name": "agent:blocked"})
        blocked_errors = agentctl.validate_merge_gate(
            POLICY, blocked, EMPTY_THREADS, "abc123"
        )
        self.assertTrue(any("rejects agent:blocked" in error for error in blocked_errors))

    def test_merge_gate_rejects_blocked_or_multiple_linked_issues(self):
        pull_request = remote_pull_request()
        pull_request["closingIssuesReferences"][0]["labels"].append(
            {"name": "agent:blocked"}
        )
        pull_request["closingIssuesReferences"].append(
            {
                "state": "OPEN",
                "repository": "owner/repo",
                "labels": [],
            }
        )
        errors = agentctl.validate_merge_gate(
            POLICY, pull_request, EMPTY_THREADS, "abc123"
        )
        self.assertTrue(any("exactly one issue" in error for error in errors))
        self.assertTrue(any("blocked or requires" in error for error in errors))

    def test_merge_gate_requires_complete_evidence_and_resolved_review(self):
        pull_request = remote_pull_request()
        pull_request["body"] = pull_request["body"].replace(
            "- Findings: none", "- Findings: pending"
        ) + "\n* [ ] Extra evidence\n"
        errors = agentctl.validate_merge_gate(
            POLICY, pull_request, EMPTY_THREADS, "abc123"
        )
        self.assertTrue(any("unresolved evidence" in error for error in errors))
        self.assertTrue(any("findings are not resolved" in error for error in errors))

    def test_merge_gate_rejects_stale_head_pending_check_and_unresolved_thread(self):
        pull_request = remote_pull_request(
            body="## Outcome\nDone\n\n## Evidence\n- [ ] Runtime\n\n## Risk and decisions\nMedium\n",
            statusCheckRollup=[
                {
                    "__typename": "CheckRun",
                    "workflowName": "CI",
                    "name": "test",
                    "status": "IN_PROGRESS",
                    "conclusion": None,
                    "startedAt": "2026-01-02T00:00:00Z",
                }
            ],
        )
        errors = agentctl.validate_merge_gate(
            POLICY,
            pull_request,
            {"nodes": [{"isResolved": False}], "pageInfo": {"hasNextPage": False}},
            "different",
        )
        self.assertTrue(any("expected-head" in error for error in errors))
        self.assertTrue(any("unresolved evidence" in error for error in errors))
        self.assertTrue(any("Required checks are missing" in error for error in errors))
        self.assertTrue(any("not successful" in error for error in errors))
        self.assertTrue(any("unresolved review" in error for error in errors))

    def test_merge_gate_uses_latest_attempt_for_duplicate_check(self):
        pull_request = remote_pull_request()
        pull_request["statusCheckRollup"].insert(
            0,
            {
                "__typename": "CheckRun",
                "workflowName": "CI",
                "name": "test",
                "status": "COMPLETED",
                "conclusion": "CANCELLED",
                "startedAt": "2025-12-31T00:00:00Z",
            },
        )
        self.assertEqual(
            agentctl.validate_merge_gate(POLICY, pull_request, EMPTY_THREADS, "abc123"),
            [],
        )

    def test_newer_queued_check_without_timestamps_blocks_merge(self):
        pull_request = remote_pull_request()
        pull_request["statusCheckRollup"].append(
            {
                "__typename": "CheckRun",
                "id": 2,
                "workflowName": "CI",
                "name": "test",
                "status": "QUEUED",
                "conclusion": None,
                "startedAt": "",
                "completedAt": "",
            }
        )
        errors = agentctl.validate_merge_gate(
            POLICY, pull_request, EMPTY_THREADS, "abc123"
        )
        self.assertTrue(any("Checks are not successful" in error for error in errors))

    def test_merge_gate_rejects_incomplete_linked_issue_result(self):
        errors = agentctl.validate_merge_gate(
            POLICY,
            remote_pull_request(closingIssuesReferencesIncomplete=True),
            EMPTY_THREADS,
            "abc123",
        )
        self.assertTrue(any("Linked issue result is incomplete" in error for error in errors))

    def test_parse_name_status_includes_both_sides_of_rename(self):
        paths = agentctl.parse_name_status_z(
            "R100\0.github/workflows/ci.yml\0docs/old-ci.yml\0M\0README.md\0"
        )
        self.assertEqual(
            paths,
            [".github/workflows/ci.yml", "docs/old-ci.yml", "README.md"],
        )

    def test_later_comment_does_not_clear_requested_changes(self):
        reviews = agentctl.latest_decisive_reviews(
            [
                {
                    "id": 1,
                    "state": "CHANGES_REQUESTED",
                    "submitted_at": "2026-01-01T00:00:00Z",
                    "user": {"login": "reviewer"},
                },
                {
                    "id": 2,
                    "state": "COMMENTED",
                    "submitted_at": "2026-01-02T00:00:00Z",
                    "user": {"login": "reviewer"},
                },
            ]
        )
        self.assertEqual(reviews, [{"state": "CHANGES_REQUESTED"}])

    @mock.patch("agentctl.gh_json")
    @mock.patch("agentctl.gh_value")
    def test_fetch_merge_gate_state_paginates_files_and_loads_issue_labels(
        self, gh_value, gh_json
    ):
        gh_value.side_effect = [
            [
                [
                    {
                        "filename": "cmd/root.go",
                        "previous_filename": ".github/workflows/old-ci.yml",
                    }
                ],
                [{"filename": "internal/output/json.go"}],
            ],
            [
                {
                    "total_count": 1,
                    "check_runs": [
                        {
                            "name": "test",
                            "app": {"slug": "github-actions"},
                            "details_url": "https://github.test/actions/runs/1/job/22",
                            "status": "completed",
                            "conclusion": "success",
                            "started_at": "2026-01-01T00:00:00Z",
                            "completed_at": "2026-01-01T00:01:00Z",
                        }
                    ],
                }
            ],
            [[]],
            [[]],
        ]
        gh_json.side_effect = [
            {"headRefOid": "abc123"},
            {
                "changed_files": 2,
                "labels": [{"name": "agent:codex"}],
                "requested_reviewers": [],
                "requested_teams": [],
            },
            {"head_sha": "abc123", "workflow_name": "CI"},
            {
                "data": {
                    "repository": {
                        "pullRequest": {
                            "closingIssuesReferences": {
                                "nodes": [
                                    {
                                        "number": 7,
                                        "state": "OPEN",
                                        "repository": {"nameWithOwner": "owner/repo"},
                                        "labels": {
                                            "nodes": [{"name": "agent:ready"}],
                                            "pageInfo": {"hasNextPage": False},
                                        },
                                    }
                                ],
                                "pageInfo": {"hasNextPage": False},
                            },
                            "reviewThreads": {
                                "nodes": [],
                                "pageInfo": {"hasNextPage": False},
                            },
                        }
                    }
                }
            },
        ]
        pull_request, threads = agentctl.fetch_merge_gate_state("owner/repo", 3)
        self.assertEqual(
            pull_request["files"],
            [
                {"path": ".github/workflows/old-ci.yml"},
                {"path": "cmd/root.go"},
                {"path": "internal/output/json.go"},
            ],
        )
        self.assertEqual(
            pull_request["closingIssuesReferences"][0]["labels"],
            [{"name": "agent:ready"}],
        )
        self.assertEqual(pull_request["statusCheckRollup"][0]["identifier"], "CI / test")
        self.assertEqual(threads, EMPTY_THREADS)


if __name__ == "__main__":
    unittest.main()
