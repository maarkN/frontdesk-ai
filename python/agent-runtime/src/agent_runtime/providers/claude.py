"""Claude LLM provider: cost cascade Haiku/Sonnet (US-3.6) + deterministic fake.

Cascade heuristic (explicit and testable — every routing decision is recorded
per turn in ``agent.turn.completed`` emission metadata):

Route to **Sonnet** (``ModelTier.REASONING``) when ANY of:
1. tools are offered this turn (scheduling, message taking — tool use needs
   reliable function calling);
2. the user turn is long (> ``_LONG_TURN_WORDS`` words — ramblers need real
   summarization, not slot filling);
3. the node is a reasoning node (intent disambiguation after a failed attempt,
   out-of-coverage negotiation).

Everything else — greeting, confirmations, single-slot collection — routes to
**Haiku** (``ModelTier.SIMPLE``). Simple turns dominate the distribution, so
this is where the cost target (G6: variable cost < 30% of price/min) is won.
"""

from __future__ import annotations

from collections.abc import AsyncIterator, Sequence
from typing import TYPE_CHECKING

from agent_runtime.providers.base import (
    LLMChunk,
    LLMMessage,
    LLMProvider,
    ModelTier,
    ToolSpec,
)

_MODEL_BY_TIER = {
    ModelTier.SIMPLE: "claude-haiku-4-5",
    ModelTier.REASONING: "claude-sonnet-4-5",
}
_LONG_TURN_WORDS = 18


def route_tier(
    *,
    user_text: str,
    tools_offered: bool,
    reasoning_node: bool,
) -> ModelTier:
    """Pick the cascade tier for one turn. Pure function: unit-testable."""
    if tools_offered or reasoning_node:
        return ModelTier.REASONING
    if len(user_text.split()) > _LONG_TURN_WORDS:
        return ModelTier.REASONING
    return ModelTier.SIMPLE


class ClaudeLLM:
    """Claude streaming provider via ``langchain-anthropic`` (sketch).

    The real implementation builds a ``ChatAnthropic`` per tier (see
    ``_MODEL_BY_TIER``), binds tools, and adapts ``astream`` events to
    :class:`LLMChunk`. Kept as a sketch: CI never calls model APIs; the
    EPIC-006 hedge/circuit-breaker composes around this same Protocol.
    """

    def __init__(self, api_key: str, *, region: str = "ca-central-1") -> None:
        self._api_key = api_key
        self._region = region

    async def stream(
        self,
        messages: Sequence[LLMMessage],
        *,
        tools: Sequence[ToolSpec] = (),
        tier: ModelTier = ModelTier.SIMPLE,
    ) -> AsyncIterator[LLMChunk]:
        """Stream a completion from Claude (not implemented in the MVP runtime)."""
        raise NotImplementedError(
            f"ClaudeLLM is a sketch; use ScriptedLLM in tests (would call {_MODEL_BY_TIER[tier]})"
        )
        yield LLMChunk(done=True)  # pragma: no cover


class ScriptedLLM:
    """Deterministic fake: streams back the draft the graph composed.

    The graph's dialogue policy is deterministic (templates + slot filling);
    the LLM's job in flow sim is to be a transparent pipe so assertions stay
    stable. The final ``done`` chunk carries token accounting derived from
    text length, so billing folds have non-zero, reproducible numbers.
    """

    def __init__(self) -> None:
        self.calls: list[ModelTier] = []

    async def stream(
        self,
        messages: Sequence[LLMMessage],
        *,
        tools: Sequence[ToolSpec] = (),
        tier: ModelTier = ModelTier.SIMPLE,
    ) -> AsyncIterator[LLMChunk]:
        """Echo the last assistant-draft message token by token."""
        self.calls.append(tier)
        draft = messages[-1].content if messages else ""
        for word in draft.split(" "):
            yield LLMChunk(token=word + " ")
        tokens_in = sum(len(m.content.split()) for m in messages)
        yield LLMChunk(done=True, tokens_in=tokens_in, tokens_out=len(draft.split()))


if TYPE_CHECKING:
    _llm_impls: tuple[LLMProvider, ...] = (ClaudeLLM(api_key=""), ScriptedLLM())
