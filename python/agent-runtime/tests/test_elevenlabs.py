"""ElevenLabs TTS: char->word aggregation, streaming, voices, residency."""

from __future__ import annotations

import asyncio
import base64
import itertools
import json
from collections.abc import AsyncIterator
from types import TracebackType

import pytest

from agent_runtime.models import Locale
from agent_runtime.providers import ProviderFactory, ResidencyViolationError
from agent_runtime.providers.claude import ScriptedLLM
from agent_runtime.providers.deepgram import PassthroughSTT
from agent_runtime.providers.elevenlabs import (
    ELEVENLABS_REGION,
    MULTILINGUAL_MODEL,
    ElevenLabsTTS,
    FakeElevenLabsTTS,
    aggregate_word_timings,
)


def _alignment(text: str, *, ms_per_char: int = 10) -> tuple[list[str], list[int], list[int]]:
    chars = list(text)
    starts = [i * ms_per_char for i in range(len(chars))]
    durations = [ms_per_char] * len(chars)
    return chars, starts, durations


def test_aggregate_keeps_punctuation_attached() -> None:
    chars, starts, durations = _alignment("Hi, there!")
    timings = aggregate_word_timings(chars, starts, durations)
    assert [t.word for t in timings] == ["Hi,", "there!"]
    assert timings[0].start_ms == 0
    assert timings[0].end_ms == 30  # "Hi," = 3 chars x 10 ms
    assert timings[1].start_ms == 40
    assert timings[1].end_ms == 100


def test_aggregate_handles_french_accents() -> None:
    chars, starts, durations = _alignment("Ça va très bien, allô?")
    timings = aggregate_word_timings(chars, starts, durations)
    assert [t.word for t in timings] == ["Ça", "va", "très", "bien,", "allô?"]


def test_aggregate_collapses_multiple_spaces_and_edges() -> None:
    chars, starts, durations = _alignment("  bonjour   le  monde  ")
    timings = aggregate_word_timings(chars, starts, durations)
    assert [t.word for t in timings] == ["bonjour", "le", "monde"]
    # No empty words; timings stay strictly increasing and non-overlapping.
    for a, b in itertools.pairwise(timings):
        assert a.end_ms > a.start_ms
        assert b.start_ms > a.end_ms


def test_aggregate_applies_offset() -> None:
    chars, starts, durations = _alignment("ok")
    timings = aggregate_word_timings(chars, starts, durations, offset_ms=500)
    assert timings[0].start_ms == 500
    assert timings[0].end_ms == 520


def test_aggregate_rejects_mismatched_lengths() -> None:
    with pytest.raises(ValueError, match="shorter"):
        aggregate_word_timings(["a", "b"], [0], [10, 10])


def test_fake_streams_chunks_with_word_timings() -> None:
    async def scenario() -> list[tuple[bytes, tuple[str, ...]]]:
        tts = FakeElevenLabsTTS(ms_per_char=40, chunk_words=2)
        out: list[tuple[bytes, tuple[str, ...]]] = []
        async for chunk in tts.synthesize("please stay on the line", locale=Locale.EN_CA):
            assert chunk.timings, "every chunk must carry word timings (PlayoutTracker)"
            out.append((chunk.audio, tuple(t.word for t in chunk.timings)))
        return out

    chunks = asyncio.run(scenario())
    assert [words for _, words in chunks] == [("please", "stay"), ("on", "the"), ("line",)]
    assert [audio for audio, _ in chunks] == [b"please stay", b"on the", b"line"]


def test_fake_timings_are_monotonic_and_calls_recorded() -> None:
    async def scenario() -> tuple[list[tuple[int, int]], list[tuple[str, Locale]]]:
        tts = FakeElevenLabsTTS()
        spans: list[tuple[int, int]] = []
        async for chunk in tts.synthesize("un deux trois", locale=Locale.FR_CA):
            spans.extend((t.start_ms, t.end_ms) for t in chunk.timings)
        return spans, tts.calls

    spans, calls = asyncio.run(scenario())
    for (s1, e1), (s2, _) in itertools.pairwise(spans):
        assert e1 > s1
        assert s2 > e1
    assert calls == [("un deux trois", Locale.FR_CA)]


