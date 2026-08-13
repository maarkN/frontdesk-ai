"""Bridge: gateway messages in, replies + published envelopes out."""

from __future__ import annotations

import asyncio
import json

import pytest

from agent_runtime.bridge import subject_for
from conftest import TENANT_ID, make_bridge


def _start_msg(call_id: str = "call-9") -> str:
    return json.dumps(
        {
            "type": "call.start",
            "call_id": call_id,
            "tenant_id": TENANT_ID,
            "flow_id": "receptionist",
            "flow_version": 1,
        }
    )


def _turn_msg(text: str, call_id: str = "call-9") -> str:
    return json.dumps({"type": "turn.final", "call_id": call_id, "text": text})


def test_bridge_runs_turns_and_publishes_envelopes() -> None:
    bridge, publisher, sink = make_bridge()

    async def scenario() -> None:
        assert await bridge.on_message(_start_msg()) is None
        reply = await bridge.on_message(_turn_msg("Hi, I need to book a water heater repair"))
        assert reply is not None
        assert "postal code" in reply.text.lower()
        assert reply.ended is False

    asyncio.run(scenario())
    assert len(sink.sent) == 1
    # Every emitted envelope was published, in seq order, on the tenant subject.
    seqs = [e.seq for _, e in publisher.published]
    assert seqs == sorted(seqs)
    assert seqs[0] == 1
    subjects = {s for s, _ in publisher.published}
    assert all(s.startswith(f"frontdesk.events.{TENANT_ID}.") for s in subjects)
    types = [e.type for _, e in publisher.published]
    assert "stt.final" in types
    assert "agent.turn.completed" in types
    assert "stt.partial" not in types


def test_bridge_subject_convention() -> None:
    bridge, publisher, _ = make_bridge()

    async def scenario() -> None:
        await bridge.on_message(_start_msg())
        await bridge.on_message(_turn_msg("hello there"))

    asyncio.run(scenario())
    subject, envelope = publisher.published[0]
    assert subject == subject_for(envelope)
    assert subject == f"frontdesk.events.{TENANT_ID}.{envelope.type}"


def test_bridge_rejects_unknown_tenant_and_type() -> None:
    bridge, _, _ = make_bridge()

    async def scenario() -> None:
        bad_start = json.dumps(
            {
                "type": "call.start",
                "call_id": "c",
                "tenant_id": "ghost",
                "flow_id": "f",
                "flow_version": 1,
            }
        )
        with pytest.raises(KeyError):
            await bridge.on_message(bad_start)
        with pytest.raises(ValueError, match="unknown gateway message"):
            await bridge.on_message(json.dumps({"type": "nope"}))

    asyncio.run(scenario())
