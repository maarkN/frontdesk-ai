"""Telemetry tests: in-memory exporters, no network, no global provider leak."""

from __future__ import annotations

import asyncio
import json
from collections.abc import Iterator

import pytest
from opentelemetry.sdk.metrics.export import InMemoryMetricReader
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

from agent_runtime import telemetry
from agent_runtime.models import Locale
from conftest import CALL_ID, make_bridge


@pytest.fixture
def span_exporter() -> Iterator[InMemorySpanExporter]:
    """Configure telemetry against an in-memory exporter, torn down after."""
    exporter = InMemorySpanExporter()
    assert telemetry.configure(
        span_exporter=exporter,
        metric_readers=[InMemoryMetricReader()],
    )
    yield exporter
    telemetry.shutdown()


def test_configure_without_endpoint_is_noop() -> None:
    assert telemetry.configure() is False
    with telemetry.turn_span(call_id="call-x") as span:
        # The no-op span accepts attributes without error.
        span.set_attribute("frontdesk.locale", "en-CA")


def test_turn_span_records_product_attributes(span_exporter: InMemorySpanExporter) -> None:
    with telemetry.turn_span(call_id=CALL_ID) as span:
        telemetry.annotate_turn(
            span,
            turn=3,
            locale=Locale.FR_CA.value,
            tier="haiku",
            tools=["calendar.confirm"],
            ended=False,
        )
    telemetry.shutdown()  # flush the batch processor

    spans = [s for s in span_exporter.get_finished_spans() if s.name == "agent.turn"]
    assert len(spans) == 1
    attrs = spans[-1].attributes
    assert attrs is not None
    assert attrs["frontdesk.call_id"] == CALL_ID
    assert attrs["frontdesk.turn"] == 3
    assert attrs["frontdesk.locale"] == "fr-CA"
    assert attrs["frontdesk.llm_tier"] == "haiku"
    assert attrs["frontdesk.tool"] == "calendar.confirm"
    assert attrs["frontdesk.call_ended"] is False


def test_bridge_emits_one_span_per_turn(span_exporter: InMemorySpanExporter) -> None:
    bridge, _publisher, _sink = make_bridge()
    call_id = "call-telemetry-1"

    async def scenario() -> None:
        from conftest import TENANT_ID

        await bridge.on_message(
            json.dumps(
                {
                    "type": "call.start",
                    "call_id": call_id,
                    "tenant_id": TENANT_ID,
                    "flow_id": "receptionist",
                    "flow_version": 1,
                }
            )
        )
        await bridge.on_message(
            json.dumps({"type": "turn.final", "call_id": call_id, "text": "hi"})
        )
        await bridge.on_message(
            json.dumps(
                {
                    "type": "turn.final",
                    "call_id": call_id,
                    "text": "my water heater is broken",
                }
            )
        )

    asyncio.run(scenario())
    telemetry.shutdown()  # flush the batch processor

    spans = [s for s in span_exporter.get_finished_spans() if s.name == "agent.turn"]
    assert len(spans) == 2
    for s in spans:
        assert s.attributes is not None
        assert s.attributes["frontdesk.call_id"] == call_id
        assert s.attributes["frontdesk.locale"] in {"en-CA", "fr-CA"}
        assert s.attributes["frontdesk.turn"] in {1, 2}
