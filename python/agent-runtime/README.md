# agent-runtime

Runtime do agente FrontDesk AI (EPIC-003, ADR-002). Python 3.12+, `uv`.

## O que vive aqui

- `src/agent_runtime/graph.py` — grafo LangGraph do fluxo RF2 (saudação → intenção →
  qualificação → agendamento/recado → confirmação), com detecção EN/FR no 1º turno e troca de
  idioma no meio da chamada **sem reiniciar o nó** (o nó é dado no estado, não posição de código).
- `src/agent_runtime/providers/` — STT/LLM/TTS atrás de `Protocol` (US-3.5): Deepgram, Claude
  (cascata Haiku/Sonnet com heurística explícita em `claude.route_tier`), Cartesia (word timings
  obrigatórios — PlayoutTracker). Impls reais esboçadas; fakes determinísticos completos.
- `src/agent_runtime/guardrails.py` — sem preço fechado, sem promessa fora da config, emergência
  EN/FR → transferir para o celular do dono, resistência a prompt injection (system prompt
  imutável + triagem de entrada e saída).
- `src/agent_runtime/events.py` — envelope de `contracts/events/v1` com `seq` monotônico, ULID,
  PII redigida NA EMISSÃO (Luhn, SIN, código postal, data de nascimento — EN e FR).
  `stt.partial` não tem caminho de emissão.
- `src/agent_runtime/bridge.py` — ponte telephony-gw (WS) ↔ grafo ↔ NATS (transportes atrás de
  `Protocol`; fakes em memória nos testes).
- `tests/` — flow sim em texto (camada do meio da nota 09): caso feliz EN, recado FR,
  emergência→transfer, code_switcher, off_topic, pii_dumper, prompt_injector.

## Comandos

```sh
uv sync          # instala deps + o pacote em modo editável
uv run pytest    # sem rede, sem serviços externos
uv run ruff check .
uv run mypy
```
