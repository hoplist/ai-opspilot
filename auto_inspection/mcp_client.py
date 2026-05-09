#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import itertools
import json

import requests


DEFAULT_MCP_URL = "http://127.0.0.1:18081/mcp"
PROTOCOL_VERSION = "2025-06-18"


class MCPClient:
    def __init__(self, mcp_url=DEFAULT_MCP_URL):
        self.mcp_url = str(mcp_url or DEFAULT_MCP_URL).rstrip("/")
        self._session = requests.Session()
        self._session.trust_env = False
        self._ids = itertools.count(1)
        self._initialized = False
        self._protocol_version = None
        self._session_id = None

    def _request(self, payload, *, timeout=60, include_protocol_header=True):
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
        }
        if include_protocol_header and self._protocol_version:
            headers["MCP-Protocol-Version"] = self._protocol_version
        if self._session_id:
            headers["Mcp-Session-Id"] = self._session_id
        response = self._session.post(
            self.mcp_url,
            json=payload,
            timeout=timeout,
            proxies={"http": None, "https": None},
            headers=headers,
        )
        response.raise_for_status()
        if response.status_code == 202 or not response.content:
            return None
        data = response.json()
        if "error" in data and data["error"]:
            message = (data["error"] or {}).get("message") or str(data["error"])
            raise RuntimeError(message)
        return data.get("result")

    def initialize(self):
        if self._initialized:
            return None
        payload = {
            "jsonrpc": "2.0",
            "id": next(self._ids),
            "method": "initialize",
            "params": {
                "protocolVersion": PROTOCOL_VERSION,
                "capabilities": {},
                "clientInfo": {
                    "name": "auto-inspection-skill-client",
                    "version": "0.1.0",
                },
            },
        }
        response = self._session.post(
            self.mcp_url,
            json=payload,
            timeout=30,
            proxies={"http": None, "https": None},
            headers={
                "Content-Type": "application/json",
                "Accept": "application/json, text/event-stream",
            },
        )
        response.raise_for_status()
        data = response.json()
        if "error" in data and data["error"]:
            message = (data["error"] or {}).get("message") or str(data["error"])
            raise RuntimeError(message)
        result = data.get("result")
        self._session_id = str(response.headers.get("Mcp-Session-Id") or "").strip() or None
        self._protocol_version = str((result or {}).get("protocolVersion") or PROTOCOL_VERSION)
        self._request(
            {
                "jsonrpc": "2.0",
                "method": "notifications/initialized",
                "params": {},
            },
            timeout=30,
        )
        self._initialized = True
        return result

    def list_tools(self):
        self.initialize()
        payload = {
            "jsonrpc": "2.0",
            "id": next(self._ids),
            "method": "tools/list",
            "params": {},
        }
        return self._request(payload, timeout=30)

    def call_tool(self, name, arguments=None):
        self.initialize()
        payload = {
            "jsonrpc": "2.0",
            "id": next(self._ids),
            "method": "tools/call",
            "params": {
                "name": name,
                "arguments": arguments or {},
            },
        }
        result = self._request(payload, timeout=120)
        structured = (result or {}).get("structuredContent")
        if structured is not None:
            return structured
        content = (result or {}).get("content") or []
        if content and isinstance(content[0], dict) and "text" in content[0]:
            return json.loads(content[0]["text"])
        return result
