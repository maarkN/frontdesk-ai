"""OpenTelemetry wiring for the agent runtime (EPIC-010).

One span per caller turn (``agent.turn``) with the product attributes —
``frontdesk.locale``, ``frontdesk.llm_tier`` (cost cascade, US-3.6),
``frontdesk.tool`` — exported over OTLP gRPC alongside metrics.

Design constraints:

* **Off by default.** Without ``OTEL_EXPORTER_OTLP_ENDPOINT`` (or an injected
  exporter) nothing is installed and :func:`turn_span` yields a no-op span, so
  the existing test suite runs untouched.
* **No global provider mutation.** The provider lives in module state, not in
  ``opentelemetry.trace.set_tracer_provider`` — the global registry can only
  be set once per process, which breaks test isolation. Tests inject an
  in-memory exporter via :func:`configure` and call :func:`shutdown`.
"""

from __future__ import annotations

import os
from collections.abc import Iterator
from contextlib import contextmanager

from opentelemetry import trace
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import MetricReader, PeriodicExportingMetricReader
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, SpanExporter
from opentelemetry.trace import Span

SERVICE_NAME = "agent-runtime"
_INSTRUMENTATION = "agent_runtime"

_tracer_provider: TracerProvider | None = None
_meter_provider: MeterProvider | None = None


def configure(
    *,
    service_name: str = SERVICE_NAME,
    endpoint: str | None = None,
    span_exporter: SpanExporter | None = None,
    metric_readers: list[MetricReader] | None = None,
) -> bool:
    """Install the runtime's tracer/meter providers.

    Resolution order: explicit ``span_exporter``/``metric_readers`` (tests),
    else ``endpoint``, else ``OTEL_EXPORTER_OTLP_ENDPOINT``. Returns ``False``
    and installs nothing when no destination is configured.
    """
    global _tracer_provider, _meter_provider
    endpoint = endpoint or os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT")
    if endpoint is None and span_exporter is None and metric_readers is None:
        return False

    resource = Resource.create({"service.name": service_name})

    if span_exporter is None:
        # Imported lazily: the gRPC exporter spins up a channel at import in
        # some versions; tests with injected exporters never touch it.
        from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import (
            OTLPSpanExporter,
        )

        span_exporter = OTLPSpanExporter(endpoint=endpoint, insecure=True)
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(BatchSpanProcessor(span_exporter))
    _tracer_provider = provider

    if metric_readers is None:
        from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import (
            OTLPMetricExporter,
        )

        metric_readers = [
            PeriodicExportingMetricReader(OTLPMetricExporter(endpoint=endpoint, insecure=True))
        ]
    _meter_provider = MeterProvider(resource=resource, metric_readers=metric_readers)
    return True


def shutdown() -> None:
    """Flush and drop the installed providers (idempotent)."""
    global _tracer_provider, _meter_provider
    if _tracer_provider is not None:
        _tracer_provider.shutdown()
        _tracer_provider = None
    if _meter_provider is not None:
        _meter_provider.shutdown()
        _meter_provider = None


def _tracer() -> trace.Tracer:
    if _tracer_provider is not None:
        return _tracer_provider.get_tracer(_INSTRUMENTATION)
    # Global default is a no-op tracer unless the host app installed one.
    return trace.get_tracer(_INSTRUMENTATION)


@contextmanager
def turn_span(*, call_id: str) -> Iterator[Span]:
    """One span per caller turn; the bridge fills the attributes post-turn."""
    with _tracer().start_as_current_span("agent.turn") as span:
        span.set_attribute("frontdesk.call_id", call_id)
        yield span


def annotate_turn(
    span: Span,
    *,
    turn: int,
    locale: str,
    tier: str | None,
    tools: list[str],
    ended: bool,
) -> None:
    """Stamp the product attributes of one finished turn on its span."""
    span.set_attribute("frontdesk.turn", turn)
    span.set_attribute("frontdesk.locale", locale)
    if tier is not None:
        span.set_attribute("frontdesk.llm_tier", tier)
    if tools:
        span.set_attribute("frontdesk.tool", tools[-1])
        span.set_attribute("frontdesk.tools_invoked", tools)
    span.set_attribute("frontdesk.call_ended", ended)
