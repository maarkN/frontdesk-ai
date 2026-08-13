"""Agent tools behind Protocols: calendar.propose/confirm and message.take.

Side effects carry ``idempotency_key = "{callId}:{nodeId}:{attempt}"`` — the
event stream is at-least-once, so a replayed tool.invoked must never create a
second calendar event (US-3.3).
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol, runtime_checkable

from pydantic import BaseModel, ConfigDict, Field

from agent_runtime.models import Urgency


class CalendarSlot(BaseModel):
    """One proposable appointment slot."""

    model_config = ConfigDict(frozen=True)

    slot_id: str
    start_iso: str
    end_iso: str
    label_en: str
    label_fr: str


class BookingResult(BaseModel):
    """Outcome of calendar.confirm."""

    model_config = ConfigDict(frozen=True)

    event_id: str
    slot: CalendarSlot
    created: bool = Field(description="False when the idempotency key was replayed.")


class TakenMessage(BaseModel):
    """Structured message recorded for the owner (RF2 fallback path)."""

    model_config = ConfigDict(frozen=True)

    caller_name: str | None
    service: str | None
    postal_code: str | None
    urgency: Urgency | None
    text: str
    reason: str


@runtime_checkable
class CalendarTool(Protocol):
    """Scheduling tool (Google Calendar in production)."""

    async def propose(self, *, service: str, urgency: Urgency) -> list[CalendarSlot]:
        """Return real availability for the service."""
        ...

    async def confirm(self, *, slot_id: str, idempotency_key: str) -> BookingResult:
        """Create the event; replaying the same key must not duplicate it."""
        ...


@runtime_checkable
class MessageTool(Protocol):
    """Structured message taking."""

    async def take(self, *, message: TakenMessage, idempotency_key: str) -> str:
        """Persist the message; returns a message id."""
        ...


class FakeGoogleCalendar:
    """Deterministic in-memory Google Calendar fake.

    Availability is fixed; ``confirm`` is idempotent by key, which lets tests
    replay tool events and assert no duplicate booking.
    """

    def __init__(self) -> None:
        self._slots = [
            CalendarSlot(
                slot_id="slot-1",
                start_iso="2026-08-14T09:00:00-04:00",
                end_iso="2026-08-14T11:00:00-04:00",
                label_en="tomorrow between 9 and 11 AM",
                label_fr="demain entre 9 h et 11 h",
            ),
            CalendarSlot(
                slot_id="slot-2",
                start_iso="2026-08-14T14:00:00-04:00",
                end_iso="2026-08-14T16:00:00-04:00",
                label_en="tomorrow between 2 and 4 PM",
                label_fr="demain entre 14 h et 16 h",
            ),
        ]
        self._bookings: dict[str, BookingResult] = {}

    @property
    def bookings(self) -> list[BookingResult]:
        """Confirmed bookings (test inspection)."""
        return list(self._bookings.values())

    async def propose(self, *, service: str, urgency: Urgency) -> list[CalendarSlot]:
        """Return the fixed availability; urgent calls only get the earliest slot."""
        if urgency is Urgency.HIGH:
            return self._slots[:1]
        return list(self._slots)

    async def confirm(self, *, slot_id: str, idempotency_key: str) -> BookingResult:
        """Create the booking once per idempotency key."""
        existing = self._bookings.get(idempotency_key)
        if existing is not None:
            return existing.model_copy(update={"created": False})
        slot = next(s for s in self._slots if s.slot_id == slot_id)
        result = BookingResult(event_id=f"gcal-{len(self._bookings) + 1}", slot=slot, created=True)
        self._bookings[idempotency_key] = result
        return result


class FakeMessageStore:
    """Deterministic in-memory message store."""

    def __init__(self) -> None:
        self.messages: dict[str, TakenMessage] = {}

    async def take(self, *, message: TakenMessage, idempotency_key: str) -> str:
        """Persist the message once per idempotency key."""
        self.messages.setdefault(idempotency_key, message)
        return f"msg-{idempotency_key}"


if TYPE_CHECKING:
    _calendar_impls: tuple[CalendarTool, ...] = (FakeGoogleCalendar(),)
    _message_impls: tuple[MessageTool, ...] = (FakeMessageStore(),)