class _FakeWS:
    """Scripted server: records sent messages, replays canned frames."""

    def __init__(self, frames: list[str]) -> None:
        self.sent: list[str] = []
        self._frames = frames

    async def send(self, message: str) -> None:
        self.sent.append(message)

    def __aiter__(self) -> AsyncIterator[str | bytes]:
        async def gen() -> AsyncIterator[str | bytes]:
            for frame in self._frames:
                yield frame

        return gen()

    async def __aenter__(self) -> _FakeWS:
        return self

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc_value: BaseException | None,
        traceback: TracebackType | None,
    ) -> bool | None:
        return None


class _FakeConnect:
    def __init__(self, frames: list[str]) -> None:
        self.ws = _FakeWS(frames)
        self.url = ""
        self.headers: dict[str, str] = {}

    def __call__(self, url: str, headers: dict[str, str]) -> _FakeWS:
        self.url = url
        self.headers = headers
        return self.ws


def _frame(
    text_with_timings: str | None,
    *,
    audio: bytes = b"",
    key: str = "normalizedAlignment",
    is_final: bool = False,
) -> str:
    payload: dict[str, object] = {}
    if audio:
        payload["audio"] = base64.b64encode(audio).decode("ascii")
    if text_with_timings is not None:
        chars, starts, durations = _alignment(text_with_timings)
        payload[key] = {
            "chars": chars,
            "charStartTimesMs": starts,
            "charDurationsMs": durations,
        }
    if is_final:
        payload["isFinal"] = True
    return json.dumps(payload)


def test_provider_parses_frames_and_stitches_offsets() -> None:
    frames = [
        _frame("hello ", audio=b"\x01\x02"),
        # Chunk-relative timestamps restart at 0; falls back to "alignment".
        _frame("world", audio=b"\x03", key="alignment"),
        _frame(None, is_final=True),
    ]
    connect = _FakeConnect(frames)
    tts = ElevenLabsTTS(api_key="k-tenant", connect=connect)

    async def scenario() -> list[tuple[bytes, tuple[str, ...], tuple[int, ...]]]:
        return [
            (
                chunk.audio,
                tuple(t.word for t in chunk.timings),
                tuple(t.start_ms for t in chunk.timings),
            )
            async for chunk in tts.synthesize("hello world", locale=Locale.EN_CA)
        ]

    chunks = asyncio.run(scenario())
    assert [(a, w) for a, w, _ in chunks] == [
        (b"\x01\x02", ("hello",)),
        (b"\x03", ("world",)),
    ]
    # Second chunk's word is shifted by the first chunk's end (50 ms).
    assert chunks[1][2] == (50,)

    # Handshake: BOS (space) -> text -> EOS (empty).
    sent = [json.loads(m) for m in connect.ws.sent]
    assert sent[0]["text"] == " "
    assert sent[1]["text"] == "hello world "
    assert sent[2]["text"] == ""
    assert connect.headers == {"xi-api-key": "k-tenant"}


def test_voice_and_model_selected_per_locale() -> None:
    tts = ElevenLabsTTS(
        api_key="k",
        voice_by_locale={Locale.EN_CA: "voice-en", Locale.FR_CA: "voice-fr"},
        # FR-CA quality fallback is a config choice, not a code change.
        model_by_locale={
            Locale.EN_CA: "eleven_flash_v2_5",
            Locale.FR_CA: MULTILINGUAL_MODEL,
        },
    )
    en_url = tts.stream_url(Locale.EN_CA)
    fr_url = tts.stream_url(Locale.FR_CA)
    assert "/voice-en/stream-input" in en_url
    assert "model_id=eleven_flash_v2_5" in en_url
    assert "/voice-fr/stream-input" in fr_url
    assert f"model_id={MULTILINGUAL_MODEL}" in fr_url


def test_elevenlabs_bundle_violates_strict_ca_residency() -> None:
    factory = ProviderFactory()
    factory.register(
        "elevenlabs",
        stt=PassthroughSTT(),
        llm=ScriptedLLM(),
        tts=FakeElevenLabsTTS(),
        region=ELEVENLABS_REGION,
        default=True,
    )
    with pytest.raises(ResidencyViolationError) as exc:
        factory.for_tenant("t-strict", required_region="ca-central-1")
    assert exc.value.provider == "elevenlabs"
    assert exc.value.region == ELEVENLABS_REGION
    # A tenant without a strict requirement may opt in (BYOK).
    _, _, tts = factory.for_tenant("t-open", required_region="")
    assert isinstance(tts, FakeElevenLabsTTS)
