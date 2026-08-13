"""Core pydantic models: locales, slots, tenant config, event envelope.

Envelope mirrors contracts/events/v1/envelope.schema.json — the JSON schema
is the source of truth (ADR-003); this module must stay compatible with it.
"""

from __future__ import annotations

import enum
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field


class Locale(enum.StrEnum):
    """Supported conversation locales (catalog $defs/locale)."""

    EN_CA = "en-CA"
    FR_CA = "fr-CA"


class Urgency(enum.StrEnum):
    """Caller-declared urgency collected during qualification."""

    EMERGENCY = "emergency"
    HIGH = "high"
    NORMAL = "normal"


class Intent(enum.StrEnum):
    """Top-level caller intent decided at the intent node."""

    SCHEDULE = "schedule"
    MESSAGE = "message"
    EMERGENCY = "emergency"
    UNKNOWN = "unknown"


class Slots(BaseModel):
    """Structured slots collected by the qualification flow (RF2)."""

    model_config = ConfigDict(frozen=False)

    caller_name: str | None = None
    service: str | None = None
    postal_code: str | None = None
    in_coverage: bool | None = None
    urgency: Urgency | None = None
    preferred_slot: str | None = None
    message_text: str | None = None

    def missing_qualification(self) -> list[str]:
        """Return qualification slots still unfilled, in ask order."""
        missing: list[str] = []
        if self.service is None:
            missing.append("service")
        if self.postal_code is None:
            missing.append("postal_code")
        if self.urgency is None:
            missing.append("urgency")
        return missing


class PriceRange(BaseModel):
    """Configured price band for a service. Guardrail: quote ranges, never a closed price."""

    service: str
    min_cad: int
    max_cad: int


class TenantConfig(BaseModel):
    """Per-tenant agent configuration (EPIC-004 supplies this; fixture here)."""

    tenant_id: str
    business_name: str
    services: list[str]
    coverage_fsa: list[str] = Field(
        description="Forward sortation areas (first 3 chars of postal code) served."
    )
    owner_mobile: str = Field(description="E.164; emergency transfer target.")
    price_ranges: list[PriceRange] = Field(default_factory=list)
    locales: list[Locale] = Field(default_factory=lambda: [Locale.EN_CA, Locale.FR_CA])
    default_locale: Locale = Locale.EN_CA
    required_region: str = "ca-central-1"

    def price_range_for(self, service: str) -> PriceRange | None:
        """Return the configured price band for a service, if any."""
        for pr in self.price_ranges:
            if pr.service == service:
                return pr
        return None

    def covers(self, postal_code: str) -> bool:
        """Check a canonical postal code (``A1A 1A1``) against the coverage area."""
        fsa = postal_code.replace(" ", "")[:3].upper()
        return fsa in {f.upper() for f in self.coverage_fsa}


class Redaction(BaseModel):
    """PII span redacted at emission (envelope.schema.json $defs/redaction)."""

    model_config = ConfigDict(frozen=True)

    start: int
    end: int
    kind: Literal["card", "sin", "postal_code", "dob"]
    vault: str = ""


class EventEnvelope(BaseModel):
    """Business event envelope (contracts/events/v1). ``seq`` is monotonic per call."""

    model_config = ConfigDict(frozen=True)

    event_id: str = Field(alias="eventId")
    call_id: str = Field(alias="callId")
    seq: int = Field(ge=1)
    ts: str
    tenant_id: str = Field(alias="tenantId")
    flow_id: str = Field(alias="flowId")
    flow_version: int = Field(alias="flowVersion", ge=1)
    type: str
    node_id: str = Field(alias="nodeId", default="")
    payload: dict[str, Any]
    redactions: list[Redaction] = Field(default_factory=list)

    def to_wire(self) -> dict[str, Any]:
        """Serialize with the contract's camelCase field names."""
        return self.model_dump(by_alias=True, mode="json")
