import unittest
import urllib.error
import urllib.request
from threading import Thread

from auto_inspection import auto_inspection_mcp
from http.server import ThreadingHTTPServer


class TestAutoInspectionMcp(unittest.TestCase):
    def test_resource_filter_payload_groups_cpu_mem_disk(self):
        payload = {
            "items": [
                {"metric": "cpu", "instance": "k8s-worker-1", "cluster": "kubernetes", "group": "cluster=kubernetes", "usage_current": 0.7, "remaining_current": 0.3, "remaining_zone": "watch"},
                {"metric": "mem", "instance": "k8s-worker-1", "cluster": "kubernetes", "group": "cluster=kubernetes", "usage_current": 0.5, "remaining_current": 0.5, "remaining_zone": "safe"},
                {"metric": "disk", "instance": "k8s-worker-1", "cluster": "kubernetes", "group": "cluster=kubernetes", "usage_current": 0.8, "remaining_current": 0.2, "remaining_zone": "alert", "avail_bytes": 100, "size_bytes": 500},
                {"metric": "pod", "instance": "default/api-0", "cluster": "kubernetes"},
            ]
        }
        result = auto_inspection_mcp._resource_filter_payload(payload, {"instance": "worker-1", "limit": 5})
        self.assertEqual(len(result["items"]), 1)
        item = result["items"][0]
        self.assertEqual(item["instance"], "k8s-worker-1")
        self.assertEqual(item["cpu_remaining"], 0.3)
        self.assertEqual(item["mem_remaining"], 0.5)
        self.assertEqual(item["disk_remaining"], 0.2)

    def test_get_mcp_opens_sse_stream(self):
        with _run_server() as server:
            session_id = _initialize_session(server)
            _post_notification_initialized(server, session_id)
            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                headers={
                    "Accept": "text/event-stream",
                    "Mcp-Session-Id": session_id,
                    "MCP-Protocol-Version": "2025-06-18",
                },
                method="GET",
            )
            with urllib.request.urlopen(request, timeout=3) as response:
                self.assertEqual(response.status, 200)
                self.assertEqual(response.headers.get_content_type(), "text/event-stream")
                self.assertEqual(response.readline(), b": stream-open\n")
                response.readline()
                event_id_line = response.readline()
                data_line = response.readline()
                self.assertTrue(event_id_line.startswith(b"id: "))
                self.assertTrue(data_line.startswith(b"data: "))

    def test_get_mcp_without_session_opens_probe_sse_stream(self):
        with _run_server() as server:
            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                headers={"Accept": "text/event-stream"},
                method="GET",
            )
            with urllib.request.urlopen(request, timeout=3) as response:
                self.assertEqual(response.status, 200)
                self.assertEqual(response.headers.get_content_type(), "text/event-stream")
                self.assertEqual(response.readline(), b": stream-open\n")

    def test_initialized_notification_returns_202_empty_response(self):
        with _run_server() as server:
            session_id = _initialize_session(server)
            response = _post_notification_initialized(server, session_id)
            self.assertEqual(response.status, 202)
            self.assertEqual(response.read(), b"")

    def test_initialized_notification_marks_session_ready(self):
        with _run_server() as server:
            session_id = _initialize_session(server)
            with _post_notification_initialized(server, session_id) as response:
                self.assertEqual(response.status, 202)

            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                data=(
                    b'{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}'
                ),
                headers={
                    "Content-Type": "application/json",
                    "Mcp-Session-Id": session_id,
                    "MCP-Protocol-Version": "2025-06-18",
                },
                method="POST",
            )
            with urllib.request.urlopen(request) as response:
                body = response.read().decode("utf-8")
                self.assertIn('"result"', body)
                self.assertIn('"tools"', body)

    def test_initialize_returns_updated_protocol_version(self):
        with _run_server() as server:
            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                data=(
                    b'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.1"}}}'
                ),
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with urllib.request.urlopen(request) as response:
                self.assertEqual(response.status, 200)
                payload = response.read().decode("utf-8")
                self.assertIn('"protocolVersion": "2025-06-18"', payload)
                self.assertTrue(response.headers.get("Mcp-Session-Id"))

    def test_non_initialize_request_without_session_uses_recent_client_session(self):
        with _run_server() as server:
            _initialize_session(server)
            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                data=(
                    b'{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}'
                ),
                headers={
                    "Content-Type": "application/json",
                    "MCP-Protocol-Version": "2025-06-18",
                },
                method="POST",
            )
            with urllib.request.urlopen(request) as response:
                self.assertEqual(response.status, 202)

            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                data=(
                    b'{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
                ),
                headers={
                    "Content-Type": "application/json",
                    "MCP-Protocol-Version": "2025-06-18",
                },
                method="POST",
            )
            with urllib.request.urlopen(request) as response:
                body = response.read().decode("utf-8")
                self.assertIn('"result"', body)
                self.assertIn('"tools"', body)

    def test_non_initialize_request_without_any_session_returns_404(self):
        with _run_server() as server:
            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                data=(
                    b'{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
                ),
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with self.assertRaises(urllib.error.HTTPError) as cm:
                urllib.request.urlopen(request)

            response = cm.exception
            self.assertEqual(response.code, 404)
            self.assertIn("Session not found", response.read().decode("utf-8"))

    def test_invalid_protocol_header_returns_400(self):
        with _run_server() as server:
            session_id = _initialize_session(server)
            _post_notification_initialized(server, session_id)
            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                data=(
                    b'{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
                ),
                headers={
                    "Content-Type": "application/json",
                    "Mcp-Session-Id": session_id,
                    "MCP-Protocol-Version": "2024-11-05",
                },
                method="POST",
            )
            with self.assertRaises(urllib.error.HTTPError) as cm:
                urllib.request.urlopen(request)

            response = cm.exception
            self.assertEqual(response.code, 400)
            self.assertIn("Unsupported MCP protocol version", response.read().decode("utf-8"))

    def test_delete_mcp_session_returns_204(self):
        with _run_server() as server:
            session_id = _initialize_session(server)
            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                headers={
                    "Mcp-Session-Id": session_id,
                    "MCP-Protocol-Version": "2025-06-18",
                },
                method="DELETE",
            )
            with urllib.request.urlopen(request) as response:
                self.assertEqual(response.status, 204)
                self.assertEqual(response.read(), b"")


class _ServerContext:
    def __enter__(self):
        _clear_sessions()
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), auto_inspection_mcp.MCPHandler)
        self.server.daemon_threads = True
        self.thread = Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        return self.server

    def __exit__(self, exc_type, exc, tb):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)
        _clear_sessions()


def _run_server():
    return _ServerContext()


def _clear_sessions():
    for session_id in list(auto_inspection_mcp.SESSION_MANAGER._sessions.keys()):
        auto_inspection_mcp.SESSION_MANAGER.remove(session_id)


def _initialize_session(server):
    request = urllib.request.Request(
        f"http://127.0.0.1:{server.server_port}/mcp",
        data=(
            b'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.1"}}}'
        ),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request) as response:
        response.read()
        return response.headers.get("Mcp-Session-Id")


def _post_notification_initialized(server, session_id):
    request = urllib.request.Request(
        f"http://127.0.0.1:{server.server_port}/mcp",
        data=(
            b'{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}'
        ),
        headers={
            "Content-Type": "application/json",
            "Mcp-Session-Id": session_id,
            "MCP-Protocol-Version": "2025-06-18",
        },
        method="POST",
    )
    return urllib.request.urlopen(request)


if __name__ == "__main__":
    unittest.main()
