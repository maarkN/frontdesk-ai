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
- [ ] **Fase 2 — Backend Go + Python**
  - `go/` (cmd/{core-api,telephony-gw,notifier}, internal/*, uber-go style, `go build ./...` verde)
  - `python/agent-runtime/` (LangGraph, providers via Protocol, testes)
  - `contracts/` (schemas de eventos JSON)
- [ ] **Fase 3 — Frontend (web + mobile)**
  - `apps/web` (React 19 + Vite + TanStack Router/Query + Tailwind, TS strict)
  - `apps/mobile` (Expo SDK 54 + expo-router + TanStack Query)
  - `packages/shared` (client de API + schemas zod)
- [ ] **Fase 4 — Verificação e revisão**
  - Builds/testes de todos os módulos; revisão de consistência specs ↔ código; README final.

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
