#!/usr/bin/env python3
import asyncio
import os
import unittest
from unittest.mock import patch

from weknora_mcp_server import (
    WeKnoraClient,
    _REMOTE_AUTHORIZATION_HEADER,
    _REMOTE_TOOLSET_HEADER,
    handle_call_tool,
    handle_list_tools,
)


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

    def test_remote_authorization_header_overrides_server_api_key(self):
        client = WeKnoraClient(
            "http://localhost:8080/api/v1",
            api_key="tenant-key",
            pat="",
        )

        token = _REMOTE_AUTHORIZATION_HEADER.set("Bearer wika_pat_remote")
        try:
            headers = client._request_headers()
        finally:
            _REMOTE_AUTHORIZATION_HEADER.reset(token)

        self.assertEqual(headers["Authorization"], "Bearer wika_pat_remote")
        self.assertNotIn("X-API-Key", headers)

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

    def test_push_knowledge_preserves_intake_location_metadata(self):
        class MetadataClient(RecordingClient):
            def _request(self, method, endpoint, **kwargs):
                self.calls.append((method, endpoint, kwargs))
                return {
                    "knowledge_id": "knowledge-1",
                    "tenant_id": 70,
                    "kb_id": "kb-personal",
                    "source_channel": "api",
                    "created_at": "2026-07-02T10:20:30Z",
                    "status": "created",
                }

        client = MetadataClient()

        result = client.push_knowledge(content="先看日志", title="排查记录")

        self.assertEqual(result["knowledge_id"], "knowledge-1")
        self.assertEqual(result["tenant_id"], 70)
        self.assertEqual(result["kb_id"], "kb-personal")
        self.assertEqual(result["source_channel"], "api")
        self.assertEqual(result["created_at"], "2026-07-02T10:20:30Z")

    def test_default_toolset_only_exposes_wika_daily_tools(self):
        env = {
            key: value
            for key, value in os.environ.items()
            if key != "WEKNORA_MCP_TOOLSET"
        }
        with patch.dict(os.environ, env, clear=True):
            tools = asyncio.run(handle_list_tools())

        names = {tool.name for tool in tools}
        self.assertEqual(
            names,
            {
                "push_knowledge",
                "search_knowledge",
                "expand_knowledge_result",
                "get_my_knowledge",
                "suggest_to_team",
            },
        )
        self.assertNotIn("create_tenant", names)
        self.assertNotIn("delete_knowledge_base", names)
        self.assertNotIn("create_model", names)

    def test_dynamic_toolset_exposes_admin_tools_when_backend_authorizes(self):
        with patch.dict(os.environ, {"WEKNORA_MCP_TOOLSET": "dynamic"}):
            toolset_token = _REMOTE_TOOLSET_HEADER.set("admin")
            auth_token = _REMOTE_AUTHORIZATION_HEADER.set("Bearer wika_pat_admin")
            try:
                with patch("weknora_mcp_server.client") as mock_client:
                    mock_client.authorize_mcp_admin_toolset.return_value = True
                    tools = asyncio.run(handle_list_tools())
            finally:
                _REMOTE_AUTHORIZATION_HEADER.reset(auth_token)
                _REMOTE_TOOLSET_HEADER.reset(toolset_token)

        names = {tool.name for tool in tools}
        self.assertIn("create_tenant", names)
        self.assertIn("delete_knowledge_base", names)
        self.assertIn("create_model", names)

    def test_dynamic_toolset_rejects_admin_tools_when_backend_denies(self):
        with patch.dict(os.environ, {"WEKNORA_MCP_TOOLSET": "dynamic"}):
            toolset_token = _REMOTE_TOOLSET_HEADER.set("admin")
            auth_token = _REMOTE_AUTHORIZATION_HEADER.set("Bearer wika_pat_viewer")
            try:
                with patch("weknora_mcp_server.client") as mock_client:
                    mock_client.authorize_mcp_admin_toolset.return_value = False
                    with self.assertRaises(PermissionError):
                        asyncio.run(handle_list_tools())
                    result = asyncio.run(
                        handle_call_tool(
                            "create_tenant",
                            {
                                "name": "tenant",
                                "description": "tenant",
                                "business": "test",
                            },
                        )
                    )
            finally:
                _REMOTE_AUTHORIZATION_HEADER.reset(auth_token)
                _REMOTE_TOOLSET_HEADER.reset(toolset_token)

        self.assertIn("Permission denied", result[0].text)
        mock_client.create_tenant.assert_not_called()

    def test_default_toolset_blocks_non_daily_tool_calls(self):
        env = {
            key: value
            for key, value in os.environ.items()
            if key != "WEKNORA_MCP_TOOLSET"
        }
        with patch.dict(os.environ, env, clear=True):
            result = asyncio.run(
                handle_call_tool(
                    "create_tenant",
                    {
                        "name": "tenant",
                        "description": "tenant",
                        "business": "test",
                    },
                )
            )

        self.assertIn("daily MCP toolset", result[0].text)

    def test_all_toolset_exposes_admin_tools(self):
        with patch.dict(os.environ, {"WEKNORA_MCP_TOOLSET": "all"}):
            tools = asyncio.run(handle_list_tools())

        names = {tool.name for tool in tools}
        self.assertIn("create_tenant", names)
        self.assertIn("delete_knowledge_base", names)
        self.assertIn("create_model", names)


if __name__ == "__main__":
    unittest.main()
