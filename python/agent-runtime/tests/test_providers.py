"""Provider Protocols, deterministic fakes, cascade routing, residency (US-3.5/3.6)."""

from __future__ import annotations

import asyncio
import itertools
from collections.abc import AsyncIterator

import pytest

from agent_runtime.models import Locale
from agent_runtime.providers import (
    LLMMessage,
    LLMProvider,
    ModelTier,
    ProviderFactory,
    ResidencyViolationError,
    STTProvider,
    TranscriptEvent,
    TTSProvider,
)
from agent_runtime.providers.cartesia import CartesiaTTS, PassthroughTTS
from agent_runtime.providers.claude import ClaudeLLM, ScriptedLLM, route_tier
from agent_runtime.providers.deepgram import DeepgramSTT, PassthroughSTT


async def _frames(*texts: str) -> AsyncIterator[bytes]:
    for t in texts:
        yield t.encode("utf-8")


def test_passthrough_stt_partials_then_final() -> None:
    async def scenario() -> list[TranscriptEvent]:
        stt = PassthroughSTT()
        return [e async for e in stt.stream(_frames("hello there"), locale=Locale.EN_CA)]

    events = asyncio.run(scenario())
    assert [e.is_final for e in events] == [False, True]
    final = events[-1]
    assert final.text == "hello there"
    assert final.locale is Locale.EN_CA
    assert final.audio_ms > 0


def test_passthrough_tts_word_timings_are_monotonic() -> None:
    async def scenario() -> list[tuple[str, int, int]]:
        tts = PassthroughTTS(ms_per_word=300, gap_ms=50)
        out: list[tuple[str, int, int]] = []
        async for chunk in tts.synthesize("please stay on the line", locale=Locale.EN_CA):
            assert chunk.timings, "every chunk must carry word timings (PlayoutTracker)"
            for t in chunk.timings:
                out.append((t.word, t.start_ms, t.end_ms))
        return out

    timings = asyncio.run(scenario())
    assert [w for w, _, _ in timings] == ["please", "stay", "on", "the", "line"]
    # Strictly increasing, non-overlapping: playout truncation depends on it.
    for (_, s1, e1), (_, s2, _) in itertools.pairwise(timings):
        assert e1 > s1
        assert s2 > e1


def test_scripted_llm_echoes_draft_and_counts_tokens() -> None:
    async def scenario() -> tuple[str, int, int, list[ModelTier]]:
        llm = ScriptedLLM()
        messages = [
            LLMMessage(role="system", content="rules"),
            LLMMessage(role="assistant", content="How can I help you today?"),
        ]
        tokens: list[str] = []
        tin = tout = 0
        async for chunk in llm.stream(messages, tier=ModelTier.REASONING):
            tokens.append(chunk.token)
            if chunk.done:
                tin, tout = chunk.tokens_in, chunk.tokens_out
        return "".join(tokens).strip(), tin, tout, llm.calls

    text, tin, tout, calls = asyncio.run(scenario())
    assert text == "How can I help you today?"
    assert tin > 0
    assert tout == 6
    assert calls == [ModelTier.REASONING]


def test_route_tier_heuristic() -> None:
    # Simple slot answer -> Haiku.
    assert route_tier(user_text="H2X 1Y4", tools_offered=False, reasoning_node=False) is (
        ModelTier.SIMPLE
    )
    # Tool use -> Sonnet.
    assert route_tier(user_text="yes", tools_offered=True, reasoning_node=False) is (
        ModelTier.REASONING
    )
    # Reasoning node -> Sonnet.
    assert route_tier(user_text="ok", tools_offered=False, reasoning_node=True) is (
        ModelTier.REASONING
    )
    # Rambler (long turn) -> Sonnet.
    long_turn = " ".join(["word"] * 25)
    assert route_tier(user_text=long_turn, tools_offered=False, reasoning_node=False) is (
        ModelTier.REASONING
    )


def test_real_providers_are_sketches_not_networked() -> None:
    async def scenario() -> None:
        stt = DeepgramSTT(api_key="k")
        with pytest.raises(NotImplementedError):
            async for _ in stt.stream(_frames("x"), locale=Locale.EN_CA):
                pass
        llm = ClaudeLLM(api_key="k")
        with pytest.raises(NotImplementedError):
            async for _ in llm.stream([LLMMessage(role="user", content="x")]):
                pass
        tts = CartesiaTTS(api_key="k")
        with pytest.raises(NotImplementedError):
            async for _ in tts.synthesize("x", locale=Locale.FR_CA):
                pass

    asyncio.run(scenario())


def _bundle() -> tuple[STTProvider, LLMProvider, TTSProvider]:
    return PassthroughSTT(), ScriptedLLM(), PassthroughTTS()


def test_factory_enforces_residency() -> None:
    factory = ProviderFactory()
    stt, llm, tts = _bundle()
    factory.register("ca", stt=stt, llm=llm, tts=tts, region="ca-central-1", default=True)
    stt2, llm2, tts2 = _bundle()
    factory.register("us", stt=stt2, llm=llm2, tts=tts2, region="us-east-1")

    got = factory.for_tenant("t1", required_region="ca-central-1")
    assert got == (stt, llm, tts)

    factory.assign("t2", "us")
    with pytest.raises(ResidencyViolationError) as exc:
        factory.for_tenant("t2", required_region="ca-central-1")
    assert exc.value.provider == "us"
    assert exc.value.required == "ca-central-1"

    # Tenant without residency requirement may use the US bundle (BYOK).
    assert factory.for_tenant("t2", required_region="") == (stt2, llm2, tts2)
