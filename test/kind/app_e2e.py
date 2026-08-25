#!/usr/bin/env python3
"""Bounded black-box checks for a local kubePeep process on a Kind fixture."""

from __future__ import annotations

import argparse
import base64
import hashlib
import http.client
import json
import os
import pathlib
import socket
import struct
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from typing import Any, Callable


class E2EFailure(RuntimeError):
    pass


class HTTPStatusFailure(E2EFailure):
    def __init__(self, status: int, code: str = "unknown") -> None:
        super().__init__(f"stream request returned HTTP {status}/{code}")
        self.status = status
        self.code = code


class Client:
    def __init__(self, origin: str) -> None:
        parsed = urllib.parse.urlsplit(origin)
        if (
            parsed.scheme != "http"
            or parsed.hostname != "127.0.0.1"
            or parsed.username is not None
            or parsed.password is not None
            or parsed.path not in ("", "/")
            or parsed.query
            or parsed.fragment
            or parsed.port is None
            or parsed.port < 1024
            or parsed.port > 65535
        ):
            raise E2EFailure("origin must be an exact uncredentialed http://127.0.0.1:PORT origin")
        self.origin = f"http://127.0.0.1:{parsed.port}"
        self.port = parsed.port
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        self.csrf = ""

    def exchange(
        self,
        method: str,
        path: str,
        *,
        body: dict[str, Any] | None = None,
        accept: str = "application/json",
        csrf: bool = False,
        idempotency_key: str | None = None,
        headers: dict[str, str] | None = None,
        timeout: float = 20,
    ) -> tuple[int, bytes, Any | None]:
        encoded = None if body is None else json.dumps(body, separators=(",", ":")).encode("utf-8")
        request_headers = {"Accept": accept, "Origin": self.origin}
        if encoded is not None:
            request_headers["Content-Type"] = "application/json"
        if csrf:
            if not self.csrf:
                raise E2EFailure("mutation attempted before session/CSRF bootstrap")
            request_headers["X-KubePeep-CSRF"] = self.csrf
        if idempotency_key is not None:
            request_headers["Idempotency-Key"] = idempotency_key
        if headers:
            request_headers.update(headers)
        request = urllib.request.Request(self.origin + path, data=encoded, headers=request_headers, method=method)
        try:
            with self.opener.open(request, timeout=timeout) as response:
                raw = response.read(12 << 20)
                status = response.status
                content_type = response.headers.get("Content-Type", "")
        except urllib.error.HTTPError as error:
            raw = error.read(1 << 20)
            status = error.code
            content_type = error.headers.get("Content-Type", "")
        except (OSError, urllib.error.URLError) as error:
            raise E2EFailure(f"request transport failed for {method} {path}: {type(error).__name__}") from error
        parsed = None
        if content_type.lower().startswith("application/json") and raw:
            try:
                parsed = json.loads(raw)
            except ValueError as error:
                raise E2EFailure(f"{method} {path} returned invalid JSON") from error
        return status, raw, parsed

    def request(
        self,
        method: str,
        path: str,
        *,
        expected: int | tuple[int, ...] = 200,
        **kwargs: Any,
    ) -> tuple[bytes, Any | None]:
        status, raw, parsed = self.exchange(method, path, **kwargs)
        expected_statuses = (expected,) if isinstance(expected, int) else expected
        if status not in expected_statuses:
            code = parsed.get("code", "unknown") if isinstance(parsed, dict) else "unknown"
            raise E2EFailure(f"{method} {path} returned HTTP {status}/{code}; expected {expected_statuses}")
        return raw, parsed

    def data(self, method: str, path: str, **kwargs: Any) -> Any:
        _, payload = self.request(method, path, **kwargs)
        if not isinstance(payload, dict) or "data" not in payload:
            raise E2EFailure(f"{method} {path} did not return the canonical envelope")
        return payload["data"]

    def bootstrap(self) -> tuple[dict[str, Any], dict[str, Any]]:
        status = self.data("GET", "/api/v1/status")
        session = self.data("GET", "/api/v1/session")
        if not isinstance(status, dict) or not isinstance(status.get("selection"), dict):
            raise E2EFailure("status has no active selection")
        if not isinstance(session, dict) or not isinstance(session.get("csrfToken"), str):
            raise E2EFailure("session has no CSRF token")
        if session.get("origin") != self.origin:
            raise E2EFailure("session origin differs from the loopback origin")
        if session.get("generation") != status["selection"].get("generation"):
            raise E2EFailure("status/session generations differ")
        self.csrf = session["csrfToken"]
        return status, session


