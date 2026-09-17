import copy
import unittest

import agentctl


POLICY = {
    "schema_version": 1,
    "pull_requests": {
        "agent_label": "agent:codex",
        "auto_merge_label": "agent:auto-merge",
        "human_decision_label": "decision:human",
        "risk_labels": ["risk:low", "risk:medium", "risk:high"],
        "required_body_headings": ["## Outcome", "## Evidence", "## Risk and decisions"],
    },
    "auto_merge": {
        "allowed_risk_label": "risk:low",
        "protected_paths": [".github/workflows/**", "go.mod"],
    },
    "labels": [
        {"name": name, "color": "000000", "description": name}
        for name in (
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
            or "## Outcome\nDone\n\n## Evidence\nTests\n\n## Risk and decisions\nLow\n",
            "labels": [{"name": label} for label in labels],
        }
    }


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

    def test_auto_merge_rejects_draft_human_decision_and_wrong_risk(self):
        errors = agentctl.validate_pull_request(
            POLICY,
            event(
                "agent:codex",
                "agent:auto-merge",
                "decision:human",
                "risk:medium",
                draft=True,
            ),
            [],
        )
        self.assertTrue(any("risk:low" in error for error in errors))
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


if __name__ == "__main__":
    unittest.main()
