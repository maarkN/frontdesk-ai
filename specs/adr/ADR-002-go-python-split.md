# ADR-002 — Split de linguagens: Go no caminho quente, Python no agente

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** notas 02, 10, 16 do vault `maarkN/Voice AI/`; CONTEXT.md
- **Depende de:** ADR-001

## Contexto

O sistema tem dois regimes de execução com exigências opostas:

- **Caminho quente de mídia e controle**: frames de áudio de 20 ms, uma chamada = uma máquina de
  estados concorrente, barge-in com ducking em ~60 ms, orçamento de turno p50 < 1,2 s. Pausas de
  GC ou um event loop bloqueado quebram o stream para *todas* as chamadas.
- **Orquestração do agente**: grafo de conversa, tool use, cascata de modelos, evals. Aqui o que
  manda é o ecossistema de IA — LangGraph, SDKs de STT/LLM/TTS, tooling de avaliação — que é
  Python-first.

A referência (nota 02) analisou o problema para o par Go/TypeScript e concluiu por um híbrido:
Go no daemon de controle de chamada e em tudo que toca mídia (goroutines mapeiam 1:1 com
chamadas; código sequencial com `context.Context` em vez de cascata de callbacks; AudioSocket
com frames de 20 ms "Go ganha com folga"), TypeScript na API de negócio, painel e billing. E
alertou: "Go se houver qualquer processamento de áudio em tempo real no roadmap".

O CRM imobiliário (nota 10) decidiu o oposto — **Go-only** — porque o CRM já era Go e eliminar o
cross-language mata a categoria de bug "drift de tipos" e remove `proto/`, `buf` e o CI que os
valida.

O Receptionist não tem base Go existente nem softphone TS que justifique TS no backend, mas tem
um agente LangGraph no centro do produto.

## Decisão

**Go** nos serviços onde latência e concorrência mandam:

- `telephony-gw` — call control, media WS com Telnyx, jitter, barge-in, máquina de estados do
  turno (1 goroutine por chamada, loop `select`, cancelamento em cascata via `context.Context`).
- `core-api` — API de negócio, multi-tenancy (RLS), onboarding, billing.
- `notifier` — SMS/e-mail pós-chamada (SLA de SMS ao dono < 60 s).

**Python** (3.12+) no serviço onde o ecossistema de IA manda:

- `agent-runtime` — grafo do agente em LangGraph, tools (Google Calendar), STT/LLM/TTS atrás de
  `Protocol`, cascata de custo (Haiku/Sonnet), evals.

TypeScript fica restrito ao **frontend** (web React + mobile Expo), fora do caminho de voz.
Contratos Go↔Python: JSON com schemas versionados em `contracts/` (sem protobuf no MVP).

## Consequências

**Positivas**
- Cada regime roda na linguagem certa: sem GC pause/event loop no caminho de 20 ms; sem
  reimplementar LangGraph e SDKs de IA em Go.
- O corte entre linguagens coincide com um corte natural de processo (gateway ↔ agente via NATS),
  então o custo do híbrido é pago numa fronteira que existiria de qualquer forma.
- Padrões existentes reutilizados (nota 16): DDD/outbox/OTel do imobitech-crm no lado Go;
  `Protocol` para provedores e cascata de custo do jobradar no lado Python.
- A fronteira força o contrato de eventos a ser explícito e versionado (ver ADR-003).

**Negativas / riscos assumidos**
- Reintroduz o cross-language que o CRM eliminou de propósito: risco de drift de tipos entre Go
  e Python. Mitigação: schemas JSON versionados em `contracts/` como fonte única, validados nos
  dois lados em CI — o problema é aceito e gerenciado, não ignorado.
- Dois toolchains, dois pipelines de CI, dois conjuntos de convenções (uber-go/guide; `uv` +
  `ruff` + `mypy` strict).
- O agente em Python impõe uma fronteira de rede dentro do orçamento de latência do turno; o
  orçamento (STT final ~150 ms, LLM 1º token ~400 ms, TTS 1º chunk ~200 ms) precisa absorver o
  hop gateway↔runtime.

## Alternativas consideradas

1. **Híbrido Go + TypeScript, como a referência (nota 02).** Rejeitada: o veredito da referência
   depende de um contexto que não existe aqui — softphone WebRTC e painel compartilhando tipos
   com o backend TS. No Receptionist o centro do backend não-mídia é o agente, e o ecossistema
   de agentes (LangGraph, SDKs, evals) é Python, não TS. TS permanece, mas só no frontend.
2. **Go-only, como o CRM (nota 10).** Rejeitada: a justificativa do CRM era uma base Go
   existente ("o CRM já é Go") e a eliminação do custo cross-language. Aqui não há base a
   preservar, e o custo de reimplementar orquestração de agente, integrações e evals em Go
   supera o custo do contrato Go↔Python.
3. **Python-only.** Rejeitada: o caminho de mídia (frames de 20 ms, barge-in ~60 ms, milhares de
   máquinas de estado concorrentes) é exatamente o cenário em que GIL/GC/asyncio degradam; a
   referência já havia descartado até o Node por motivo análogo (event loop único, pausas de GC
   quebram streams de 20 ms).
4. **Go + Python com gRPC/protobuf entre eles.** Adiada: protobuf resolve drift de tipos, mas
   adiciona toolchain (`buf`, codegen) que a nota 10 lista como custo real. Para o MVP, JSON com
   schema versionado é suficiente; revisitar se o contrato crescer.
