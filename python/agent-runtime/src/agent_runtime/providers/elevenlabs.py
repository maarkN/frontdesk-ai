"""ElevenLabs streaming TTS over websocket + deterministic fake with word timings.

Word timings are non-negotiable for any TTS provider here: the gateway's
PlayoutTracker records only what the caller actually HEARD, truncating at word
boundaries on barge-in (CONTEXT.md, concept 3). ElevenLabs reports alignment
per CHARACTER (``alignment`` / ``normalizedAlignment``), so this module
aggregates character timings into :class:`WordTiming` — the same shape
``cartesia.py`` exposes.

Latency: ``eleven_flash_v2_5`` targets the <200 ms first-chunk budget
(CONTEXT.md, concept 1). ``eleven_multilingual_v2`` is the configurable
quality fallback for FR-CA (better French prosody, higher latency).

Residency: ElevenLabs does NOT guarantee Canadian data residency. Bundles
using this provider must be registered with :data:`ELEVENLABS_REGION` so the
:class:`~agent_runtime.providers.base.ProviderFactory` raises
``ResidencyViolationError`` for tenants that require ``ca-central-1`` strictly
(never a silent fallback — CONTEXT.md, concept 7).
"""

from __future__ import annotations

import base64
import json
import os
from collections.abc import AsyncIterator, Callable, Mapping, Sequence
from types import TracebackType
from typing import TYPE_CHECKING, Protocol

from websockets.asyncio.client import connect as _ws_connect

from agent_runtime.models import Locale
from agent_runtime.providers.base import AudioChunk, TTSProvider, WordTiming

_ELEVENLABS_URL = "wss://api.elevenlabs.io/v1/text-to-speech"
_API_KEY_ENV = "ELEVENLABS_API_KEY"

# eleven_flash_v2_5: ~75 ms model latency, fits the <200 ms first-chunk budget.
FLASH_MODEL = "eleven_flash_v2_5"
# eleven_multilingual_v2: quality fallback for FR-CA (better prosody, slower).
MULTILINGUAL_MODEL = "eleven_multilingual_v2"

# ElevenLabs does not guarantee CA data residency; registering a bundle with
# this region makes ProviderFactory refuse strict ca-central-1 tenants.
ELEVENLABS_REGION = "global"

_DEFAULT_MODEL_BY_LOCALE: Mapping[Locale, str] = {
    Locale.EN_CA: FLASH_MODEL,
    Locale.FR_CA: FLASH_MODEL,
}
_DEFAULT_VOICE_BY_LOCALE: Mapping[Locale, str] = {
    # Persona-versioned voice ids (placeholder ids; chosen with the persona).
    Locale.EN_CA: "el-voice-en-ca-frontdesk-v1",
    Locale.FR_CA: "el-voice-fr-ca-frontdesk-v1",
}


def aggregate_word_timings(
    chars: Sequence[str],
    char_start_ms: Sequence[int],
    char_duration_ms: Sequence[int],
    *,
    offset_ms: int = 0,
) -> tuple[WordTiming, ...]:
    """Aggregate ElevenLabs character alignment into per-word timings.

    Words are maximal runs of non-whitespace characters (punctuation and
    accented characters stay attached to their word, mirroring how
    ``PassthroughTTS`` splits on whitespace). Runs of whitespace produce no
    empty words. ``offset_ms`` shifts all timings, letting the caller stitch
    chunk-relative alignments into one monotonic stream.
    """
    words: list[WordTiming] = []
    current: list[str] = []
    start_ms = 0
    end_ms = 0
    for ch, start, duration in zip(chars, char_start_ms, char_duration_ms, strict=True):
        if ch.isspace():
            if current:
                words.append(
                    WordTiming(
                        word="".join(current),
                        start_ms=offset_ms + start_ms,
                        end_ms=offset_ms + end_ms,
                    )
                )
                current = []
            continue
        if not current:
            start_ms = start
        current.append(ch)
        end_ms = start + duration
    if current:
        words.append(
            WordTiming(
                word="".join(current),
                start_ms=offset_ms + start_ms,
                end_ms=offset_ms + end_ms,
            )
        )
    return tuple(words)


class _WSConnection(Protocol):
    """Minimal websocket surface ``ElevenLabsTTS`` needs (injectable in tests)."""

    async def send(self, message: str) -> None: ...

    def __aiter__(self) -> AsyncIterator[str | bytes]: ...


class _WSContext(Protocol):
    """Async context manager yielding a websocket connection."""

    async def __aenter__(self) -> _WSConnection: ...

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc_value: BaseException | None,
        traceback: TracebackType | None,
    ) -> bool | None: ...


_ConnectFn = Callable[[str, dict[str, str]], "_WSContext"]


def _default_connect(url: str, headers: dict[str, str]) -> _WSContext:
    return _ws_connect(url, additional_headers=headers)


