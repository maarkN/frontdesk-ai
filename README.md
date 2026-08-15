# FrontDesk AI

AI Voice Receptionist bilíngue (EN/FR-CA) para PMEs de serviço no Canadá — atende a linha do
negócio em modo transbordo, qualifica, agenda e notifica. Derivado das notas de domínio em
`Obsidian/maarkN/Voice AI/` (frente 3, incorporando os padrões da arquitetura de referência).

## Layout

```
specs/            PRD · EPICs · ADRs · CONTEXT.md (digest do domínio)
contracts/        Schemas JSON dos eventos (contrato Go ↔ Python ↔ TS)
go/               Serviços Go (uber-go style): cmd/{telephony-gw, core-api, notifier}
                  + internal/{event, tenantctx, turn, media, degrade, telnyx,
                  asterisk, store, api, domain, billing, notify, audiobank, obs}
python/           agent-runtime (LangGraph, providers STT/LLM/TTS via Protocol)
apps/             web: React 19 + Vite + TanStack; mobile: Expo SDK 54 + expo-router
packages/         shared: schemas zod (espelham o domínio Go) + client /v1 + i18n EN/FR
asterisk/         Backend de mídia próprio (ADR-009): Dockerfile + configs ARI/PJSIP
infra/            docker/ (Dockerfiles) · k8s/ (kustomize base+overlays) ·
                  observability/ (otel-collector, prometheus, loki, tempo, grafana)
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

## Infra

### Subir a stack local (docker compose)

```sh
make up                                # infra + observabilidade + serviços (backend Telnyx)
make up PROFILES="--profile asterisk"  # inclui o Asterisk (backend de mídia próprio, ADR-009)
```

- Web: `http://localhost:3000` · core-api: `:8080` · telephony-gw: `:8081` (host)
- **Grafana**: `http://localhost:3001` (porta 3000 do container; o host `:3000` é o web).
  Login default `admin`/`admin` — dashboards FrontDesk (voice health, calls funnel,
  business) e datasources Prometheus/Loki/Tempo já provisionados.
- Backend de mídia do `telephony-gw` via env `MEDIA_BACKEND=telnyx|asterisk` (default
  `telnyx`). Com `asterisk`: `ASTERISK_ARI_URL` (obrigatória), `ASTERISK_APP`
  (default `frontdesk-v1`), `ARI_USERNAME`/`ARI_PASSWORD` (mesmos nomes do entrypoint
  do container Asterisk), `ASTERISK_TRANSFER_CONTEXT` (default `transfer-owner`),
  `ASTERISK_EXTERNAL_MEDIA_ADDR` (socket UDP do gateway visto do Asterisk) e
  `MEDIA_LISTEN_ADDR` (default `:4000`). O compose já passa tudo coerente.

### Aplicar no Kubernetes (kustomize)

```sh
kubectl apply -k infra/k8s/overlays/dev    # ou prod
# dashboards do Grafana vêm de fora da raiz do kustomization:
kustomize build --load-restrictor LoadRestrictionsNone infra/k8s/overlays/dev | kubectl apply -f -
```

Preencher `infra/k8s/base/secrets.yaml` antes (ver `SECRETS.md`); `MEDIA_BACKEND` e as
envs do Asterisk vivem em `base/configmap.yaml` (senha ARI em `frontdesk-secrets`).
O Deployment do Asterisk fica em `replicas: 0` (alavanca v2; RTP em k8s exige
hostNetwork/nó dedicado — ver comentários em `base/asterisk/asterisk.yaml`).

> **Nota de validação**: docker/kubectl/kustomize não existem na máquina onde esta fase
> foi verificada — a validação de compose/k8s/observability foi **estática** (YAML/JSON
> parseados, paths de volumes/build contexts/configMapGenerator conferidos, portas e
> nomes de env cruzados com o código). O primeiro `docker compose up` / `kubectl apply`
> real pode revelar ajustes finos de runtime.

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