class SSEStream:
    def __init__(self, client: Client, path: str, *, last_event_id: str = "") -> None:
        self.connection = http.client.HTTPConnection("127.0.0.1", client.port, timeout=90)
        headers = {
            "Accept": "text/event-stream",
            "Origin": client.origin,
            "X-KubePeep-CSRF": client.csrf,
            "Cache-Control": "no-cache",
        }
        if last_event_id:
            headers["Last-Event-ID"] = last_event_id
        self.connection.request("GET", path, headers=headers)
        self.response = self.connection.getresponse()
        if self.response.status != 200:
            raw = self.response.read(1 << 20)
            code = "unknown"
            try:
                payload = json.loads(raw)
                if isinstance(payload, dict):
                    code = str(payload.get("code", "unknown"))
            except (TypeError, ValueError):
                pass
            status = self.response.status
            self.close()
            raise HTTPStatusFailure(status, code)
        if not self.response.headers.get("Content-Type", "").lower().startswith("text/event-stream"):
            self.close()
            raise E2EFailure("stream response did not use text/event-stream")

    def next_event(self, timeout: float) -> dict[str, Any]:
        deadline = time.monotonic() + timeout
        fields: dict[str, Any] = {"dataLines": []}
        while time.monotonic() < deadline:
            remaining = max(0.25, deadline - time.monotonic())
            if self.connection.sock is not None:
                self.connection.sock.settimeout(remaining)
            try:
                raw_line = self.response.readline(1 << 20)
            except (OSError, socket.timeout) as error:
                raise E2EFailure("SSE event deadline expired") from error
            if not raw_line:
                raise E2EFailure("SSE stream ended before the expected event")
            try:
                line = raw_line.decode("utf-8").rstrip("\r\n")
            except UnicodeDecodeError as error:
                raise E2EFailure("SSE stream emitted invalid UTF-8") from error
            if line == "":
                if not fields["dataLines"] and "event" not in fields and "id" not in fields:
                    continue
                data_text = "\n".join(fields.pop("dataLines"))
                if data_text:
                    try:
                        fields["data"] = json.loads(data_text)
                    except ValueError as error:
                        raise E2EFailure("SSE event data was not canonical JSON") from error
                return fields
            if line.startswith(":"):
                continue
            name, separator, value = line.partition(":")
            if separator and value.startswith(" "):
                value = value[1:]
            if name == "data":
                fields["dataLines"].append(value)
            elif name in {"event", "id"}:
                fields[name] = value
        raise E2EFailure("SSE event deadline expired")

    def wait_for(self, predicate: Callable[[dict[str, Any]], bool], timeout: float) -> dict[str, Any]:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            event = self.next_event(max(0.25, deadline - time.monotonic()))
            if predicate(event):
                return event
        raise E2EFailure("SSE stream omitted the expected event")

    def close(self) -> None:
        try:
            self.response.close()
        except (AttributeError, OSError):
            pass
        self.connection.close()


class RawWebSocket:
    def __init__(self, connection: socket.socket, buffered: bytes = b"") -> None:
        self.connection = connection
        self.buffered = bytearray(buffered)

    @staticmethod
    def open(client: Client, path: str, protocols: list[str]) -> tuple[int, RawWebSocket | None]:
        if not path.startswith("/api/v1/exec/") or "\r" in path or "\n" in path:
            raise E2EFailure("exec ticket returned an invalid WebSocket path")
        connection = socket.create_connection(("127.0.0.1", client.port), timeout=30)
        key = base64.b64encode(os.urandom(16)).decode("ascii")
        request = (
            f"GET {path} HTTP/1.1\r\n"
            f"Host: 127.0.0.1:{client.port}\r\n"
            "Upgrade: websocket\r\n"
            "Connection: Upgrade\r\n"
            f"Sec-WebSocket-Key: {key}\r\n"
            "Sec-WebSocket-Version: 13\r\n"
            f"Origin: {client.origin}\r\n"
            f"Sec-WebSocket-Protocol: {', '.join(protocols)}\r\n"
            "\r\n"
        ).encode("ascii")
        connection.sendall(request)
        response = bytearray()
        while b"\r\n\r\n" not in response and len(response) <= 16384:
            chunk = connection.recv(4096)
            if not chunk:
                break
            response.extend(chunk)
        if b"\r\n\r\n" not in response:
            connection.close()
            raise E2EFailure("WebSocket handshake returned incomplete headers")
        head, buffered = bytes(response).split(b"\r\n\r\n", 1)
        lines = head.split(b"\r\n")
        try:
            status = int(lines[0].split(b" ", 2)[1])
        except (IndexError, ValueError) as error:
            connection.close()
            raise E2EFailure("WebSocket handshake returned an invalid status line") from error
        headers: dict[str, str] = {}
        for line in lines[1:]:
            name, separator, value = line.partition(b":")
            if not separator:
                continue
            headers[name.decode("ascii").lower()] = value.decode("ascii").strip()
        if status != 101:
            connection.close()
            return status, None
        expected_accept = base64.b64encode(
            hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode("ascii")).digest()
        ).decode("ascii")
        if headers.get("sec-websocket-accept") != expected_accept:
            connection.close()
            raise E2EFailure("WebSocket handshake returned an invalid accept proof")
        if headers.get("sec-websocket-protocol") != protocols[0]:
            connection.close()
            raise E2EFailure("WebSocket exposed or selected an unexpected subprotocol")
        return status, RawWebSocket(connection, buffered)

    def _read_exact(self, size: int) -> bytes:
        result = bytearray()
        if self.buffered:
            take = min(size, len(self.buffered))
            result.extend(self.buffered[:take])
            del self.buffered[:take]
        while len(result) < size:
            chunk = self.connection.recv(size - len(result))
            if not chunk:
                raise E2EFailure("WebSocket closed before a complete frame")
            result.extend(chunk)
        return bytes(result)

    def read_frame(self, timeout: float) -> tuple[int, bool, bytes]:
        self.connection.settimeout(timeout)
        first, second = self._read_exact(2)
        fin = bool(first & 0x80)
        opcode = first & 0x0F
        if first & 0x70 or second & 0x80:
            raise E2EFailure("WebSocket server emitted an invalid frame")
        size = second & 0x7F
        if size == 126:
            size = struct.unpack("!H", self._read_exact(2))[0]
        elif size == 127:
            size = struct.unpack("!Q", self._read_exact(8))[0]
        if size > (1 << 20):
            raise E2EFailure("WebSocket server frame exceeded the E2E bound")
        return opcode, fin, self._read_exact(size)

    def send_frame(self, opcode: int, payload: bytes = b"") -> None:
        if len(payload) > (1 << 20):
            raise E2EFailure("WebSocket client frame exceeded the E2E bound")
        header = bytearray([0x80 | opcode])
        size = len(payload)
        if size < 126:
            header.append(0x80 | size)
        elif size <= 0xFFFF:
            header.append(0x80 | 126)
            header.extend(struct.pack("!H", size))
        else:
            header.append(0x80 | 127)
            header.extend(struct.pack("!Q", size))
        mask = os.urandom(4)
        masked = bytes(value ^ mask[index % 4] for index, value in enumerate(payload))
        self.connection.sendall(bytes(header) + mask + masked)

    def close(self) -> None:
        try:
            self.connection.close()
        except OSError:
            pass


