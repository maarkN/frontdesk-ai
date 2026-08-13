"""PII detection and redaction at emission (ADR-003, decision on redactions[]).

Detectors, in both EN and FR-CA: payment card (Luhn), SIN/NAS (9 digits, Luhn),
Canadian postal code (``A1A 1A1``), date of birth (numeric and month-name forms).
Spans are byte offsets (UTF-8) over the ORIGINAL text, as the contract requires.
No consumer of the event stream ever sees raw PII.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

from agent_runtime.models import Redaction

_CARD_RE = re.compile(r"(?<!\d)(?:\d[ -]?){13,19}(?<![ -])(?!\d)")
_SIN_RE = re.compile(r"(?<!\d)\d{3}[ -]?\d{3}[ -]?\d{3}(?!\d)")
_POSTAL_RE = re.compile(r"\b[A-Za-z]\d[A-Za-z][ -]?\d[A-Za-z]\d\b")
_DOB_NUMERIC_RE = re.compile(r"\b\d{1,4}[/-]\d{1,2}[/-]\d{2,4}\b")
_MONTHS = (
    "january|february|march|april|may|june|july|august|september|october|november|december"
    "|janvier|février|fevrier|mars|avril|mai|juin|juillet|août|aout|septembre|octobre"
    "|novembre|décembre|decembre"
)
_DOB_WORDS_RE = re.compile(
    rf"\b(?:\d{{1,2}}(?:er|st|nd|rd|th)?\s+(?:of\s+)?)?(?:{_MONTHS})"
    rf"(?:\s+\d{{1,2}}(?:er|st|nd|rd|th)?)?,?\s+\d{{4}}\b",
    re.IGNORECASE,
)


def luhn_ok(digits: str) -> bool:
    """Return True when the digit string passes the Luhn checksum."""
    total = 0
    for i, ch in enumerate(reversed(digits)):
        d = ord(ch) - ord("0")
        if i % 2 == 1:
            d *= 2
            if d > 9:
                d -= 9
        total += d
    return total % 10 == 0


@dataclass(frozen=True)
class _Span:
    start: int  # character offsets while scanning; converted to bytes at the end
    end: int
    kind: str


def _find_spans(text: str) -> list[_Span]:
    spans: list[_Span] = []

    def overlaps(a: int, b: int) -> bool:
        return any(not (b <= s.start or a >= s.end) for s in spans)

    # Cards first: a SIN-shaped window can sit inside a card number.
    for m in _CARD_RE.finditer(text):
        digits = re.sub(r"[ -]", "", m.group())
        if 13 <= len(digits) <= 19 and luhn_ok(digits):
            spans.append(_Span(m.start(), m.end(), "card"))
    for m in _SIN_RE.finditer(text):
        digits = re.sub(r"[ -]", "", m.group())
        if len(digits) == 9 and luhn_ok(digits) and not overlaps(m.start(), m.end()):
            spans.append(_Span(m.start(), m.end(), "sin"))
    for m in _POSTAL_RE.finditer(text):
        if not overlaps(m.start(), m.end()):
            spans.append(_Span(m.start(), m.end(), "postal_code"))
    for pattern in (_DOB_WORDS_RE, _DOB_NUMERIC_RE):
        for m in pattern.finditer(text):
            if not overlaps(m.start(), m.end()):
                spans.append(_Span(m.start(), m.end(), "dob"))
    spans.sort(key=lambda s: s.start)
    return spans


def redact(text: str) -> tuple[str, list[Redaction]]:
    """Redact PII from ``text``.

    Returns the redacted text (spans replaced with ``[redacted:<kind>]``) and
    the redaction list with byte offsets over the original text. ``vault`` is
    left empty: the runtime does not persist originals (crypto-shredding is
    the gateway's job, which owns the per-call DEK).
    """
    spans = _find_spans(text)
    if not spans:
        return text, []

    redactions: list[Redaction] = []
    out: list[str] = []
    cursor = 0
    for span in spans:
        out.append(text[cursor : span.start])
        out.append(f"[redacted:{span.kind}]")
        byte_start = len(text[: span.start].encode("utf-8"))
        byte_end = len(text[: span.end].encode("utf-8"))
        redactions.append(
            Redaction.model_validate(
                {"start": byte_start, "end": byte_end, "kind": span.kind, "vault": ""}
            )
        )
        cursor = span.end
    out.append(text[cursor:])
    return "".join(out), redactions


def contains_pii(text: str) -> bool:
    """True when any detector fires on ``text`` (used by NoPIIInEvents asserts)."""
    return bool(_find_spans(text))
