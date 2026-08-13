"""Bridge between telephony-gw and the agent graph (EPIC-003 x EPIC-002).

Hot path: the gateway owns the turn state machine (endpointing, barge-in) and
pushes FINAL turn transcripts over a websocket; the bridge runs one graph turn
and answers with the reply text (the gateway drives TTS + PlayoutTracker).

Cold path: every business event built by the :class:`EventEmitter` (monotonic
``seq`` per call) is published to NATS JetStream with ``Nats-Msg-Id = eventId``
for dedup (contracts/events/v1). ``stt.partial`` stays on the hot path only.

Transports are Protocols: tests use in-memory fakes; production wires
``websockets`` and ``nats-py`` implementations (sketched below).
"""

from __future__ import annotations

import json
from typing import Any, Literal, Protocol, runtime_checkable

from pydantic import BaseModel, ConfigDict

from agent_runtime.events import EventEmitter
from agent_runtime.graph import AgentRuntime
from agent_runtime.models import EventEnvelope, TenantConfig

SUBJECT_PREFIX = "frontdesk.events"


class GatewayCallStart(BaseModel):
    """First message of a call session on the gateway websocket."""

    model_config = ConfigDict(frozen=True)

    type: Literal["call.start"]
    call_id: str
    tenant_id: str
    flow_id: str
    flow_version: int


class GatewayTurnFinal(BaseModel):
    """A finalized caller turn (endpointing already done by the gateway)."""

    model_config = ConfigDict(frozen=True)

    type: Literal["turn.final"]
    call_id: str
    text: str


class AgentReply(BaseModel):
    """Reply sent back to the gateway for TTS."""

    model_config = ConfigDict(frozen=True)

    type: Literal["agent.reply"] = "agent.reply"
    call_id: str
    text: str
    ended: bool


@runtime_checkable
class EventPublisher(Protocol):
    """NATS JetStream publisher abstraction (at-least-once)."""

    async def publish(self, subject: str, envelope: EventEnvelope) -> None:
        """Publish one envelope; must set ``Nats-Msg-Id = eventId`` for dedup."""
        ...


@runtime_checkable
class ReplySink(Protocol):
    """Where agent replies go (the gateway websocket in production)."""

    async def send(self, reply: AgentReply) -> None:
        """Deliver one reply to the gateway."""
        ...


class RuntimeFactory(Protocol):
    """Builds an :class:`AgentRuntime` per call (tenant-scoped providers/tools)."""

    def __call__(self, *, config: TenantConfig, emitter: EventEmitter) -> AgentRuntime:
        """Create the runtime for one call."""
        ...


def subject_for(envelope: EventEnvelope) -> str:
    """NATS subject convention: ``frontdesk.events.<tenantId>.<type>``."""
    return f"{SUBJECT_PREFIX}.{envelope.tenant_id}.{envelope.type}"


class CallSession:
    """One active call: graph runtime + event flushing."""

    def __init__(
        self,
        *,
        runtime: AgentRuntime,
        emitter: EventEmitter,
        publisher: EventPublisher,
        sink: ReplySink,
        call_id: str,
    ) -> None:
        self._runtime = runtime
        self._emitter = emitter
        self._publisher = publisher
        self._sink = sink
        self._call_id = call_id
        self._flushed = 0

    async def on_turn(self, text: str) -> AgentReply:
        """Run one graph turn, publish its events, reply to the gateway."""
        reply_text = await self._runtime.handle_turn(text)
        await self._flush_events()
        reply = AgentReply(call_id=self._call_id, text=reply_text, ended=self._runtime.state.ended)
        await self._sink.send(reply)
        return reply

    async def _flush_events(self) -> None:
        # Emission order == seq order; the consumer still dedups/orders by
        # (callId, seq) because NATS is at-least-once and unordered.
        pending = self._emitter.emitted[self._flushed :]
        for envelope in pending:
            await self._publisher.publish(subject_for(envelope), envelope)
        self._flushed += len(pending)


class Bridge:
    """Dispatches gateway websocket messages to per-call sessions."""

    def __init__(
        self,
        *,
        config_by_tenant: dict[str, TenantConfig],
        runtime_factory: RuntimeFactory,
        publisher: EventPublisher,
        sink: ReplySink,
    ) -> None:
        self._configs = config_by_tenant
        self._runtime_factory = runtime_factory
        self._publisher = publisher
        self._sink = sink
        self._sessions: dict[str, CallSession] = {}

    async def on_message(self, raw: str | bytes) -> AgentReply | None:
        """Handle one gateway message (JSON). Returns the reply for turn finals."""
        data: dict[str, Any] = json.loads(raw)
        kind = data.get("type")
        if kind == "call.start":
            start = GatewayCallStart.model_validate(data)
            config = self._configs.get(start.tenant_id)
            if config is None:
                # Unknown tenant: the gateway hangs up (CONTEXT.md, concept 5).
                raise KeyError(f"unknown tenant: {start.tenant_id}")
            emitter = EventEmitter(
                call_id=start.call_id,
                tenant_id=start.tenant_id,
                flow_id=start.flow_id,
                flow_version=start.flow_version,
            )
            runtime = self._runtime_factory(config=config, emitter=emitter)
            self._sessions[start.call_id] = CallSession(
                runtime=runtime,
                emitter=emitter,
                publisher=self._publisher,
                sink=self._sink,
                call_id=start.call_id,
            )
            return None
        if kind == "turn.final":
            turn = GatewayTurnFinal.model_validate(data)
            session = self._sessions[turn.call_id]
            return await session.on_turn(turn.text)
        raise ValueError(f"unknown gateway message type: {kind!r}")


class WebsocketGatewayServer:
    """Production server (sketch): one ``websockets`` connection per gateway.

    To be completed: ``websockets.serve(handler, ...)`` where the handler feeds
    ``Bridge.on_message`` and writes ``AgentReply`` JSON back on the same
    socket. Backpressure and reconnect live in the gateway (EPIC-002); NATS
    publishing uses ``nats-py`` with ``Nats-Msg-Id = eventId`` headers
    (EPIC-006 adds the circuit breaker around it).
    """

    def __init__(self, bridge: Bridge, *, host: str = "0.0.0.0", port: int = 8091) -> None:
        self._bridge = bridge
        self._host = host
        self._port = port

    async def serve_forever(self) -> None:
        """Run the websocket server (not implemented in the MVP runtime)."""
        raise NotImplementedError("WebsocketGatewayServer is a sketch; tests drive Bridge directly")
