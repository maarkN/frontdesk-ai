"""EN/FR-CA language detection (first turn) and mid-call switch detection.

Deterministic marker-based scoring: cheap, offline, testable, and good enough
for the >=95% first-turn target on the fixture suite (US-3.1). A provider-based
detector can replace this behind the same function signature later.
"""

from __future__ import annotations

import re

from agent_runtime.models import Locale

_FR_MARKERS = frozenset(
    {
        "bonjour",
        "salut",
        "merci",
        "oui",
        "non",
        "je",
        "j'ai",
        "suis",
        "veux",
        "voudrais",
        "besoin",
        "mon",
        "ma",
        "mes",
        "le",
        "la",
        "les",
        "un",
        "une",
        "des",
        "est",
        "et",
        "pour",
        "avec",
        "chez",
        "rendez-vous",
        "s'il",
        "plaît",
        "plait",
        "urgence",
        "urgent",
        "fuite",
        "chauffage",
        "eau",
        "plombier",
        "problème",
        "probleme",
        "maison",
        "vous",
        "pouvez",
        "être",
        "laisser",
        "aimerais",
        "prendre",
        "svp",
        "ça",
        "pas",
        "de",
        "du",
    }
)
_EN_MARKERS = frozenset(
    {
        "hello",
        "hi",
        "hey",
        "thanks",
        "thank",
        "yes",
        "no",
        "i",
        "i'm",
        "my",
        "the",
        "a",
        "an",
        "is",
        "and",
        "for",
        "with",
        "need",
        "want",
        "would",
        "like",
        "book",
        "appointment",
        "schedule",
        "message",
        "leave",
        "please",
        "water",
        "heater",
        "help",
        "can",
        "you",
        "have",
        "problem",
        "house",
        "call",
        "back",
        "it's",
        "urgent",
        "not",
        "this",
        "week",
        "today",
    }
)

_WORD_RE = re.compile(r"[a-zà-ÿ'\u2019-]+", re.IGNORECASE)


def detect(text: str, *, default: Locale = Locale.EN_CA) -> tuple[Locale, float]:
    """Detect the locale of one utterance.

    Returns (locale, confidence in [0, 1]). Falls back to ``default`` when the
    utterance carries no markers (e.g. digits only).
    """
    words = [w.lower() for w in _WORD_RE.findall(text)]
    if not words:
        return default, 0.0
    fr = sum(1 for w in words if w in _FR_MARKERS)
    en = sum(1 for w in words if w in _EN_MARKERS)
    # Accented characters are a strong FR signal on their own.
    if re.search(r"[àâçéèêëîïôùûü]", text, re.IGNORECASE):
        fr += 2
    total = fr + en
    if total == 0:
        return default, 0.0
    if fr > en:
        return Locale.FR_CA, fr / total
    if en > fr:
        return Locale.EN_CA, en / total
    return default, 0.5


def should_switch(current: Locale, text: str, *, min_confidence: float = 0.7) -> Locale | None:
    """Return the new locale when the caller clearly switched languages mid-call.

    Conservative on purpose: a single borrowed word must not flip the locale
    (the turn state machine must never restart a node on a false switch).
    """
    detected, confidence = detect(text, default=current)
    if detected is not current and confidence >= min_confidence:
        return detected
    return None
