"""Envelope emission: ULID shape, monotonic seq, redaction, stt.partial ban."""

from __future__ import annotations

import re
from datetime import UTC, datetime

import pytest

from agent_runtime.events import EventEmitter, new_ulid

_ULID_RE = re.compile(r"^[0-7][0-9A-HJKMNP-TV-Z]{25}$")


def make_emitter() -> EventEmitter:
    return EventEmitter(
        call_id="call-42",
        tenant_id="tenant-1",
        flow_id="receptionist",
        flow_version=3,
        clock=lambda: datetime(2026, 8, 13, 12, 0, 0, tzinfo=UTC),
    )


def test_ulid_matches_contract_pattern() -> None:
    for _ in range(50):
        assert _ULID_RE.match(new_ulid())
    # Deterministic when time and entropy are injected.
    a = new_ulid(now_ms=1, entropy=b"\x00" * 10)
    b = new_ulid(now_ms=1, entropy=b"\x00" * 10)
    assert a == b


def test_seq_is_monotonic_from_one() -> None:
    emitter = make_emitter()
    e1 = emitter.emit("call.answered", {"answeredBy": "agent"})
    e2 = emitter.emit("node.entered", {"nodeType": "greeting"}, node_id="greeting")
    e3 = emitter.emit("call.ended", {"reason": "completed", "durationMs": 1000})
    assert [e1.seq, e2.seq, e3.seq] == [1, 2, 3]
    assert e2.node_id == "greeting"


def test_envelope_wire_format_matches_contract() -> None:
    emitter = make_emitter()
    envelope = emitter.emit("stt.final", {"text": "hello", "locale": "en-CA"})
    wire = envelope.to_wire()
    assert set(wire) == {
        "eventId",
        "callId",
        "seq",
        "ts",
        "tenantId",
        "flowId",
        "flowVersion",
        "type",
        "nodeId",
        "payload",
        "redactions",
    }
    assert wire["callId"] == "call-42"
    assert wire["flowVersion"] == 3
    assert wire["ts"].endswith("Z")
    assert _ULID_RE.match(wire["eventId"])


def test_stt_partial_is_banned_from_the_bus() -> None:
    emitter = make_emitter()
    with pytest.raises(ValueError, match=r"stt\.partial"):
        emitter.emit("stt.partial", {"text": "hel"})


def test_emitter_redacts_text_payloads() -> None:
    emitter = make_emitter()
    envelope = emitter.emit("stt.final", {"text": "my SIN is 046 454 286 ok", "locale": "en-CA"})
    assert "046" not in envelope.payload["text"]
    assert envelope.redactions[0].kind == "sin"

    var = emitter.emit("var.assigned", {"name": "postal_code", "value": "H2X 1Y4"})
    assert var.payload["value"] == "[redacted:postal_code]"