def target(selection: dict[str, Any], namespace: str, kind: str, name: str) -> dict[str, Any]:
    return {
        "clusterProfileId": selection["clusterProfileId"],
        "context": selection["context"],
        "namespace": namespace,
        "kind": kind,
        "name": name,
    }


def confirmation(
    selection: dict[str, Any], namespace: str, kind: str, name: str, action: str, consequence: str
) -> dict[str, Any]:
    return {
        "confirmed": True,
        "action": action,
        "consequenceCode": consequence,
        "target": target(selection, namespace, kind, name),
        "expectedGeneration": selection["generation"],
    }


def assert_no_secret_payload(value: Any) -> None:
    forbidden = {"data", "stringData", "annotations", "managedFields"}

    def walk(item: Any) -> None:
        if isinstance(item, dict):
            leaked = forbidden.intersection(item)
            if leaked:
                raise E2EFailure(f"Secret DTO exposed forbidden fields: {sorted(leaked)}")
            for nested in item.values():
                walk(nested)
        elif isinstance(item, list):
            for nested in item:
                walk(nested)

    walk(value)


def metadata(detail: Any) -> dict[str, Any]:
    if not isinstance(detail, dict) or not isinstance(detail.get("metadata"), dict):
        raise E2EFailure("resource detail omitted metadata")
    return detail["metadata"]


def api_error_code(payload: Any) -> str:
    return str(payload.get("code", "unknown")) if isinstance(payload, dict) else "unknown"


def select_context(client: Client, status: dict[str, Any]) -> dict[str, Any]:
    selection = status["selection"]
    profile_id = selection.get("clusterProfileId")
    context_name = selection.get("context")
    if type(profile_id) is not int or profile_id <= 0 or not isinstance(context_name, str) or not context_name:
        raise E2EFailure("active cluster profile/context is invalid")
    contexts = client.data("GET", "/api/v1/contexts?" + urllib.parse.urlencode({"clusterProfileId": profile_id}))
    if not isinstance(contexts, list) or not any(
        isinstance(item, dict) and item.get("name") == context_name for item in contexts
    ):
        raise E2EFailure("context catalog omitted the active context")
    selected = client.data(
        "POST",
        "/api/v1/contexts/select",
        body={
            "clusterProfileId": profile_id,
            "context": context_name,
            "setDefault": False,
            "expectedGeneration": selection["generation"],
        },
        csrf=True,
    )
    if not isinstance(selected, dict) or selected.get("context") != context_name:
        raise E2EFailure("context selection did not return the selected context")
    refreshed, _ = client.bootstrap()
    return refreshed


def create_scope(client: Client, status: dict[str, Any], name: str, mode: str, namespaces: list[str], default: str | None) -> dict[str, Any]:
    return client.data(
        "POST",
        "/api/v1/namespace-scopes",
        body={
            "name": name,
            "mode": mode,
            "namespaces": namespaces,
            "defaultNamespace": default,
            "expectedGeneration": status["selection"]["generation"],
        },
        expected=201,
        csrf=True,
    )


def select_scope(client: Client, status: dict[str, Any], scope: dict[str, Any]) -> dict[str, Any]:
    scope_id = scope.get("id")
    if type(scope_id) is not int or scope_id <= 0:
        raise E2EFailure("created namespace scope omitted its id")
    selected = client.data(
        "POST",
        f"/api/v1/namespace-scopes/{scope_id}/select",
        body={"expectedGeneration": status["selection"]["generation"]},
        csrf=True,
    )
    if not isinstance(selected, dict) or selected.get("scopeId") != scope_id:
        raise E2EFailure("namespace scope selection returned an unexpected scope")
    refreshed, _ = client.bootstrap()
    return refreshed


def assert_dashboard_full(client: Client) -> None:
    block = client.data("GET", "/api/v1/dashboard/summary")
    if not isinstance(block, dict) or block.get("complete") is not True or block.get("errors") != []:
        raise E2EFailure("dashboard full state was not complete")
    values = block.get("value")
    required = {"namespaces", "podsTotal", "podsHealthy", "podsProblematic", "workloadsDegraded", "restarts", "warningEvents"}
    if not isinstance(values, dict) or not required.issubset(values):
        raise E2EFailure("dashboard full state omitted canonical counters")
    if any(not isinstance(values[key], dict) or values[key].get("value") is None for key in required):
        raise E2EFailure("dashboard full state marked a canonical counter unavailable")
    problems = client.data("GET", "/api/v1/dashboard/problems?limit=100")
    events = client.data("GET", "/api/v1/dashboard/events?limit=100")
    if not isinstance(problems, dict) or not isinstance(problems.get("value"), list):
        raise E2EFailure("dashboard problems block is invalid")
    if not isinstance(events, dict) or not any(
        isinstance(item, dict) and item.get("objectName") == "kp-restarting"
        for item in events.get("value", [])
    ):
        raise E2EFailure("dashboard events omitted the Warning fixture")


def assert_dashboard_partial(client: Client) -> None:
    block = client.data("GET", "/api/v1/dashboard/summary")
    if not isinstance(block, dict) or block.get("complete") is not False:
        raise E2EFailure("dashboard partial state incorrectly reported completeness")
    errors = block.get("errors")
    if not isinstance(errors, list) or not any(
        isinstance(item, dict) and item.get("namespace") == "kp-denied" and item.get("code") == "FORBIDDEN"
        for item in errors
    ):
        raise E2EFailure("dashboard partial state did not isolate the denied namespace")
    values = block.get("value")
    if (
        not isinstance(values, dict)
        or not isinstance(values.get("namespaces"), dict)
        or values["namespaces"].get("value") != 2
    ):
        raise E2EFailure("dashboard partial state discarded safe values")


