# CONTEXT — Digest do domínio Voice AI (fonte: vault Obsidian `maarkN/Voice AI/`)

> Este arquivo é o digest canônico para agentes de implementação. Fonte original:
> `/Users/mac/Documents/Obsidian/maarkN/Voice AI/` (notas 00–17). Em conflito, as notas vencem.

## O que estamos construindo

**FrontDesk AI** — AI Voice Receptionist SaaS para PMEs de serviço no Canadá (trades: encanador,
eletricista, HVAC; clínicas como vertical 2). Atende a linha telefônica em segundos, bilíngue
EN/FR-CA, qualifica (serviço, código postal, urgência), agenda no Google Calendar ou registra
recado estruturado, envia SMS ao dono (<60s) e ao cliente. Modo padrão: **transbordo** — a IA só
entra se ninguém atender em X segundos.

Decisão de escopo desta implementação (delta vs PRD original): **inclui app web E mobile**
(pedido explícito do owner), React + React Native (Expo SDK 54), TypeScript.

## Arquitetura (microserviços)

```
PSTN ── Telnyx ──▶ telephony-gw (Go)  — call control, media WS, jitter, barge-in, turn state machine
                        │ audio frames + call events (NATS JetStream)
                   agent-runtime (Python) — LangGraph, tools, STT/LLM/TTS atrás de Protocols
                        │
        core-api (Go) · notifier (Go, SMS/e-mail) · dashboard (web/mobile TS)
        Postgres · Redis · S3 · OTel — AWS ca-central-1 (residência de dados)
```

Divisão deliberada: **Go onde latência/concorrência mandam** (gateway de mídia, call control,
API, notificações); **Python onde o ecossistema de IA manda** (agent-runtime: LangGraph, SDKs,
evals, cascata de custo).

## Decisões-chave (MVP)

| Decisão | Escolha |
|---|---|
| Carrier | **Telnyx** (media streams por WS; sem Asterisk próprio — Asterisk é alavanca de margem v2) |
| STT | Deepgram streaming (validar FR-CA) |
| LLM | Claude em cascata (Haiku turnos simples, Sonnet raciocínio) |
| TTS | Cartesia ou ElevenLabs Flash (<200ms; word timings obrigatórios p/ PlayoutTracker) |
| Orquestração agente | LangGraph |
| Bus de eventos | NATS JetStream |
| Banco | Postgres (RLS por tenant), Redis, S3 |

## Conceitos transversais obrigatórios

1. **Orçamento de latência por turno**: percebido p50 < 1,2s, p95 < 2,5s. Decomposição-alvo:
   VAD→fim de fala ~200ms · STT final ~150ms · LLM 1º token ~400ms · TTS 1º chunk ~200ms.
   Consequências: TTS streaming, cache de frases fixas (banco de áudio), filler (>600ms), barge-in.
2. **Eventos como fonte da verdade**: `state = fold(events)`. Envelope: `eventId` (ULID),
   `callId`, `seq` monotônico por chamada (dedup/ordenação — NATS é at-least-once), `ts`,
   `tenantId`, `flowId`+`flowVersion`, `type`, `nodeId`, `payload`. Catálogo: call.started/
   answered/ended, consent.captured, language.detected/switched, node.entered/exited, stt.final,
   agent.turn.completed, tool.invoked/completed, var.assigned, dtmf.received, transfer.initiated/
   answered, recording.started, error.raised. **`stt.partial` nunca vai ao bus.** PII é redigida
   NA EMISSÃO (`redactions: [{start,end,kind,vault}]`) — detectores: cartão (Luhn), SIN, código
   postal `A1A 1A1`, data de nascimento, nos DOIS idiomas. Billing = fold sobre o stream
   (CallMinutes, STTSeconds, LLMTokensIn/Out, TTSChars).
3. **Máquina de estados do turno** (1 goroutine por chamada, loop `select`):
   `stListening → stEndpointing → stThinking → stSpeaking → stListening`. O STT recebe frames em
   TODOS os estados (é o que permite barge-in). Quatro problemas difíceis:
   - **Endpointing adaptativo por idioma**: hesitação ("um/uh" EN; "euh","ben","tsé" FR-CA) →
     ~1400ms; frase incompleta (termina em preposição) → ~900ms; ditando números → ~1100ms;
     pergunta → ~400ms; default ~600ms.
   - **Barge-in em dois níveis**: energia → ducking (~60ms) → STT parcial confirma. Backchannel
     ("uh huh","yeah"/"ouais","han han") desfaz ducking e NÃO corta. Texto real (≥minChars):
     cancela LLM, para TTS, FlushPlayback, histórico só com o que foi ouvido.
   - **PlayoutTracker**: histórico grava só o que o cliente OUVIU — `Spoken()` trunca na palavra
     via word timings do TTS, marca corte com "—"; `writtenMs` ≠ ouvido (offset 100–200ms).
   - **Cancelamento em cascata**: `context.Context` + produtor com
     `select { case ch <- tok: case <-ctx.Done(): return }` (sem vazamento de goroutine).
