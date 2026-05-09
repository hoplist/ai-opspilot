import unittest
from unittest.mock import Mock

from auto_inspection.mcp_client import MCPClient, PROTOCOL_VERSION


class TestMcpClient(unittest.TestCase):
    def test_initialize_sends_initialized_notification_and_protocol_header(self):
        session = Mock()

        initialize_response = Mock()
        initialize_response.status_code = 200
        initialize_response.content = b'{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}'
        initialize_response.headers = {"Mcp-Session-Id": "session-123"}
        initialize_response.json.return_value = {
            "jsonrpc": "2.0",
            "id": 1,
            "result": {"protocolVersion": PROTOCOL_VERSION},
        }

        notify_response = Mock()
        notify_response.status_code = 202
        notify_response.content = b""

        session.post.side_effect = [initialize_response, notify_response]

        client = MCPClient()
        client._session = session

        result = client.initialize()

        self.assertEqual(result["protocolVersion"], PROTOCOL_VERSION)
        self.assertEqual(session.post.call_count, 2)

        init_headers = session.post.call_args_list[0].kwargs["headers"]
        self.assertEqual(init_headers["Accept"], "application/json, text/event-stream")
        self.assertNotIn("MCP-Protocol-Version", init_headers)

        notify_headers = session.post.call_args_list[1].kwargs["headers"]
        self.assertEqual(notify_headers["MCP-Protocol-Version"], PROTOCOL_VERSION)
        self.assertEqual(notify_headers["Mcp-Session-Id"], "session-123")
        notify_payload = session.post.call_args_list[1].kwargs["json"]
        self.assertEqual(notify_payload["method"], "notifications/initialized")
        self.assertEqual(client._session_id, "session-123")

    def test_request_returns_none_for_202_empty_response(self):
        session = Mock()

        response = Mock()
        response.status_code = 202
        response.content = b""
        session.post.return_value = response

        client = MCPClient()
        client._session = session
        client._protocol_version = PROTOCOL_VERSION

        result = client._request({"jsonrpc": "2.0", "method": "notifications/initialized"})

        self.assertIsNone(result)


if __name__ == "__main__":
    unittest.main()
