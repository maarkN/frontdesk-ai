"""Agent guardrails (CONTEXT.md concept 8, US-3.4).

Deterministic, testable checks that run OUTSIDE the LLM:

1. Emergency detection (EN/FR dictionary) -> signal transfer to the owner's
   mobile. Checked on every inbound turn, before any node logic.
2. Never quote a closed price — price questions get the configured range or a
   deflection; outbound text is scanned for dollar amounts not backed by config.
3. Never promise outside tenant config — services not configured route to
   message taking, and outbound promises are lexically screened.
4. Prompt-injection resistance: system instructions are immutable (the caller's
   text is DATA, never instructions), injection attempts are flagged, and the
   output screen blocks discounts/free-service promises regardless of what the
   caller asked (note 09: caller_prompt_injector is a security gate).
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

from agent_runtime.models import Locale, TenantConfig

EMERGENCY_TERMS: dict[Locale, frozenset[str]] = {
    Locale.EN_CA: frozenset(
        {
            "flood",
            "flooding",
            "flooded",
            "burst pipe",
            "pipe burst",
            "no heat",
            "gas leak",
            "smell gas",
            "sewage backup",
            "sewer backup",
            "water everywhere",
            "carbon monoxide",
            "sparks",
            "electrical fire",
        }
    ),
    Locale.FR_CA: frozenset(
        {
            "inondation",
            "inondé",
            "inonde",
            "fuite de gaz",
            "odeur de gaz",
            "dégât d'eau",
            "degat d'eau",
            "tuyau éclaté",
            "tuyau eclate",
            "pas de chauffage",
            "plus de chauffage",
            "refoulement d'égout",
            "refoulement d'egout",
            "monoxyde de carbone",
            "fuite d'eau majeure",
            "eau partout",
        }
    ),
}

_INJECTION_PATTERNS = (
    re.compile(r"ignore\s+(your|all|previous|the)\s+(instructions|rules|prompt)", re.I),
    re.compile(r"you\s+are\s+now\s+", re.I),
    re.compile(r"system\s*prompt", re.I),
    re.compile(r"oublie\s+(tes|les)\s+(instructions|consignes|règles|regles)", re.I),
    re.compile(r"pretend\s+to\s+be", re.I),
    re.compile(r"\bjailbreak\b", re.I),
    re.compile(r"(give|donne)[^.]{0,40}(discount|rabais|réduction|reduction)", re.I),
    re.compile(r"\d{1,3}\s*%\s*(off|discount|de\s+(rabais|réduction|reduction))", re.I),
)

_PRICE_QUESTION_PATTERNS = (
    re.compile(r"how\s+much|price|cost|quote|charge", re.I),
    re.compile(r"combien|prix|coût|cout|tarif|soumission", re.I),
)

# Dollar amounts in outbound text, e.g. "$120", "120 $", "120 dollars".
_DOLLAR_RE = re.compile(r"\$\s*\d[\d,.]*|\b\d[\d,.]*\s*(?:\$|dollars?)\b", re.I)
_PROMISE_RE = re.compile(
    r"\b(free|no charge|gratuit|sans frais|guarantee[ds]?|garanti[es]?|discount|rabais|réduction)\b",
    re.I,
)


@dataclass(frozen=True)
class InboundVerdict:
    """Result of screening one caller turn."""

    emergency: bool = False
    matched_term: str = ""
    injection_attempt: bool = False
    price_question: bool = False


@dataclass(frozen=True)
class OutboundVerdict:
    """Result of screening one agent reply before it is spoken."""

    allowed: bool
    violations: tuple[str, ...] = field(default=())


def screen_inbound(text: str) -> InboundVerdict:
    """Classify a caller turn against the guardrail dictionaries (both locales).

    Emergency terms are checked in BOTH languages regardless of the active
    locale: a panicking bilingual caller will not wait for language detection.
    """
    lowered = text.lower()
    for terms in EMERGENCY_TERMS.values():
        for term in terms:
            if term in lowered:
                return InboundVerdict(
                    emergency=True,
                    matched_term=term,
                    injection_attempt=_is_injection(text),
                    price_question=_is_price_question(text),
                )
    return InboundVerdict(
        injection_attempt=_is_injection(text),
        price_question=_is_price_question(text),
    )


def screen_outbound(text: str, *, config: TenantConfig) -> OutboundVerdict:
    """Screen an agent reply before TTS.

    Blocks closed prices (any dollar amount that is not one of the configured
    range endpoints) and promise words (free/guarantee/discount) that tenant
    config does not authorize. This is the last line of defense: it applies to
    LLM output no matter what the caller injected upstream.
    """
    violations: list[str] = []
    allowed_amounts = {
        str(amount) for pr in config.price_ranges for amount in (pr.min_cad, pr.max_cad)
    }
    for m in _DOLLAR_RE.finditer(text):
        digits = re.sub(r"[^\d]", "", m.group())
        if digits and digits not in allowed_amounts:
            violations.append(f"closed_price:{m.group().strip()}")
    if _PROMISE_RE.search(text):
        violations.append("unconfigured_promise")
    return OutboundVerdict(allowed=not violations, violations=tuple(violations))


def price_answer(service: str | None, *, config: TenantConfig, locale: Locale) -> str:
    """Compose the only allowed price answer: the configured range, or a deflection."""
    pr = config.price_range_for(service) if service else None
    if pr is None:
        if locale is Locale.FR_CA:
            return (
                "Le prix dépend du travail sur place, je ne peux pas donner de montant "
                "ferme au téléphone. Le technicien confirmera le prix avant de commencer."
            )
        return (
            "Pricing depends on what the technician finds on site, so I can't give a "
            "firm number over the phone. The technician will confirm the price before "
            "any work starts."
        )
    if locale is Locale.FR_CA:
        return (
            f"Pour ce service, c'est généralement entre {pr.min_cad} $ et {pr.max_cad} $, "
            "selon le travail. Le technicien confirmera le prix exact sur place."
        )
    return (
        f"For that service it's typically between ${pr.min_cad} and ${pr.max_cad} "
        "depending on the job. The technician will confirm the exact price on site."
    )


def _is_injection(text: str) -> bool:
    return any(p.search(text) for p in _INJECTION_PATTERNS)


def _is_price_question(text: str) -> bool:
    return any(p.search(text) for p in _PRICE_QUESTION_PATTERNS)
