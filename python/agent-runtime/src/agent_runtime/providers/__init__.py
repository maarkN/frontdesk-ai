"""STT/LLM/TTS providers behind Protocols (CONTEXT.md, concept 7).

Real vendors (Deepgram, Claude, Cartesia) are sketched; deterministic fakes
back every test — nothing in this package touches the network under pytest.
"""

from agent_runtime.providers.base import (
    AudioChunk,
    LLMChunk,
    LLMMessage,
    LLMProvider,
    ModelTier,
    ProviderFactory,
    ProviderInfo,
    ResidencyViolationError,
    STTProvider,
    ToolCall,
    ToolSpec,
    TranscriptEvent,
    TTSProvider,
    WordTiming,
)

__all__ = [
    "AudioChunk",
    "LLMChunk",
    "LLMMessage",
    "LLMProvider",
    "ModelTier",
    "ProviderFactory",
    "ProviderInfo",
    "ResidencyViolationError",
    "STTProvider",
    "TTSProvider",
    "ToolCall",
    "ToolSpec",
    "TranscriptEvent",
    "WordTiming",
]
