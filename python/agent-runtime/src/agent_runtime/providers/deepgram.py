"""Deepgram streaming STT (sketch) + deterministic fake for tests.

The real implementation opens a websocket to Deepgram's streaming API and maps
its messages to :class:`TranscriptEvent`. It is intentionally a sketch: tests
never touch the network (EPIC-009 flow sim uses :class:`PassthroughSTT`).
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import TYPE_CHECKING

from agent_runtime.models import Locale
from agent_runtime.providers.base import STTProvider, TranscriptEvent

_DEEPGRAM_URL = "wss://api.deepgram.com/v1/listen"
_MODEL_BY_LOCALE = {
    Locale.EN_CA: "nova-2-general",
    # FR-CA support must be validated against real fixtures before GA (CONTEXT.md).
    Locale.FR_CA: "nova-2-general",
}


class DeepgramSTT:
    """Streaming STT over Deepgram's websocket API (sketch).

    Wire protocol (to be completed):
    - connect to ``_DEEPGRAM_URL`` with ``model``, ``language``, ``encoding=linear16``,
      ``sample_rate=8000``, ``interim_results=true``, ``endpointing=off`` (the
      gateway owns endpointing, EPIC-002);
    - send binary frames as they arrive; send ``{"type": "CloseStream"}`` at end;
    - map ``Results`` messages: ``is_final`` -> final event with confidence and
      audio duration, otherwise partial (hot path only, never the bus).
    """

    def __init__(self, api_key: str, *, url: str = _DEEPGRAM_URL) -> None:
        self._api_key = api_key
        self._url = url

    async def stream(
        self, frames: AsyncIterator[bytes], *, locale: Locale
    ) -> AsyncIterator[TranscriptEvent]:
        """Transcribe frames via Deepgram (not implemented in the MVP runtime)."""
        raise NotImplementedError(
            "DeepgramSTT is a sketch; use PassthroughSTT in tests "
            f"(would connect to {self._url} model={_MODEL_BY_LOCALE[locale]})"
        )
        # Unreachable; keeps the method an async generator for the Protocol.
        yield TranscriptEvent(text="", is_final=True)  # pragma: no cover


class PassthroughSTT:
    """Deterministic fake: each frame is UTF-8 text, echoed as partial then final.

    This is the flow-sim middle layer of note 09: STT becomes a text
    passthrough so a whole conversation runs in milliseconds.
    """

    def __init__(self, *, ms_per_char: int = 60) -> None:
        self._ms_per_char = ms_per_char

    async def stream(
        self, frames: AsyncIterator[bytes], *, locale: Locale
    ) -> AsyncIterator[TranscriptEvent]:
        """Yield a partial and a final transcript per text frame."""
        async for frame in frames:
            text = frame.decode("utf-8")
            if not text:
                continue
            yield TranscriptEvent(text=text[: max(1, len(text) // 2)], is_final=False)
            yield TranscriptEvent(
                text=text,
                is_final=True,
                locale=locale,
                confidence=0.99,
                audio_ms=len(text) * self._ms_per_char,
            )


# Protocol compliance checks (uber-go style `var _ Iface = ...`, Python flavor).
if TYPE_CHECKING:
    _stt_impls: tuple[STTProvider, ...] = (DeepgramSTT(api_key=""), PassthroughSTT())