def exercise_manual_scopes(client: Client, status: dict[str, Any]) -> dict[str, Any]:
    status = select_context(client, status)
    single = create_scope(client, status, "e2e-single", "single", ["kp-allowed"], "kp-allowed")
    status = select_scope(client, status, single)
    selection = status["selection"]
    if selection.get("scopeMode") != "single" or selection.get("namespaceCount") != 1:
        raise E2EFailure("single namespace scope resolved incorrectly")
    listed = create_scope(client, status, "e2e-list", "list", ["kp-allowed", "kp-denied"], "kp-allowed")
    status = select_scope(client, status, listed)
    selection = status["selection"]
    if selection.get("scopeMode") != "list" or selection.get("namespaceCount") != 2:
        raise E2EFailure("list namespace scope resolved incorrectly")
    assert_dashboard_partial(client)
    status = select_scope(client, status, single)
    _, denied = client.request(
        "POST",
        "/api/v1/namespace-scopes",
        body={
            "name": "e2e-all-denied",
            "mode": "all",
            "namespaces": [],
            "defaultNamespace": None,
            "expectedGeneration": status["selection"]["generation"],
        },
        expected=403,
        csrf=True,
    )
    if api_error_code(denied) != "FORBIDDEN":
        raise E2EFailure("all scope without namespace LIST did not fail closed")
    return status


def check_all_scope(client: Client, status: dict[str, Any]) -> None:
    status = select_context(client, status)
    scope = create_scope(client, status, "e2e-all-authorized", "all", [], None)
    if scope.get("namespaces") != []:
        raise E2EFailure("all scope persisted namespace items")
    status = select_scope(client, status, scope)
    selection = status["selection"]
    if selection.get("scopeMode") != "all" or type(selection.get("namespaceCount")) is not int or selection["namespaceCount"] < 3:
        raise E2EFailure("authorized all scope did not resolve the cluster namespace catalog")
    namespaces = client.data("GET", "/api/v1/namespaces?limit=100")
    names = {item.get("name") for item in namespaces if isinstance(item, dict)} if isinstance(namespaces, list) else set()
    if not {"kp-allowed", "kp-denied", "kp-harness"}.issubset(names):
        raise E2EFailure("authorized namespace catalog omitted Kind fixtures")


def scale_deployment(client: Client, status: dict[str, Any], replicas: int) -> None:
    selection = status["selection"]
    namespace = "kp-allowed"
    path = f"/api/v1/workloads/deployments/{namespace}/kp-action-deployment"
    deployment = metadata(client.data("GET", path))
    body = confirmation(selection, namespace, "Deployment", "kp-action-deployment", "scale", "CHANGE_REPLICA_COUNT")
    body.update({"replicas": replicas, "expectedResourceVersion": deployment["resourceVersion"]})
    client.request("PUT", path + "/scale", body=body, csrf=True)


