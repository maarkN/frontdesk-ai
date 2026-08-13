# specs/ — FrontDesk AI (AI Voice Receptionist)

Especificações de engenharia do FrontDesk AI: receptionist de voz por IA para PMEs de serviço
no Canadá, bilíngue EN/FR-CA. Fonte original do domínio: vault Obsidian
`maarkN/Voice AI/` (notas 00–17) — em conflito, as notas vencem.

Atualizado em 2026-08-13.

## Documentos base

| Documento | Conteúdo |
|---|---|
| [CONTEXT.md](CONTEXT.md) | Digest canônico do domínio para agentes de implementação: arquitetura, decisões-chave, conceitos transversais, RF1–RF4, metas G1–G6, pricing, convenções de código |
| [PRD.md](PRD.md) | PRD v1.0: visão, tese, personas, requisitos funcionais e não-funcionais, metas, pricing, fases (0/1/2+), riscos e desvios do PRD original (mobile no MVP) |

## EPICs

| EPIC | Escopo | Depende de |
|---|---|---|
| [EPIC-001](epics/EPIC-001-fundacao-e-contratos.md) | Fundação e contratos: monorepo, `contracts/` (schemas de eventos Go↔Python↔TS), CI, infra local | — (raiz) |
| [EPIC-002](epics/EPIC-002-telephony-gw-go.md) | `telephony-gw` (Go): Telnyx, máquina de estados do turno, barge-in, endpointing, PlayoutTracker | 001, 003, 004 |
| [EPIC-003](epics/EPIC-003-agent-runtime-python.md) | `agent-runtime` (Python): grafo LangGraph, providers via `Protocol`, cascata Haiku/Sonnet, guardrails, EN/FR | 001, 002, 004 |
| [EPIC-004](epics/EPIC-004-core-api-go.md) | `core-api` (Go): tenants, DIDs, RLS, projeções por fold, onboarding ≤10min, Stripe, crypto-shredding | 001, 010 |
| [EPIC-005](epics/EPIC-005-notifier-go.md) | `notifier` (Go): SMS dono <60s, SMS cliente, relatório semanal | 001, 002/003, 004 |
| [EPIC-006](epics/EPIC-006-resiliencia-e-degradacao.md) | Resiliência: escada de degradação, ring buffer STT, hedge de LLM, banco de áudio, circuit breaker | 001, 002, 003 |
| [EPIC-007](epics/EPIC-007-web-dashboard.md) | Web dashboard: React 19 + Vite, onboarding, lista de chamadas, toggle de modo | 001, 004, 005 |
| [EPIC-008](epics/EPIC-008-mobile-app.md) | Mobile app: Expo SDK 54, push pós-chamada, recados, toggle | 001, 004, 005, 007 |
| [EPIC-009](epics/EPIC-009-evals-e-qualidade.md) | Evals: três camadas, callers adversariais, gate de publish (`piiLeaks > 0` bloqueia) | 001, 002, 003, 006 |
| [EPIC-010](epics/EPIC-010-observabilidade-e-billing.md) | Observabilidade e billing: OTel por chamada, métricas de saúde, usage por fold | 001, 002/003/005/006 |

## ADRs

| ADR | Decisão | Depende de |
|---|---|---|
| [ADR-001](adr/ADR-001-frente-receptionist.md) | Implementar a frente Receptionist como produto, incorporando os padrões da arquitetura de referência | — |
| [ADR-002](adr/ADR-002-go-python-split.md) | Go no caminho quente (telephony-gw, core-api, notifier); Python no agente (agent-runtime); TS só no frontend | ADR-001 |
| [ADR-003](adr/ADR-003-eventos-fonte-da-verdade.md) | Eventos como fonte da verdade (`state = fold(events)`), NATS JetStream, PII redigida na emissão | ADR-001, ADR-002 |
| [ADR-004](adr/ADR-004-telnyx-sem-asterisk.md) | Telnyx como carrier e camada de mídia no MVP; Asterisk próprio só na v2 (alavanca de margem) | ADR-001 |
| [ADR-005](adr/ADR-005-multi-tenancy-rls.md) | Multi-tenancy: resolução por DID, tenant no contexto (lint), Postgres RLS, crypto-shredding | ADR-001, ADR-003 |
| [ADR-006](adr/ADR-006-degradacao-nunca-cair.md) | Degradação controlada: escada `degNormal→degVoicemail`, hedge (não retry), breaker por provedor, failover para o celular do dono | ADR-001, ADR-004 |
| [ADR-007](adr/ADR-007-frontend-stack.md) | Frontend: monorepo pnpm, React 19 + Vite (web), Expo SDK 54 (mobile); registra o desvio "mobile no MVP" | ADR-001, ADR-002 |
| [ADR-008](adr/ADR-008-uber-go-style.md) | Adoção integral do uber-go/guide para os serviços Go, com enforcement via golangci-lint | ADR-002 |

## Convenções

- Serviços: `telephony-gw`, `agent-runtime`, `core-api`, `notifier` (nomes canônicos; ver CONTEXT.md).
- Metas de referência: turno p50 < 1,2s / p95 < 2,5s (G1) · contenção ≥70% (G2) · idioma ≥95% (G3) ·
  SMS < 60s (G4) · uptime ≥99,9% + failover (G5) · custo/min < 30% do preço/min (G6).
- Delta de escopo vs PRD original: **app mobile incluído no MVP** — registrado no PRD (§11) e no ADR-007.
