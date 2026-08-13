"""Flow-sim harness (note 09, middle layer): STT/TTS as text passthrough.

No network, no real providers. A whole conversation runs in milliseconds.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field

import pytest

from agent_runtime.bridge import AgentReply, Bridge
from agent_runtime.events import EventEmitter
from agent_runtime.graph import AgentRuntime
from agent_runtime.models import EventEnvelope, Locale, PriceRange, TenantConfig
from agent_runtime.providers.claude import ScriptedLLM
from agent_runtime.tools import FakeGoogleCalendar, FakeMessageStore

CALL_ID = "call-0001"
TENANT_ID = "tenant-plumber-mtl"


def make_config() -> TenantConfig:
    return TenantConfig(
        tenant_id=TENANT_ID,
        business_name="Plomberie Tremblay",
        services=["water heater", "drain cleaning", "faucet repair"],
        coverage_fsa=["H2X", "H2Y", "H3A"],
        owner_mobile="+15145550123",
        price_ranges=[PriceRange(service="water heater", min_cad=180, max_cad=450)],
        default_locale=Locale.EN_CA,
    )


@dataclass
class Sim:
    """One simulated call wired to fakes; drives the runtime turn by turn."""

    config: TenantConfig
    emitter: EventEmitter
    runtime: AgentRuntime
    llm: ScriptedLLM
    calendar: FakeGoogleCalendar
    messages: FakeMessageStore
    replies: list[str] = field(default_factory=list)

    async def say(self, text: str) -> str:
        reply = await self.runtime.handle_turn(text)
        self.replies.append(reply)
        return reply

    @property
    def events(self) -> list[EventEnvelope]:
        return self.emitter.emitted

    def events_of(self, type_: str) -> list[EventEnvelope]:
        return [e for e in self.events if e.type == type_]

    def wire_json(self) -> str:
        """Every emitted event serialized as it would hit the bus."""
        return json.dumps([e.to_wire() for e in self.events])


def make_sim(config: TenantConfig | None = None) -> Sim:
    cfg = config or make_config()
    emitter = EventEmitter(
        call_id=CALL_ID,
        tenant_id=cfg.tenant_id,
        flow_id="receptionist",
        flow_version=1,
    )
    llm = ScriptedLLM()
    calendar = FakeGoogleCalendar()
    messages = FakeMessageStore()
    runtime = AgentRuntime(
        config=cfg, emitter=emitter, llm=llm, calendar=calendar, messages=messages
    )
    return Sim(
        config=cfg,
        emitter=emitter,
        runtime=runtime,
        llm=llm,
        calendar=calendar,
        messages=messages,
    )


@pytest.fixture
def sim() -> Sim:
    return make_sim()


@dataclass
class MemoryPublisher:
    """In-memory NATS stand-in."""

    published: list[tuple[str, EventEnvelope]] = field(default_factory=list)

    async def publish(self, subject: str, envelope: EventEnvelope) -> None:
        self.published.append((subject, envelope))


@dataclass
class MemorySink:
    """In-memory gateway websocket stand-in."""

    sent: list[AgentReply] = field(default_factory=list)

    async def send(self, reply: AgentReply) -> None:
        self.sent.append(reply)


def make_bridge() -> tuple[Bridge, MemoryPublisher, MemorySink]:
    cfg = make_config()
    publisher = MemoryPublisher()
    sink = MemorySink()

    def factory(*, config: TenantConfig, emitter: EventEmitter) -> AgentRuntime:
        return AgentRuntime(
            config=config,
            emitter=emitter,
            llm=ScriptedLLM(),
            calendar=FakeGoogleCalendar(),
            messages=FakeMessageStore(),
        )

    bridge = Bridge(
        config_by_tenant={cfg.tenant_id: cfg},
        runtime_factory=factory,
        publisher=publisher,
        sink=sink,
    )
    return bridge, publisher, sink
