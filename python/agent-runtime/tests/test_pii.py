"""PII redaction at emission: both locales, byte offsets, Luhn (ADR-003)."""

from __future__ import annotations

from agent_runtime.pii import contains_pii, luhn_ok, redact


def test_luhn() -> None:
    assert luhn_ok("4111111111111111")
    assert not luhn_ok("4111111111111112")
    assert luhn_ok("046454286")  # canonical valid SIN


def test_card_redacted_en() -> None:
    text = "my card is 4532 0151 1283 0366 thanks"
    clean, spans = redact(text)
    assert "4532" not in clean
    assert "[redacted:card]" in clean
    assert [s.kind for s in spans] == ["card"]
    start, end = spans[0].start, spans[0].end
    assert text.encode("utf-8")[start:end].decode("utf-8") == "4532 0151 1283 0366"


def test_card_not_luhn_valid_is_kept() -> None:
    clean, spans = redact("reference number 1234 5678 9012 3456")
    assert spans == []
    assert "1234" in clean


def test_sin_redacted_fr() -> None:
    text = "mon NAS est 046-454-286, merci"
    clean, spans = redact(text)
    assert "046" not in clean
    assert [s.kind for s in spans] == ["sin"]


def test_postal_code_redacted() -> None:
    clean, spans = redact("I'm at H2X 1Y4 near downtown")
    assert "H2X" not in clean
    assert [s.kind for s in spans] == ["postal_code"]


def test_dob_redacted_both_locales() -> None:
    clean_en, spans_en = redact("I was born on January 5, 1985 you know")
    assert [s.kind for s in spans_en] == ["dob"]
    assert "1985" not in clean_en

    clean_fr, spans_fr = redact("née le 5 janvier 1985 à Montréal")
    assert [s.kind for s in spans_fr] == ["dob"]
    assert "1985" not in clean_fr

    clean_num, spans_num = redact("dob 1985-01-05 ok")
    assert [s.kind for s in spans_num] == ["dob"]
    assert "1985" not in clean_num


def test_byte_offsets_with_accents() -> None:
    # Multibyte chars before the span: offsets are BYTES over the original.
    text = "téléphone déjà noté — NAS 046 454 286 fin"
    clean, spans = redact(text)
    assert len(spans) == 1
    span = spans[0]
    raw = text.encode("utf-8")[span.start : span.end].decode("utf-8")
    assert raw == "046 454 286"
    assert "046" not in clean


def test_card_takes_priority_over_sin_window() -> None:
    text = "card 4111 1111 1111 1111 end"
    _, spans = redact(text)
    assert [s.kind for s in spans] == ["card"]


def test_contains_pii() -> None:
    assert contains_pii("SIN 046 454 286")
    assert not contains_pii("nothing sensitive here, just a leaky faucet")
