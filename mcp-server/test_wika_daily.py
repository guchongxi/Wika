#!/usr/bin/env python3
import unittest

from weknora_mcp_server import WeKnoraClient


class RecordingClient(WeKnoraClient):
    def __init__(self):
        super().__init__(
            "http://localhost:8080/api/v1",
            api_key="tenant-key",
            pat="wika_pat_test",
        )
        self.calls = []

    def _request(self, method, endpoint, **kwargs):
        self.calls.append((method, endpoint, kwargs))
        return {"ok": True}


class WikaDailyToolsTest(unittest.TestCase):
    def test_pat_uses_bearer_without_api_key_header(self):
        client = WeKnoraClient(
            "http://localhost:8080/api/v1",
            api_key="tenant-key",
            pat="wika_pat_test",
        )

        self.assertEqual(
            client.session.headers.get("Authorization"),
            "Bearer wika_pat_test",
        )
        self.assertNotIn("X-API-Key", client.session.headers)

    def test_push_and_search_use_wika_daily_api(self):
        client = RecordingClient()

        client.push_knowledge(
            title="排查记录",
            content="先看日志",
            source="incident-42",
            tags=["debug"],
            evidence="日志片段",
            expires_at="2026-07-29T12:00:00Z",
            idempotency_key="idem-1",
            dry_run=True,
        )
        client.search_knowledge("排查", limit=3, include_team=False, format="compact")
        client.expand_knowledge_result(["k-1", "k-2"])
        client.get_my_knowledge(limit=10, status="fresh", tag="debug")
        client.suggest_to_team(
            "k-1",
            target_space_id=80,
            target_kb_id="kb-team",
            reason="团队可复用",
            idempotency_key="idem-suggest-1",
        )

        self.assertEqual(client.calls[0][0], "POST")
        self.assertEqual(client.calls[0][1], "/wika/knowledge/push")
        self.assertEqual(client.calls[0][2]["json"]["idempotency_key"], "idem-1")
        self.assertTrue(client.calls[0][2]["json"]["dry_run"])
        self.assertEqual(client.calls[1][0], "POST")
        self.assertEqual(client.calls[1][1], "/wika/knowledge/search")
        self.assertEqual(client.calls[1][2]["json"]["query"], "排查")
        self.assertFalse(client.calls[1][2]["json"]["include_team"])
        self.assertEqual(client.calls[2][0], "POST")
        self.assertEqual(client.calls[2][1], "/wika/knowledge/expand")
        self.assertEqual(client.calls[2][2]["json"]["ids"], ["k-1", "k-2"])
        self.assertEqual(client.calls[3][0], "GET")
        self.assertEqual(client.calls[3][1], "/wika/knowledge/mine")
        self.assertEqual(client.calls[3][2]["params"]["limit"], 10)
        self.assertEqual(client.calls[3][2]["params"]["status"], "fresh")
        self.assertEqual(client.calls[3][2]["params"]["tag"], "debug")
        self.assertEqual(client.calls[4][0], "POST")
        self.assertEqual(client.calls[4][1], "/wika/suggestions")
        self.assertEqual(client.calls[4][2]["json"]["knowledge_id"], "k-1")
        self.assertEqual(client.calls[4][2]["json"]["target_space_id"], 80)
        self.assertEqual(client.calls[4][2]["json"]["target_kb_id"], "kb-team")
        self.assertEqual(client.calls[4][2]["json"]["reason"], "团队可复用")
        self.assertEqual(client.calls[4][2]["json"]["idempotency_key"], "idem-suggest-1")


if __name__ == "__main__":
    unittest.main()
