# FrontDesk AI

AI Voice Receptionist bilíngue (EN/FR-CA) para PMEs de serviço no Canadá — atende a linha do
negócio em modo transbordo, qualifica, agenda e notifica. Derivado das notas de domínio em
`Obsidian/maarkN/Voice AI/` (frente 3, incorporando os padrões da arquitetura de referência).

## Layout

```
specs/            PRD · EPICs · ADRs · CONTEXT.md (digest do domínio)
contracts/        Schemas JSON dos eventos (contrato Go ↔ Python ↔ TS)
go/               Serviços Go (uber-go style): cmd/{telephony-gw, core-api, notifier}
                  + internal/{event, tenantctx, turn, media, degrade, telnyx, store,
                  api, domain, billing, notify, audiobank}
python/           agent-runtime (LangGraph, providers STT/LLM/TTS via Protocol)
apps/             web: React 19 + Vite + TanStack; mobile: Expo SDK 54 + expo-router
packages/         shared: schemas zod (espelham o domínio Go) + client /v1 + i18n EN/FR
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

- Go: `cd go && go build ./... && go vet ./... && go test ./...`
- Python: `cd python/agent-runtime && uv sync && uv run pytest && uv run ruff check . && uv run mypy src`
- Frontend (raiz): `pnpm install && pnpm -r typecheck` (Node >= 20.19; pnpm 11 via
  corepack — em Node 20 o shim do corepack pode falhar, use Node 22/24)

### Web (`apps/web`)

- Dev server: `cd apps/web && pnpm dev` (Vite; aponte `VITE_API_URL` para o core-api `/v1`)
- Build de produção: `pnpm build` (roda `tsc --noEmit` + `vite build`, sai em `apps/web/dist/`)
- Preview do build: `pnpm preview`

### Mobile (`apps/mobile`)

- Dev: `cd apps/mobile && pnpm start` (Expo; `pnpm ios` / `pnpm android` para abrir no simulador)
- Typecheck: `pnpm typecheck` · Saúde do projeto: `npx expo-doctor`
- Monorepo: `metro.config.js` estende os `watchFolders` default com a raiz do workspace
  para hot-reload de `@frontdesk/shared` (consumido por fonte, sem build step)
