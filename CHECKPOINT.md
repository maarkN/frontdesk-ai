# CHECKPOINT — FrontDesk AI (`~/Documents/maarkn-workspace/voice-agents-ca/`)

> Raiz do projeto movida da raiz do workspace para `voice-agents-ca/` a pedido do owner
> (2026-08-13). Git próprio; cada fase concluída gera um commit `checkpoint(fase-N)`.

> **Propósito**: se a sessão for interrompida (limite de uso), abra uma nova sessão do Claude Code
> neste diretório e diga: *"leia CHECKPOINT.md e continue de onde parou"*.
> Cada fase marca `[x]` quando concluída e registra o que foi entregue.

## Estado geral

- Produto: **FrontDesk AI** (AI Voice Receptionist, frente 3 das notas `maarkN/Voice AI/`)
- Digest do domínio: `specs/CONTEXT.md` (ler PRIMEIRO ao retomar)
- Fonte original: `/Users/mac/Documents/Obsidian/maarkN/Voice AI/` (notas 00–17)

## Fases

- [x] **Fase 0 — Fundação** (2026-08-13)
  - Workspace criado, `specs/CONTEXT.md` (digest do domínio) e este checkpoint escritos.
- [x] **Fase 1 — Specs** (2026-08-13)
  - Entregue: `specs/PRD.md` (v1.0) · `specs/epics/EPIC-001..010` · `specs/adr/ADR-001..008` ·
    `specs/README.md` (índice + grafo de dependências). Verificação cruzada ok
    (nomes de serviço, metas G1–G6, pricing, delta mobile no PRD §11 e ADR-007).
- [x] **Fase 2 — Backend Go + Python** (2026-08-13)
  - `contracts/events/v1/` (envelope + catálogo, 18 tipos) · `go/internal/{event,tenantctx}`
    (fold determinístico, PII EN/FR na emissão, usage por fold, bus NATS + memória)
  - `telephony-gw`: máquina do turno (4 estados, 1 goroutine/chamada), endpointing adaptativo
    EN/FR-CA, barge-in 2 níveis, PlayoutTracker, escada de degradação, hedge LLM, circuit
    breaker por provedor, ring buffer STT, banco de áudio, Telnyx atrás de interface
  - `core-api`: domain + store (pgx com RLS `set LOCAL` + in-memory), billing por fold,
    REST chi com tenant só via context; `notifier`: SMS dono/cliente EN/FR, dedup, relatório semanal
  - `python/agent-runtime`: LangGraph (saudação→qualificação→agendamento|recado), Protocols
    STT/LLM/TTS c/ fakes, cascata Haiku/Sonnet, guardrails + anti-injection, bridge WS/NATS
  - Verificação: go build/vet/test verdes (16 pacotes) · pytest 44 ok · ruff/mypy strict ok
- [x] **Fase 3 — Frontend (web + mobile)** (2026-08-13)
  - `packages/shared`: schemas zod espelhando o domínio Go (verificado sem drift), client
    tipado `/v1`, i18n EN/FR · `VERSIONS.md` com versões pinadas via npm (Expo 54.0.36,
    RN 0.81.5, React 19.1.0 mobile / 19.2.8 web, Vite 8, TS 5.9.3, zod 4, Tailwind 4)
  - `apps/web`: login por API key, dashboard, chamadas (player + transcrição + timeline),
    recados, agendamentos, configurações (toggle transbordo), onboarding em passos
  - `apps/mobile`: Expo SDK 54 + expo-router (tabs Home/Calls/Messages/Settings),
    push via expo-notifications, SecureStore, ligar de volta via tel:
  - Nota: `pnpm install` exige Node ≥ 22 (corepack + pnpm 11); usar Node 24 (ver README)
- [x] **Fase 4 — Verificação final** (2026-08-13)
  - go build/vet/test verdes · pytest 44 ok · `pnpm -r typecheck` ok · vite build ok ·
    expo-doctor 18/18 · consistência shared↔Go e specs↔código revisada.

## Como retomar

1. Ler `specs/CONTEXT.md` e este arquivo.
2. Identificar a primeira fase não marcada.
3. Se a fase estava no meio: verificar o log abaixo e o estado dos diretórios (`git status` se
   houver repo; senão `ls` das pastas da fase).
4. Continuar usando workflows (ultracode) com agentes paralelos por módulo.

## Log

- 2026-08-13 06:3x — Fase 0 concluída. Notas 00–17 lidas e digeridas em CONTEXT.md.
  Documentos-fonte externos (`~/Documents/my-workspace/*.md`, `~/Documents/Career/Projects/*.md`)
  NÃO existem no disco — o vault é a única fonte.
- (as fases seguintes anotam aqui o que entregaram)
