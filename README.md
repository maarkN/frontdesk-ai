# FrontDesk AI

AI Voice Receptionist bilíngue (EN/FR-CA) para PMEs de serviço no Canadá — atende a linha do
negócio em modo transbordo, qualifica, agenda e notifica. Derivado das notas de domínio em
`Obsidian/maarkN/Voice AI/` (frente 3, incorporando os padrões da arquitetura de referência).

## Layout

```
specs/            PRD · EPICs · ADRs · CONTEXT.md (digest do domínio)
contracts/        Schemas JSON dos eventos (contrato Go ↔ Python ↔ TS)
go/               Serviços Go (uber-go style): cmd/{telephony-gw, core-api, notifier}
python/           agent-runtime (LangGraph, providers STT/LLM/TTS via Protocol)
apps/web          Dashboard React 19 + Vite + TanStack Router/Query + Tailwind
apps/mobile       App Expo SDK 54 + expo-router
packages/shared   Schemas zod + client de API compartilhado (web/mobile)
CHECKPOINT.md     Estado das fases — leia para retomar trabalho interrompido
```

## Serviços

| Serviço | Stack | Papel |
|---|---|---|
| `telephony-gw` | Go | Telnyx call control + media WS, máquina de estados do turno, barge-in |
| `agent-runtime` | Python | LangGraph, STT/LLM/TTS streaming, guardrails, detecção EN/FR |
| `core-api` | Go | Tenants, DIDs, chamadas, agendamentos, recados, billing (RLS por tenant) |
| `notifier` | Go | SMS dono/cliente, relatório semanal |

Bus: NATS JetStream (`state = fold(events)`). Banco: Postgres + Redis + S3. Observabilidade: OTel.

## Desenvolvimento

- Go: `cd go && go build ./... && go test ./...`
- Python: `cd python/agent-runtime && uv sync && uv run pytest`
- Frontend: `pnpm install && pnpm -r typecheck`
