"""Provider Protocols and shared datatypes (US-3.5).

Contracts:
- STT: audio frames in, partial/final transcripts out (partials stay in the
  hot path — they NEVER reach the event bus, ADR-003).
- LLM: messages in, streamed tokens + tool calls out.
- TTS: text in, audio chunks WITH word timings out — word timings are a hard
  requirement: the gateway's PlayoutTracker truncates history at the last word
  the caller actually heard.

Providers are chosen per tenant by :class:`ProviderFactory` (BYOK-ready) which
enforces data residency: a provider outside the tenant's required region raises
:class:`ResidencyViolationError` at runtime (never a silent fallback).
"""

from __future__ import annotations

import enum
from collections.abc import AsyncIterator, Sequence
from typing import Any, Protocol, runtime_checkable

from pydantic import BaseModel, ConfigDict, Field

from agent_runtime.models import Locale


class ResidencyViolationError(Exception):
    """A provider would process tenant data outside the required region."""

    def __init__(self, provider: str, region: str, required: str) -> None:
        super().__init__(f"provider {provider!r} runs in {region!r}; tenant requires {required!r}")
        self.provider = provider
        self.region = region
        self.required = required


class TranscriptEvent(BaseModel):
    """One STT emission. ``is_final=False`` events never leave the hot path."""

    model_config = ConfigDict(frozen=True)

    text: str
    is_final: bool
    locale: Locale | None = None
    confidence: float = Field(default=1.0, ge=0.0, le=1.0)
    audio_ms: int = Field(default=0, ge=0)


class LLMMessage(BaseModel):
    """Chat message for the LLM provider."""

    model_config = ConfigDict(frozen=True)

    role: str  # "system" | "user" | "assistant" | "tool"
    content: str


class ToolSpec(BaseModel):
    """Tool made available to the LLM for a given turn."""

    model_config = ConfigDict(frozen=True)

    name: str
    description: str
    parameters: dict[str, Any] = Field(default_factory=dict)


class ToolCall(BaseModel):
    """Tool call requested by the LLM."""

    model_config = ConfigDict(frozen=True)

    name: str
    args: dict[str, Any] = Field(default_factory=dict)


class LLMChunk(BaseModel):
    """One streamed LLM chunk: a token, and/or a tool call, and usage at the end."""

    model_config = ConfigDict(frozen=True)

    token: str = ""
    tool_call: ToolCall | None = None
    done: bool = False
    tokens_in: int = Field(default=0, ge=0)
    tokens_out: int = Field(default=0, ge=0)


class WordTiming(BaseModel):
    """Playout timing of one synthesized word (PlayoutTracker requirement)."""

    model_config = ConfigDict(frozen=True)

    word: str
    start_ms: int = Field(ge=0)
    end_ms: int = Field(ge=0)


class AudioChunk(BaseModel):
    """One streamed TTS chunk with the word timings it covers."""

    model_config = ConfigDict(frozen=True)

    audio: bytes
    timings: tuple[WordTiming, ...] = ()


class ModelTier(enum.StrEnum):
    """Cost-cascade tier chosen per turn (US-3.6)."""

    SIMPLE = "haiku"
    REASONING = "sonnet"


@runtime_checkable
class STTProvider(Protocol):
    """Streaming speech-to-text: frames in, partial/final transcripts out."""

    def stream(
        self, frames: AsyncIterator[bytes], *, locale: Locale
    ) -> AsyncIterator[TranscriptEvent]:
        """Transcribe an audio frame stream into transcript events."""
        ...


@runtime_checkable
class LLMProvider(Protocol):
    """Streaming chat completion with tool calling."""

    def stream(
        self,
        messages: Sequence[LLMMessage],
        *,
        tools: Sequence[ToolSpec] = (),
        tier: ModelTier = ModelTier.SIMPLE,
    ) -> AsyncIterator[LLMChunk]:
        """Stream completion chunks for ``messages``."""
        ...


@runtime_checkable
class TTSProvider(Protocol):
    """Streaming text-to-speech; every chunk carries word timings."""

    def synthesize(self, text: str, *, locale: Locale) -> AsyncIterator[AudioChunk]:
        """Synthesize ``text`` into audio chunks with word timings."""
        ...


class ProviderInfo(BaseModel):
    """Deployment metadata used for residency validation."""

    model_config = ConfigDict(frozen=True)

    name: str
    region: str


class _ProviderBundle(BaseModel):
    model_config = ConfigDict(frozen=True, arbitrary_types_allowed=True)

    stt: Any
    llm: Any
    tts: Any
    info: ProviderInfo


class ProviderFactory:
    """Per-tenant provider selection with region validation (BYOK-ready).

    Register named bundles once at startup; ``for_tenant`` picks the bundle for
    a tenant and refuses any bundle whose region violates the tenant's
    residency requirement.
    """

    def __init__(self) -> None:
        self._bundles: dict[str, _ProviderBundle] = {}
        self._tenant_bundle: dict[str, str] = {}
        self._default: str | None = None

    def register(
        self,
        name: str,
        *,
        stt: STTProvider,
        llm: LLMProvider,
        tts: TTSProvider,
        region: str,
        default: bool = False,
    ) -> None:
        """Register a provider bundle under ``name``."""
        self._bundles[name] = _ProviderBundle(
            stt=stt, llm=llm, tts=tts, info=ProviderInfo(name=name, region=region)
        )
        if default or self._default is None:
            self._default = name

    def assign(self, tenant_id: str, bundle: str) -> None:
        """Pin a tenant to a specific bundle (BYOK)."""
        if bundle not in self._bundles:
            raise KeyError(f"unknown provider bundle: {bundle!r}")
        self._tenant_bundle[tenant_id] = bundle

    def for_tenant(
        self, tenant_id: str, *, required_region: str
    ) -> tuple[STTProvider, LLMProvider, TTSProvider]:
        """Resolve providers for a tenant, enforcing residency."""
        name = self._tenant_bundle.get(tenant_id) or self._default
        if name is None:
            raise KeyError("no provider bundle registered")
        bundle = self._bundles[name]
        if required_region and bundle.info.region != required_region:
            raise ResidencyViolationError(name, bundle.info.region, required_region)
        stt: STTProvider = bundle.stt
        llm: LLMProvider = bundle.llm
        tts: TTSProvider = bundle.tts
        return stt, llm, tts