4. **Escada de degradação** (orçamento de falha em ms de silêncio, NUNCA retry com backoff
   exponencial): `degNormal → degFast (modelo menor) → degScripted (enlatadas + DTMF) →
   degTransfer (humano com contexto) → degVoicemail`. STT: ring buffer + reconexão 0/80/200ms +
   fallback de fornecedor. LLM: **hedge, não retry** (secundário após ~700ms, usa quem responder
   primeiro). TTS falha → banco de áudio pré-sintetizado (~15 frases por locale, versionadas com a
   persona). Circuit breaker POR PROVEDOR (nunca por chamada), meio-aberto 1 sonda/5s.
   **Failover de produto: agente falhou → chamada cai no celular do dono, nunca em silêncio.**
5. **Multi-tenancy**: tenant resolvido no início da chamada pelo DID (desconhecido → hangup);
   entra no `context.Context` e NENHUMA função abaixo aceita `tenantID` como parâmetro. Postgres
   RLS: `set LOCAL app.tenant_id` dentro da transação + `force row level security`. Limites por
   tenant (semáforo de chamadas, rate limit de tokens/segundos). Crypto-shredding:
   KEK (por tenant) → DEK (por chamada) → áudio+transcrição; exclusão = destruir DEK.
6. **Consent primeiro**: aviso de gravação como gate antes de gravar/agente (PIPEDA/Lei 25).
7. **Provider como interface**: STT/LLM/TTS sempre atrás de abstração (Go: interfaces pequenas;
   Python: `Protocol`), com factory por tenant (BYOK futuro) e validação de região
   (`ErrResidencyViolation` em runtime).
8. **Guardrails do agente**: não cotar preço fechado, não prometer fora da config,
   emergência ("flood/burst pipe") → transferir para o celular do dono. Detecção EN/FR no 1º
   turno (meta ≥95%), troca de idioma no meio da chamada sem reiniciar o nó.
9. **Evals em três camadas**: unit (máquina de estados, WAV sintético), flow sim (texto,
   passthrough de STT/TTS, ~300 casos/PR), voice regression (noturno). Clientes adversariais
   (code_switcher, impatient, rambler, off_topic, pii_dumper, prompt_injector, voicemail).
   Asserções determinísticas antes de LLM-as-judge. `piiLeaks > 0` bloqueia publish.

## Requisitos funcionais do MVP (RF1–RF4)

- **RF1 Telefonia**: número local; modos transbordo e sempre-IA (fora do expediente);
  transferência a quente por regra (emergência/palavra-chave → celular do dono); gravação +
  transcrição com aviso.
- **RF2 Agente**: detecção EN/FR 1º turno; saudação → intenção → qualificação (serviço, código
  postal vs área de cobertura, urgência) → agendamento/recado → confirmação; tool use com slots
  reais (Google Calendar); guardrails; barge-in.
- **RF3 Pós-chamada**: SMS ao dono <60s (quem, o quê, urgência, telefone, transcrição); SMS ao
  cliente; relatório semanal por e-mail.
- **RF4 Plataforma**: onboarding self-service ≤10min ("importar do meu site": LLM extrai FAQ);
  dashboard com chamadas/áudio/transcrição e toggle de modo; Stripe (trial 14 dias);
  multi-tenant, dados em ca-central-1.

## Metas mensuráveis

G1 turno p50<1,2s p95<2,5s · G2 ≥70% resolvem sem humano · G3 detecção idioma ≥95% ·
G4 SMS<60s + relatório semanal · G5 uptime ≥99,9% + failover para celular do dono ·
G6 custo variável/min < 30% do preço/min.

## Pricing (hipótese)

Starter CAD 149/mês (200 min) · Pro CAD 249 (500 min, calendário, FR+EN, relatório) ·
Growth CAD 399 (1.000 min, multi-calendário, Jobber) · excedente ~CAD 0,35/min.

## Convenções de código

- **Go**: seguir [uber-go/guide](https://github.com/uber-go/guide) integralmente. Destaques:
  erros com `%w` e `errors.Is/As`; accept interfaces, return structs; interfaces pequenas
  definidas no consumidor; zero-value útil; sem `init()`; `var _ Iface = (*Impl)(nil)` para
  compliance; goroutines com dono e ciclo de vida claro (nunca "fire and forget");
  channels com tamanho 0 ou 1; guard clauses / early return; sem embutir mutex em struct pública;
  functional options para configuração; nomes de pacote curtos, sem `util`.
  Layout: `go/cmd/{core-api,telephony-gw,notifier}` + `go/internal/...`. Go 1.22+.
- **Python** (agent-runtime): 3.12+, `uv`, `ruff`, `mypy` strict, `Protocol` para providers,
  pydantic v2 para modelos, pytest. LangGraph para o grafo do agente.
- **TypeScript**: strict; monorepo pnpm; web = React 19 + Vite + TanStack Router/Query + Tailwind;
  mobile = Expo SDK 54 + expo-router + TanStack Query; `packages/shared` com client de API e
  schemas zod compartilhados.
- Eventos/contratos entre Go↔Python: JSON com schemas versionados em `contracts/`.
