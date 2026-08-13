# EPIC-001 — Fundação e contratos

> Fonte: `specs/CONTEXT.md` · notas `05 - Eventos e Estado de Runtime`, `02 - Escolha de Linguagem`, `16 - Receptionist (Arquitetura e Requisitos)`.

## Objetivo

Estabelecer o monorepo, o diretório `contracts/` com os schemas versionados de eventos (fonte da verdade Go↔Python↔TS) e o pipeline de CI. Tudo que os demais EPICs consomem — envelope de evento, catálogo de tipos, codegen/validação — nasce aqui.

## Escopo

- Layout do monorepo: `go/cmd/{core-api,telephony-gw,notifier}` + `go/internal/...`, `python/agent-runtime/`, `apps/{web,mobile}`, `packages/shared`, `contracts/`.
- `contracts/`: JSON Schemas versionados do envelope e do catálogo de eventos (call.started/answered/ended, consent.captured, language.detected/switched, node.entered/exited, stt.final, agent.turn.completed, tool.invoked/completed, var.assigned, dtmf.received, transfer.initiated/answered, recording.started, error.raised).
- Envelope obrigatório: `eventId` (ULID), `callId`, `seq` monotônico por chamada, `ts`, `tenantId`, `flowId`+`flowVersion`, `type`, `nodeId`, `payload`, `redactions[]`.
- Geradores/validadores por linguagem: structs Go, modelos pydantic v2, schemas zod em `packages/shared` — todos derivados ou validados contra `contracts/` no CI.
- Regra de compatibilidade de schema (mudança breaking exige bump de versão do contrato) verificada no CI.
- CI: lint+build+test para Go (uber-go/guide via golangci-lint), Python (`uv`, `ruff`, `mypy` strict, pytest), TS (strict, pnpm); validação de contratos como job obrigatório.
- Infra local de desenvolvimento: docker-compose com NATS JetStream, Postgres, Redis, MinIO (S3).
- Convenção de dedup/ordenação documentada nos contratos: consumidor deduplica por `eventId` (`nats.MsgId`) e ordena por `seq` — NATS é at-least-once.

## Fora de escopo

- Qualquer lógica de negócio dos serviços (EPICs 002–005).
- Redação de PII na emissão (detectores vivem no emissor; EPIC-002/003) — o contrato apenas define o campo `redactions`.
- Deploy em AWS ca-central-1 (entra com o serviço que primeiro for a produção).

## User stories

**US-1.1 — Como dev de qualquer serviço, quero um comando único que rode lint+test do monorepo inteiro.**
- Aceite: `make check` (ou equivalente) roda Go, Python e TS localmente e é o mesmo comando do CI; falha em qualquer linguagem falha o todo.

**US-1.2 — Como dev do telephony-gw ou do agent-runtime, quero tipos gerados/validados a partir de `contracts/` para nunca serializar evento fora do contrato.**
- Aceite: publicar um evento com campo faltante do envelope falha em teste; Go, Python e TS compartilham o mesmo schema por versão; teste de round-trip Go→JSON→Python passa no CI para todos os tipos do catálogo.

**US-1.3 — Como revisor, quero que mudança breaking em contrato seja bloqueada sem bump de versão.**
- Aceite: job de CI compara schemas contra a versão publicada; remoção/renomeação de campo sem nova versão falha o PR.

**US-1.4 — Como dev, quero subir o ambiente local (NATS, Postgres, Redis, S3) com um comando.**
- Aceite: `docker compose up` deixa os quatro serviços saudáveis; streams JetStream criados por script idempotente.

**US-1.5 — Como consumidor de eventos, quero garantia de dedup/ordenação especificada no contrato.**
- Aceite: contrato documenta `eventId` como chave de dedup e `seq` como ordem por `callId`; teste de referência demonstra fold determinístico com eventos duplicados e fora de ordem.

## Dependências

- Nenhuma. É a raiz: EPICs 002–010 dependem deste.

## Definição de pronto

- Monorepo com CI verde nas três linguagens.
- `contracts/` v1 cobrindo envelope + catálogo completo do CONTEXT.md, com validação cross-language no CI.
- Ambiente local docker-compose funcional e documentado no README.
- Regra de versionamento de contratos escrita e aplicada por job de CI.