def wait_deployment_desired(client: Client, replicas: int, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    path = "/api/v1/workloads/deployments/kp-allowed/kp-action-deployment"
    while time.monotonic() < deadline:
        detail = client.data("GET", path)
        if isinstance(detail, dict) and detail.get("desired") == replicas:
            return
        time.sleep(0.25)
    raise E2EFailure(f"deployment did not converge to desired replica count {replicas}")


def event_names_object(event: dict[str, Any], name: str, *, desired: int | None = None) -> bool:
    data = event.get("data")
    if not isinstance(data, dict):
        return False
    value = data.get("object")
    if not isinstance(value, dict):
        return False
    matches_name = value.get("name") == name or (
        isinstance(value.get("metadata"), dict) and value["metadata"].get("name") == name
    )
    return matches_name and (desired is None or value.get("desired") == desired)


def check_resource_sse_replay(client: Client, status: dict[str, Any]) -> None:
    stream = SSEStream(client, "/api/v1/stream?topic=workloads")
    try:
        stream.wait_for(
            lambda event: event.get("event") == "snapshot"
            and isinstance(event.get("data"), dict)
            and event["data"].get("topic") == "workloads"
            and event["data"].get("final") is True,
            30,
        )
        scale_deployment(client, status, 2)
        changed = stream.wait_for(
            lambda event: event.get("event") in {"added", "modified"}
            and event_names_object(event, "kp-action-deployment", desired=2),
            30,
        )
        last_id = changed.get("id")
        if not isinstance(last_id, str) or not last_id.startswith("kpse1."):
            raise E2EFailure("live resource event omitted its opaque replay id")
    finally:
        stream.close()
    scale_deployment(client, status, 1)
    wait_deployment_desired(client, 1, 10)
    # Keep the reconnect inside the 30-second replay retention while allowing
    # the detached watch session to append the confirmed update to its ring.
    time.sleep(1)
    resumed = SSEStream(client, "/api/v1/stream?topic=workloads", last_event_id=last_id)
    try:
        deadline = time.monotonic() + 20
        replayed: dict[str, Any] | None = None
        while time.monotonic() < deadline:
            candidate = resumed.next_event(max(0.25, deadline - time.monotonic()))
            if candidate.get("event") == "snapshot":
                raise E2EFailure("resource stream resumed with a snapshot instead of replay")
            if candidate.get("event") in {"added", "modified"} and event_names_object(
                candidate, "kp-action-deployment", desired=1
            ):
                replayed = candidate
                break
        if replayed is None:
            raise E2EFailure("resource stream replay omitted the disconnected update")
        if replayed.get("id") == last_id:
            raise E2EFailure("resource stream replay did not advance continuity")
    finally:
        resumed.close()


def check_log_follow(client: Client) -> None:
    stream = SSEStream(client, "/api/v1/pods/kp-allowed/kp-interactive/logs/stream?container=web&tailLines=20")
    try:
        stream.wait_for(lambda event: event.get("event") == "meta", 10)
        line = stream.wait_for(lambda event: event.get("event") == "line", 20)
        payload = line.get("data")
        if not isinstance(payload, dict) or "synthetic current log ready" not in str(payload.get("text", "")):
            raise E2EFailure("log follow omitted the canonical current line")
    finally:
        stream.close()


def create_exec_ticket(client: Client, status: dict[str, Any], command: list[str]) -> dict[str, Any]:
    selection = status["selection"]
    body = confirmation(selection, "kp-allowed", "Pod", "kp-interactive", "exec", "OPEN_INTERACTIVE_PROCESS")
    body.update({"container": "utility", "command": command, "tty": False, "stdin": False})
    ticket = client.data(
        "POST", "/api/v1/pods/kp-allowed/kp-interactive/exec", body=body, expected=201, csrf=True
    )
    if (
        not isinstance(ticket, dict)
        or not isinstance(ticket.get("sessionId"), str)
        or not isinstance(ticket.get("websocketUrl"), str)
        or not isinstance(ticket.get("protocols"), list)
        or len(ticket["protocols"]) != 2
        or ticket["protocols"][0] != "kubepeep.exec.v1"
        or not isinstance(ticket["protocols"][1], str)
        or not ticket["protocols"][1].startswith("kubepeep.exec.ticket.")
    ):
        raise E2EFailure("exec ticket did not use the public ephemeral protocol contract")
    return ticket


def check_exec_websocket(client: Client, status: dict[str, Any]) -> str:
    ticket = create_exec_ticket(
        client, status, ["/bin/sh", "-c", "sleep 17; printf kp-exec-stdout; printf kp-exec-stderr >&2"]
    )
    protocols = ticket["protocols"]
    handshake_status, websocket = RawWebSocket.open(client, ticket["websocketUrl"], protocols)
    if handshake_status != 101 or websocket is None:
        raise E2EFailure("authorized exec ticket did not upgrade")
    ready = False
    heartbeat = False
    terminal = False
    stdout = bytearray()
    stderr = bytearray()
    fragmented_opcode: int | None = None
    fragmented = bytearray()
    deadline = time.monotonic() + 40
    try:
        while time.monotonic() < deadline:
            opcode, final, payload = websocket.read_frame(max(0.25, deadline - time.monotonic()))
            if opcode == 0x9:
                websocket.send_frame(0xA, payload)
                continue
            if opcode == 0xA:
                continue
            if opcode == 0x8:
                websocket.send_frame(0x8, payload[:125])
                break
            if opcode in {0x1, 0x2} and not final:
                fragmented_opcode = opcode
                fragmented = bytearray(payload)
                continue
            if opcode == 0x0:
                if fragmented_opcode is None:
                    raise E2EFailure("exec WebSocket emitted an orphan continuation")
                fragmented.extend(payload)
                if not final:
                    continue
                opcode, payload = fragmented_opcode, bytes(fragmented)
                fragmented_opcode = None
                fragmented = bytearray()
            if opcode == 0x2:
                if not payload or payload[0] not in {0x01, 0x02}:
                    raise E2EFailure("exec WebSocket emitted an invalid output channel")
                (stdout if payload[0] == 0x01 else stderr).extend(payload[1:])
                continue
            if opcode != 0x1:
                raise E2EFailure("exec WebSocket emitted an unsupported frame")
            try:
                control = json.loads(payload)
            except (TypeError, ValueError) as error:
                raise E2EFailure("exec WebSocket emitted invalid control JSON") from error
            if not isinstance(control, dict):
                raise E2EFailure("exec WebSocket emitted an invalid control object")
            if control.get("type") == "ready":
                ready = control.get("sessionId") == ticket["sessionId"] and control.get("stdin") is False
            elif control.get("type") == "heartbeat":
                nonce = control.get("nonce")
                if not isinstance(nonce, str) or not nonce:
                    raise E2EFailure("exec heartbeat omitted its nonce")
                websocket.send_frame(
                    0x1, json.dumps({"type": "heartbeat", "nonce": nonce}, separators=(",", ":")).encode()
                )
                heartbeat = True
            elif control.get("type") == "exit":
                terminal = control.get("exitCode") == 0 and control.get("reason") == "completed"
            elif control.get("type") == "error":
                raise E2EFailure("exec WebSocket terminated with a public error")
        if not ready or not heartbeat or not terminal:
            raise E2EFailure("exec WebSocket omitted ready, heartbeat, or successful terminal control")
        if b"kp-exec-stdout" not in stdout or b"kp-exec-stderr" not in stderr:
            raise E2EFailure("exec WebSocket did not preserve stdout/stderr channels")
    finally:
        websocket.close()
    reused_status, reused = RawWebSocket.open(client, ticket["websocketUrl"], protocols)
    if reused is not None:
        reused.close()
    if reused_status != 410:
        raise E2EFailure(f"consumed exec ticket returned HTTP {reused_status} instead of 410")
    return protocols[1]


def mark(control_dir: pathlib.Path, name: str) -> None:
    path = control_dir / name
    temporary = control_dir / (name + ".tmp")
    temporary.write_text("ready\n", encoding="ascii")
    temporary.replace(path)


def wait_marker(control_dir: pathlib.Path, name: str, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    path = control_dir / name
    while time.monotonic() < deadline:
        if path.is_file():
            return
        time.sleep(0.2)
    raise E2EFailure(f"harness did not publish bounded control marker {name}")


def wait_http_status(client: Client, method: str, path: str, expected: int, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        status, _, _ = client.exchange(method, path, timeout=10)
        if status == expected:
            return
        time.sleep(0.5)
    raise E2EFailure(f"{method} {path} did not converge to HTTP {expected}")


def assert_values_absent(root: pathlib.Path, values: list[str]) -> None:
    needles = [value.encode("utf-8") for value in values if value]
    for path in root.rglob("*"):
        try:
            if not path.is_file() or path.is_symlink() or path.stat().st_size > (64 << 20):
                continue
            contents = path.read_bytes()
        except OSError as error:
            raise E2EFailure("could not inspect app state for ephemeral values") from error
        if any(needle in contents for needle in needles):
            raise E2EFailure("app output/state persisted an ephemeral token or raw log line")


def check_periodic_revocation(
    client: Client,
    status: dict[str, Any],
    control_dir: pathlib.Path,
    scan_root: pathlib.Path,
    ephemeral_protocol: str,
) -> None:
    resource_stream = SSEStream(client, "/api/v1/stream?topic=pods")
    log_stream = SSEStream(client, "/api/v1/pods/kp-allowed/kp-interactive/logs/stream?container=web&tailLines=20")
    try:
        resource_stream.wait_for(lambda event: event.get("event") == "snapshot", 30)
        log_stream.wait_for(lambda event: event.get("event") == "meta", 10)
        log_stream.wait_for(lambda event: event.get("event") == "line", 20)
        mark(control_dir, "f6-ready")
        wait_marker(control_dir, "f6-revoked", 30)
        resource_error = resource_stream.wait_for(lambda event: event.get("event") == "error", 85)
        log_error = log_stream.wait_for(lambda event: event.get("event") == "error", 20)
        for event in (resource_error, log_error):
            payload = event.get("data")
            if not isinstance(payload, dict) or str(payload.get("code", "")).upper() != "FORBIDDEN":
                raise E2EFailure("periodic stream reauthorization did not fail closed as forbidden")
        # The stream reauthorization deliberately bypasses the ordinary read
        # cache. Once it has observed revocation, the product endpoint must use
        # that denied capability as well.
        wait_http_status(client, "GET", "/api/v1/workloads?limit=1", 403, 10)
    finally:
        resource_stream.close()
        log_stream.close()
    mark(control_dir, "f6-done")
    wait_marker(control_dir, "f6-restored", 30)
    pending_ticket = create_exec_ticket(client, status, ["/bin/true"])
    mark(control_dir, "exec-ready")
    wait_marker(control_dir, "exec-revoked", 30)
    body = confirmation(
        status["selection"], "kp-allowed", "Pod", "kp-interactive", "exec", "OPEN_INTERACTIVE_PROCESS"
    )
    body.update({"container": "utility", "command": ["/bin/true"], "tty": False, "stdin": False})
    _, denied = client.request(
        "POST", "/api/v1/pods/kp-allowed/kp-interactive/exec", body=body, expected=403, csrf=True
    )
    if api_error_code(denied) != "FORBIDDEN":
        raise E2EFailure("new product exec action did not observe RBAC revocation")
    denied_status, denied_socket = RawWebSocket.open(
        client, pending_ticket["websocketUrl"], pending_ticket["protocols"]
    )
    if denied_socket is not None:
        denied_socket.close()
    if denied_status != 403:
        raise E2EFailure("existing exec ticket did not reauthorize before WebSocket upgrade")
    assert_values_absent(
        scan_root,
        [
            client.csrf,
            ephemeral_protocol,
            pending_ticket["protocols"][1],
            "synthetic current log ready",
            "synthetic dashboard fixture unavailable",
            "kp-exec-stdout",
            "kp-exec-stderr",
        ],
    )
    mark(control_dir, "exec-done")


def check_allowed(client: Client, status: dict[str, Any], args: argparse.Namespace) -> None:
    status = exercise_manual_scopes(client, status)
    selection = status["selection"]
    namespace = "kp-allowed"
    if selection.get("defaultNamespace") != namespace:
        raise E2EFailure("allowed run selected an unexpected namespace")
    assert_dashboard_full(client)
    permission_query = urllib.parse.urlencode(
        {"namespace": namespace, "capability": "deployments.restart", "resourceName": "kp-action-deployment"}
    )
    client.data("GET", f"/api/v1/permissions?{permission_query}")
    collection_expectations = (
        ("/api/v1/workloads?limit=100", "name", {"kp-action-deployment", "kp-action-statefulset", "kp-daemonset", "kp-job", "kp-cronjob"}),
        ("/api/v1/pods?limit=100", "name", {"kp-interactive", "kp-delete-probe", "kp-restarting"}),
        ("/api/v1/events?limit=100", "objectName", {"kp-restarting"}),
        ("/api/v1/services?limit=100", "name", {"kp-service", "kp-headless"}),
        ("/api/v1/ingresses?limit=100", "name", {"kp-ingress"}),
        ("/api/v1/endpoint-slices?limit=100", "name", {"kp-service-v1"}),
        ("/api/v1/configmaps?limit=100", "name", {"kp-config"}),
    )
    for path, field, expected_values in collection_expectations:
        value = client.data("GET", path)
        if not isinstance(value, list):
            raise E2EFailure(f"{path} did not return a collection")
        actual_values = {item.get(field) for item in value if isinstance(item, dict)}
        if not expected_values.issubset(actual_values):
            raise E2EFailure(f"{path} omitted canonical fixtures")
    secrets = client.data("GET", "/api/v1/secrets?limit=100")
    if not isinstance(secrets, list):
        raise E2EFailure("Secret list did not return a collection")
    assert_no_secret_payload(secrets)
    if not any(
        isinstance(item, dict)
        and isinstance(item.get("metadata"), dict)
        and item["metadata"].get("name") == "kp-secret-metadata"
        for item in secrets
    ):
        raise E2EFailure("Secret list omitted the canonical metadata-only fixture")
    secret = client.data("GET", f"/api/v1/secrets/{namespace}/kp-secret-metadata")
    assert_no_secret_payload(secret)
    if metadata(secret).get("name") != "kp-secret-metadata":
        raise E2EFailure("Secret detail returned unexpected metadata")
    detail_paths = (
        f"/api/v1/workloads/deployments/{namespace}/kp-action-deployment",
        f"/api/v1/workloads/statefulsets/{namespace}/kp-action-statefulset",
        f"/api/v1/workloads/daemonsets/{namespace}/kp-daemonset",
        f"/api/v1/workloads/jobs/{namespace}/kp-job",
        f"/api/v1/workloads/cronjobs/{namespace}/kp-cronjob",
        f"/api/v1/pods/{namespace}/kp-interactive",
        f"/api/v1/services/{namespace}/kp-service",
        f"/api/v1/ingresses/{namespace}/kp-ingress",
        f"/api/v1/endpoint-slices/{namespace}/kp-service-v1",
    )
    for path in detail_paths:
        metadata(client.data("GET", path))
        client.request("GET", path + "/yaml", accept="application/yaml, text/yaml")
    config_path = f"/api/v1/configmaps/{namespace}/kp-config"
    config = client.data("GET", config_path)
    metadata(config)
    entries = config.get("entries") if isinstance(config, dict) else None
    if not isinstance(entries, list) or not any(
        isinstance(entry, dict) and entry.get("key") == "mode" and entry.get("value") == "canonical"
        for entry in entries
    ):
        raise E2EFailure("ConfigMap detail omitted canonical on-demand data")
    client.request("GET", config_path + "/yaml", accept="application/yaml, text/yaml")
    current_logs = client.data("GET", f"/api/v1/pods/{namespace}/kp-interactive/logs?container=web&tailLines=20")
    previous_logs = client.data(
        "GET", f"/api/v1/pods/{namespace}/kp-restarting/logs?container=synthetic-log&tailLines=20&previous=true"
    )
    current_lines = current_logs.get("lines") if isinstance(current_logs, dict) else None
    previous_lines = previous_logs.get("lines") if isinstance(previous_logs, dict) else None
    if not isinstance(current_lines, list) or not any(
        isinstance(line, dict) and "synthetic current log ready" in str(line.get("text", ""))
        for line in current_lines
    ):
        raise E2EFailure("current log response omitted the canonical line")
    if (
        not isinstance(previous_logs, dict)
        or previous_logs.get("previous") is not True
        or not isinstance(previous_lines, list)
        or not any(
            isinstance(line, dict) and "synthetic dashboard fixture unavailable" in str(line.get("text", ""))
            for line in previous_lines
        )
    ):
        raise E2EFailure("previous log response omitted the canonical line")
    check_resource_sse_replay(client, status)
    check_log_follow(client)
    deployment_path = f"/api/v1/workloads/deployments/{namespace}/kp-action-deployment"
    deployment = metadata(client.data("GET", deployment_path))
    restart = confirmation(
        selection, namespace, "Deployment", "kp-action-deployment", "restart", "RECREATE_WORKLOAD_PODS"
    )
    restart["expectedResourceVersion"] = deployment["resourceVersion"]
    _, csrf_rejection = client.request(
        "POST",
        deployment_path + "/restart",
        body=restart,
        expected=403,
        idempotency_key="kp-e2e-no-csrf-" + uuid.uuid4().hex,
    )
    if not isinstance(csrf_rejection, dict) or csrf_rejection.get("code") != "CSRF_REJECTED":
        raise E2EFailure("mutation without CSRF did not return CSRF_REJECTED")
    client.request(
        "POST",
        deployment_path + "/restart",
        body=restart,
        expected=202,
        csrf=True,
        idempotency_key="kp-e2e-restart-" + uuid.uuid4().hex,
    )
    scale_deployment(client, status, 2)
    scale_deployment(client, status, 1)
    port_forward = confirmation(
        selection, namespace, "Pod", "kp-interactive", "portForward", "EXPOSE_POD_PORT_LOCALLY"
    )
    port_forward.update({"remotePort": 8080, "localPort": None})
    session = client.data(
        "POST",
        f"/api/v1/pods/{namespace}/kp-interactive/port-forward",
        body=port_forward,
        expected=201,
        csrf=True,
        idempotency_key="kp-e2e-pf-" + uuid.uuid4().hex,
    )
    if (
        not isinstance(session, dict)
        or session.get("localAddress") != "127.0.0.1"
        or not isinstance(session.get("id"), str)
        or not session["id"]
        or type(session.get("localPort")) is not int
        or not 1024 <= session["localPort"] <= 65535
    ):
        raise E2EFailure("port-forward response did not bind exact loopback")
    try:
        with socket.create_connection(("127.0.0.1", session["localPort"]), timeout=5) as connection:
            connection.sendall(b"GET / HTTP/1.0\r\nHost: localhost\r\n\r\n")
            response_head = b""
            while b"\r\n\r\n" not in response_head and len(response_head) < 4096:
                chunk = connection.recv(4096 - len(response_head))
                if not chunk:
                    break
                response_head += chunk
            if b"200 OK" not in response_head:
                raise E2EFailure("port-forward did not carry an HTTP response")
        active = client.data("GET", "/api/v1/port-forwards")
        if not isinstance(active, list) or not any(
            isinstance(item, dict) and item.get("id") == session.get("id") for item in active
        ):
            raise E2EFailure("active port-forward was absent from the session registry")
    finally:
        if isinstance(session, dict) and isinstance(session.get("id"), str):
            client.request(
                "DELETE",
                "/api/v1/port-forwards/" + urllib.parse.quote(session["id"], safe=""),
                body={"confirmed": True, "expectedGeneration": selection["generation"]},
                expected=204,
                csrf=True,
            )
    ephemeral_protocol = check_exec_websocket(client, status)
    pod_path = f"/api/v1/pods/{namespace}/kp-delete-probe"
    pod = metadata(client.data("GET", pod_path))
    delete = confirmation(selection, namespace, "Pod", "kp-delete-probe", "deletePod", "DELETE_POD")
    delete.update({"expectedUid": pod["uid"], "expectedResourceVersion": pod["resourceVersion"]})
    client.request("DELETE", pod_path, body=delete, expected=202, csrf=True)
    if args.control_dir is None or args.scan_root is None:
        raise E2EFailure("allowed mode requires bounded revocation control and scan roots")
    check_periodic_revocation(client, status, args.control_dir, args.scan_root, ephemeral_protocol)


def check_denied(client: Client, status: dict[str, Any], args: argparse.Namespace) -> None:
    selection = status["selection"]
    namespace = "kp-denied"
    if selection.get("defaultNamespace") != namespace:
        raise E2EFailure("denied run selected an unexpected namespace")
    denied_reads = (
        "/api/v1/workloads?limit=1",
        "/api/v1/pods?limit=1",
        f"/api/v1/pods/{namespace}/kp-interactive",
        f"/api/v1/pods/{namespace}/kp-interactive/yaml",
        f"/api/v1/pods/{namespace}/kp-interactive/logs?container=utility&tailLines=1",
        "/api/v1/secrets?limit=1",
        "/api/v1/dashboard/summary",
    )
    for path in denied_reads:
        _, payload = client.request("GET", path, expected=403)
        if api_error_code(payload) != "FORBIDDEN":
            raise E2EFailure(f"denied product read did not return FORBIDDEN: {path}")
    _, stream_denied = client.request(
        "GET", "/api/v1/stream?topic=pods", accept="text/event-stream", csrf=True, expected=403
    )
    if api_error_code(stream_denied) != "FORBIDDEN":
        raise E2EFailure("denied resource stream did not fail before opening")
    cases: tuple[tuple[str, str, dict[str, Any], str | None], ...] = (
        (
            "POST",
            f"/api/v1/workloads/deployments/{namespace}/kp-action-deployment/restart",
            {
                **confirmation(selection, namespace, "Deployment", "kp-action-deployment", "restart", "RECREATE_WORKLOAD_PODS"),
                "expectedResourceVersion": args.deployment_rv,
            },
            "kp-e2e-denied-restart-" + uuid.uuid4().hex,
        ),
        (
            "PUT",
            f"/api/v1/workloads/deployments/{namespace}/kp-action-deployment/scale",
            {
                **confirmation(selection, namespace, "Deployment", "kp-action-deployment", "scale", "CHANGE_REPLICA_COUNT"),
                "replicas": 2,
                "expectedResourceVersion": args.deployment_rv,
            },
            None,
        ),
        (
            "DELETE",
            f"/api/v1/pods/{namespace}/kp-delete-probe",
            {
                **confirmation(selection, namespace, "Pod", "kp-delete-probe", "deletePod", "DELETE_POD"),
                "expectedUid": args.pod_uid,
                "expectedResourceVersion": args.pod_rv,
            },
            None,
        ),
        (
            "POST",
            f"/api/v1/pods/{namespace}/kp-interactive/port-forward",
            {
                **confirmation(selection, namespace, "Pod", "kp-interactive", "portForward", "EXPOSE_POD_PORT_LOCALLY"),
                "remotePort": 8080,
                "localPort": None,
            },
            "kp-e2e-denied-pf-" + uuid.uuid4().hex,
        ),
        (
            "POST",
            f"/api/v1/pods/{namespace}/kp-interactive/exec",
            {
                **confirmation(selection, namespace, "Pod", "kp-interactive", "exec", "OPEN_INTERACTIVE_PROCESS"),
                "container": "utility",
                "command": ["/bin/true"],
                "tty": False,
                "stdin": False,
            },
            None,
        ),
    )
    for method, path, body, key in cases:
        _, payload = client.request(method, path, body=body, expected=403, csrf=True, idempotency_key=key)
        if api_error_code(payload) != "FORBIDDEN":
            raise E2EFailure(f"denied product action did not return FORBIDDEN: {method} {path}")


def check_offline(client: Client) -> None:
    status, _, payload = client.exchange("GET", "/api/v1/dashboard/summary", timeout=25)
    if status == 200:
        block = payload.get("data") if isinstance(payload, dict) else None
        errors = block.get("errors") if isinstance(block, dict) else None
        if block.get("complete") is not False or not isinstance(errors, list) or not errors:
            raise E2EFailure("offline dashboard did not expose its partial unavailable state")
        if any(
            not isinstance(item, dict)
            or item.get("code") not in {"CLUSTER_UNAVAILABLE", "AUTHENTICATION_UNAVAILABLE", "AUTHORIZATION_UNAVAILABLE", "UPSTREAM_TIMEOUT"}
            for item in errors
        ):
            raise E2EFailure("offline dashboard returned an unexpected safe partial error")
    elif status not in {503, 504} or api_error_code(payload) not in {
        "CLUSTER_UNAVAILABLE",
        "AUTHENTICATION_UNAVAILABLE",
        "AUTHORIZATION_UNAVAILABLE",
        "UPSTREAM_TIMEOUT",
    }:
        raise E2EFailure("offline dashboard did not return a bounded unavailable state")
    status, _, payload = client.exchange("GET", "/api/v1/pods?limit=1", timeout=25)
    if status not in {503, 504}:
        raise E2EFailure(f"offline resource path returned HTTP {status} instead of an unavailable state")
    if api_error_code(payload) not in {
            "CLUSTER_UNAVAILABLE",
            "AUTHENTICATION_UNAVAILABLE",
            "AUTHORIZATION_UNAVAILABLE",
            "UPSTREAM_TIMEOUT",
    }:
        raise E2EFailure("offline resource path returned an unexpected public error code")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--origin", required=True)
    parser.add_argument("--mode", choices=("allowed", "denied", "all", "offline"), required=True)
    parser.add_argument("--deployment-rv", default="unused")
    parser.add_argument("--pod-rv", default="unused")
    parser.add_argument("--pod-uid", default="00000000-0000-0000-0000-000000000000")
    parser.add_argument("--control-dir", type=pathlib.Path)
    parser.add_argument("--scan-root", type=pathlib.Path)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    client = Client(args.origin)
    status, _ = client.bootstrap()
    if args.mode == "allowed":
        check_allowed(client, status, args)
    elif args.mode == "denied":
        check_denied(client, status, args)
    elif args.mode == "all":
        check_all_scope(client, status)
    else:
        check_offline(client)
    print(f"app-e2e: {args.mode} product checks passed")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except E2EFailure as error:
        raise SystemExit(f"app-e2e: {error}") from error