class ElevenLabsTTS:
    """Streaming TTS over ElevenLabs' ``stream-input`` websocket API.

    Wire protocol: connect to ``{url}/{voice_id}/stream-input`` with
    ``model_id`` and ``output_format`` query params and the ``xi-api-key``
    header; send a BOS message (single space) with voice settings, the text,
    then an EOS message (empty text). Server messages carry base64 ``audio``
    plus ``normalizedAlignment``/``alignment`` with per-character timings,
    which are aggregated into word timings; ``isFinal`` closes the stream.

    Alignment timestamps are treated as relative to each message's audio and
    stitched into one monotonic timeline via a running offset.

    BYOK: one instance per tenant key — build with :meth:`from_env` for the
    platform key or with the tenant's own key via the provider factory.
    """

    def __init__(
        self,
        api_key: str,
        *,
        url: str = _ELEVENLABS_URL,
        voice_by_locale: Mapping[Locale, str] | None = None,
        model_by_locale: Mapping[Locale, str] | None = None,
        output_format: str = "pcm_16000",
        connect: _ConnectFn = _default_connect,
    ) -> None:
        self._api_key = api_key
        self._url = url.rstrip("/")
        self._voice_by_locale = dict(voice_by_locale or _DEFAULT_VOICE_BY_LOCALE)
        self._model_by_locale = dict(model_by_locale or _DEFAULT_MODEL_BY_LOCALE)
        self._output_format = output_format
        self._connect = connect

    @classmethod
    def from_env(
        cls,
        *,
        voice_by_locale: Mapping[Locale, str] | None = None,
        model_by_locale: Mapping[Locale, str] | None = None,
        output_format: str = "pcm_16000",
    ) -> ElevenLabsTTS:
        """Build from the ``ELEVENLABS_API_KEY`` environment variable.

        Platform-key path; BYOK tenants get an instance built with their own
        key by the provider factory wiring instead.
        """
        api_key = os.environ.get(_API_KEY_ENV, "")
        if not api_key:
            raise KeyError(f"{_API_KEY_ENV} is not set")
        return cls(
            api_key,
            voice_by_locale=voice_by_locale,
            model_by_locale=model_by_locale,
            output_format=output_format,
        )

    def stream_url(self, locale: Locale) -> str:
        """Websocket URL for ``locale`` (voice and model resolved per locale)."""
        voice_id = self._voice_by_locale[locale]
        model_id = self._model_by_locale[locale]
        return (
            f"{self._url}/{voice_id}/stream-input"
            f"?model_id={model_id}&output_format={self._output_format}"
        )

    async def synthesize(self, text: str, *, locale: Locale) -> AsyncIterator[AudioChunk]:
        """Stream ``text`` as audio chunks with aggregated word timings."""
        headers = {"xi-api-key": self._api_key}
        async with self._connect(self.stream_url(locale), headers) as ws:
            # BOS: a single space opens the stream and pins voice settings.
            await ws.send(json.dumps({"text": " ", "voice_settings": {"stability": 0.5}}))
            await ws.send(json.dumps({"text": text + " ", "try_trigger_generation": True}))
            # EOS: empty text flushes and asks the server to finish.
            await ws.send(json.dumps({"text": ""}))

            offset_ms = 0
            async for raw in ws:
                message = json.loads(raw)
                audio_b64 = message.get("audio")
                audio = base64.b64decode(audio_b64) if audio_b64 else b""
                alignment = message.get("normalizedAlignment") or message.get("alignment")
                timings: tuple[WordTiming, ...] = ()
                if alignment:
                    timings = aggregate_word_timings(
                        alignment["chars"],
                        alignment["charStartTimesMs"],
                        alignment["charDurationsMs"],
                        offset_ms=offset_ms,
                    )
                    if timings:
                        offset_ms = timings[-1].end_ms
                if audio or timings:
                    yield AudioChunk(audio=audio, timings=timings)
                if message.get("isFinal"):
                    return


class FakeElevenLabsTTS:
    """Deterministic fake: synthesizes character alignment, then aggregates.

    Unlike ``PassthroughTTS`` (which fakes word timings directly), this fake
    fabricates the CHARACTER-level alignment ElevenLabs would return (every
    character, spaces included, lasts ``ms_per_char`` ms) and funnels it
    through :func:`aggregate_word_timings` — so tests exercise the exact
    aggregation used by the real provider. 'Audio' is the UTF-8 text of the
    words covered by each chunk; ``chunk_words`` words per chunk exercise
    streaming consumers. Calls are recorded for assertions.
    """

    def __init__(self, *, ms_per_char: int = 40, chunk_words: int = 2) -> None:
        self._ms_per_char = ms_per_char
        self._chunk_words = max(1, chunk_words)
        self.calls: list[tuple[str, Locale]] = []

    async def synthesize(self, text: str, *, locale: Locale) -> AsyncIterator[AudioChunk]:
        """Yield chunks of ``chunk_words`` words with aggregated timings."""
        self.calls.append((text, locale))
        chars = list(text)
        starts = [i * self._ms_per_char for i in range(len(chars))]
        durations = [self._ms_per_char] * len(chars)
        timings = aggregate_word_timings(chars, starts, durations)
        for i in range(0, len(timings), self._chunk_words):
            group = timings[i : i + self._chunk_words]
            audio = " ".join(t.word for t in group).encode("utf-8")
            yield AudioChunk(audio=audio, timings=group)


# Protocol compliance checks (uber-go style `var _ Iface = ...`, Python flavor).
if TYPE_CHECKING:
    _tts_impls: tuple[TTSProvider, ...] = (ElevenLabsTTS(api_key=""), FakeElevenLabsTTS())
