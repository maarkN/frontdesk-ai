"""Cartesia streaming TTS (sketch) + deterministic fake with word timings.

Word timings are non-negotiable for any TTS provider here: the gateway's
PlayoutTracker records only what the caller actually HEARD, truncating at word
boundaries on barge-in. A provider that cannot report timings cannot be used.
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import TYPE_CHECKING

from agent_runtime.models import Locale
from agent_runtime.providers.base import AudioChunk, TTSProvider, WordTiming

_CARTESIA_URL = "wss://api.cartesia.ai/tts/websocket"
_VOICE_BY_LOCALE = {
    # Persona-versioned voice ids (placeholder ids; chosen with the persona).
    Locale.EN_CA: "voice-en-ca-frontdesk-v1",
    Locale.FR_CA: "voice-fr-ca-frontdesk-v1",
}


class CartesiaTTS:
    """Streaming TTS over Cartesia's websocket API (sketch).

    Wire protocol (to be completed): connect to ``_CARTESIA_URL``, send the
    transcript with ``add_timestamps: true`` (word-level timestamps are what
    feeds :class:`WordTiming`), stream ``chunk`` messages as :class:`AudioChunk`
    with raw PCM and the ``word_timestamps`` payload mapped to timings.
    First-chunk latency budget: ~200 ms (CONTEXT.md, concept 1).
    """

    def __init__(self, api_key: str, *, url: str = _CARTESIA_URL) -> None:
        self._api_key = api_key
        self._url = url

    async def synthesize(self, text: str, *, locale: Locale) -> AsyncIterator[AudioChunk]:
        """Synthesize via Cartesia (not implemented in the MVP runtime)."""
        raise NotImplementedError(
            "CartesiaTTS is a sketch; use PassthroughTTS in tests "
            f"(would use voice {_VOICE_BY_LOCALE[locale]})"
        )
        yield AudioChunk(audio=b"")  # pragma: no cover


class PassthroughTTS:
    """Deterministic fake: 'audio' is the UTF-8 text; timings are synthetic.

    Each word takes ``ms_per_word`` ms with a fixed gap, so PlayoutTracker
    tests can assert exact truncation points. Chunked one word at a time to
    exercise streaming consumers.
    """

    def __init__(self, *, ms_per_word: int = 300, gap_ms: int = 50) -> None:
        self._ms_per_word = ms_per_word
        self._gap_ms = gap_ms

    async def synthesize(self, text: str, *, locale: Locale) -> AsyncIterator[AudioChunk]:
        """Yield one chunk per word with deterministic timings."""
        cursor_ms = 0
        for word in text.split():
            timing = WordTiming(word=word, start_ms=cursor_ms, end_ms=cursor_ms + self._ms_per_word)
            cursor_ms = timing.end_ms + self._gap_ms
            yield AudioChunk(audio=(word + " ").encode("utf-8"), timings=(timing,))


if TYPE_CHECKING:
    _tts_impls: tuple[TTSProvider, ...] = (CartesiaTTS(api_key=""), PassthroughTTS())
