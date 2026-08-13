"""Event emission per contracts/events/v1: ULID ids, monotonic seq, PII redaction.

``state = fold(events)`` (ADR-003). The emitter is the single choke point where
text payloads are redacted — no caller can publish raw PII, and ``stt.partial``
has no emission path at all (it is not in the catalog on purpose).
"""

from __future__ import annotations

import os
import time
from collections.abc import Callable
from datetime import UTC, datetime
from typing import Any

from agent_runtime.models import EventEnvelope, Redaction
from agent_runtime.pii import redact

_CROCKFORD = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

# Payload text fields that must be redacted at emission, per event type.
_TEXT_FIELDS: dict[str, tuple[str, ...]] = {
    "stt.final": ("text",),
    "agent.turn.completed": ("text",),
    "var.assigned": ("value",),
}


def new_ulid(*, now_ms: int | None = None, entropy: bytes | None = None) -> str:
    """Generate a ULID (Crockford base32, 26 chars). Injectable for tests."""
    ts = int(time.time() * 1000) if now_ms is None else now_ms
    rand = os.urandom(10) if entropy is None else entropy
    if len(rand) != 10:
        raise ValueError("ULID entropy must be exactly 10 bytes")
    value = (ts & ((1 << 48) - 1)) << 80 | int.from_bytes(rand, "big")
    chars = []
    for shift in range(125, -1, -5):
        chars.append(_CROCKFORD[(value >> shift) & 0x1F])
    return "".join(chars)


class EventEmitter:
    """Builds envelopes for one call with a monotonic ``seq`` starting at 1.

    NATS is at-least-once and unordered; consumers dedup/order by
    (``callId``, ``seq``) — so ``seq`` gaps or repeats from a single emitter
    are bugs. This class is the only allowed envelope constructor in the
    runtime.
    """

    def __init__(
        self,
        *,
        call_id: str,
        tenant_id: str,
        flow_id: str,
        flow_version: int,
        clock: Callable[[], datetime] | None = None,
        ulid: Callable[[], str] = new_ulid,
    ) -> None:
        self._call_id = call_id
        self._tenant_id = tenant_id
        self._flow_id = flow_id
        self._flow_version = flow_version
        self._clock = clock or (lambda: datetime.now(UTC))
        self._ulid = ulid
        self._seq = 0
        self.emitted: list[EventEnvelope] = []

    @property
    def call_id(self) -> str:
        """Call this emitter belongs to (used in tool idempotency keys)."""
        return self._call_id

    def emit(self, type_: str, payload: dict[str, Any], *, node_id: str = "") -> EventEnvelope:
        """Emit one event; text payload fields are redacted here, at emission."""
        if type_ == "stt.partial":
            raise ValueError("stt.partial never goes to the bus (ADR-003)")
        payload, redactions = self._redact_payload(type_, dict(payload))
        self._seq += 1
        envelope = EventEnvelope.model_validate(
            {
                "eventId": self._ulid(),
                "callId": self._call_id,
                "seq": self._seq,
                "ts": self._clock().isoformat().replace("+00:00", "Z"),
                "tenantId": self._tenant_id,
                "flowId": self._flow_id,
                "flowVersion": self._flow_version,
                "type": type_,
                "nodeId": node_id,
                "payload": payload,
                "redactions": [r.model_dump() for r in redactions],
            }
        )
        self.emitted.append(envelope)
        return envelope

    def _redact_payload(
        self, type_: str, payload: dict[str, Any]
    ) -> tuple[dict[str, Any], list[Redaction]]:
        redactions: list[Redaction] = []
        for field in _TEXT_FIELDS.get(type_, ()):
            value = payload.get(field)
            if isinstance(value, str):
                clean, spans = redact(value)
                payload[field] = clean
                redactions.extend(spans)
        return payload, redactions
