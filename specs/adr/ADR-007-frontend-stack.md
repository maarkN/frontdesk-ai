# ADR-007 — Stack de frontend: monorepo pnpm, React 19 + Vite (web), Expo SDK 54 (mobile)

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** CONTEXT.md (§ Convenções de código, § decisão de escopo), nota `03 - Arquitetura Geral e Latência` (estrutura de referência com Next.js), nota `16 - Receptionist (Arquitetura e Requisitos)` (dashboard)
- **Depende de:** ADR-001, ADR-002

## Contexto

O frontend do FrontDesk AI é o dashboard do dono da PME: lista de chamadas com áudio e
transcrição, toggle transbordo/sempre-IA, onboarding self-service, billing (Stripe). Toda a API
é servida pelo **core-api em Go** — o frontend não tem backend próprio nem lógica server-side.

A nota 03 (arquitetura de referência da plataforma de voz) cita **Next.js** para o app web.
Este ADR registra por que **não** seguimos essa referência aqui, e registra também um **desvio
de escopo**: o PRD original excluía app mobile do MVP; em **2026-08-13 o owner decidiu incluir
mobile** (React Native/Expo) junto do web. Essa decisão está refletida no CONTEXT.md e é tratada
aqui como requisito, não como proposta.

## Decisão

### Monorepo pnpm

Workspace pnpm com `apps/web`, `apps/mobile` e `packages/shared`. pnpm por hoisting estrito
(evita dependência fantasma), workspaces nativos e velocidade de instalação em CI.

### Web: React 19 + Vite + TanStack Router + TanStack Query + Tailwind

**Justificativa do desvio vs Next.js (nota 03):**

- O produto web é um **dashboard SPA autenticado**. Não há páginas públicas relevantes, logo
  **não há requisito de SEO** — o principal argumento para SSR desaparece.
- **A API já existe e é em Go** (core-api). Route handlers, server actions e o modelo
  server-first do Next.js duplicariam uma camada de backend que não nos pertence; viraríamos
  proxy de nós mesmos.
- Sem SSR/RSC, o Next.js vira um empacotador caro: mais superfície de framework (convenções de
  app router, limites server/client) sem usar o que o justifica. Vite dá dev server e build mais
  simples e rápido para uma SPA pura.
- A nota 03 descreve outro sistema (plataforma de referência com backend TS/NestJS, onde Next.js
  convive com API própria em TS). O contexto do Receptionist é diferente: backend Go, frontend
  puramente cliente.

Escolhas dentro da SPA:

- **TanStack Router** — roteamento type-safe (params e search params tipados), file-based
  opcional; combina com o padrão de dados abaixo.
- **TanStack Query** — todo estado de servidor (chamadas, transcrições, config de tenant) via
  query/mutation com cache, invalidation e polling (lista de chamadas quase-tempo-real).
  Estado global de cliente fica mínimo (tema, sessão).
- **Tailwind** — utilitários, design system leve compartilhável em tokens com o mobile.

### Mobile: Expo SDK 54 + expo-router + TanStack Query

- **Expo (managed)** — sem necessidade de módulo nativo custom no MVP (áudio de playback,
  push notification e deep link são cobertos pelo SDK); EAS para build/submit sem manter
  toolchain nativa.
- **expo-router** — roteamento file-based consistente com o mental model do web.
- **TanStack Query** — mesma camada de dados do web, consumindo o mesmo client de
  `packages/shared`.
- Caso de uso mobile: o dono recebe SMS/push de chamada nova e abre o app para ouvir o áudio,
  ler a transcrição e alternar o modo transbordo/sempre-IA de qualquer lugar.

### `packages/shared`: zod + client de API

- **Schemas zod** espelhando os contratos JSON versionados de `contracts/` (fonte de verdade
  Go↔TS). Validação em runtime na borda do client: resposta que não bate com o schema falha
  cedo e com mensagem útil.
- **Client de API tipado** (fetch + zod parse) usado por web e mobile — um único lugar para
  auth header, base URL, erros e retry de rede.
- Tokens de design e helpers de formatação (datas, duração de chamada, telefone) compartilhados.

### TypeScript strict

`strict: true` em todos os pacotes, sem `any` implícito, `noUncheckedIndexedAccess` ligado.
Tipos de API nunca escritos à mão fora de `packages/shared`.

## Registro de desvio de escopo

| Item | PRD original | Decisão vigente |
|---|---|---|
| App mobile no MVP | **Excluído** | **Incluído** — decisão do owner em 2026-08-13 |

Consequência assumida: o escopo do MVP cresce; o mitigador é o compartilhamento agressivo
(`packages/shared`, mesma camada de dados TanStack Query, mesmas telas-chave) — o mobile é uma
segunda casca sobre a mesma API, não um segundo produto.

## Consequências

- (+) Uma única fonte de tipos e validação (zod em `packages/shared`) para web e mobile;
  drift de contrato com o core-api é pego na borda, em runtime e no CI.
- (+) SPA + Vite: build simples, deploy como estático atrás de CDN, zero servidor Node para
  operar (a operação já tem Go + Python para cuidar).
- (+) Mesma stack de dados (TanStack Query) nos dois apps reduz o custo do mobile adicionado.
- (−) Desvio da referência (nota 03): se um dia surgir necessidade de páginas públicas com SEO
  (site de marketing, páginas de agendamento públicas), isso será um app separado — não migração
  do dashboard.
- (−) Mobile no MVP adiciona superfície de release (lojas, EAS, review da Apple) e QA em dois
  alvos.
- (−) SPA autenticada exige atenção a auth no cliente (tokens de curta duração, refresh via
  core-api) — sem cookies httpOnly gerenciados por um BFF.

## Alternativas consideradas

- **Next.js (como na nota 03)** — rejeitado para este produto: sem SEO, sem backend TS; SSR/RSC
  não pagam sua complexidade num dashboard autenticado sobre API Go. A referência da nota 03
  pertence a outro sistema, com backend TypeScript próprio.
- **Remix / React Router v7 em modo framework** — rejeitado pelo mesmo motivo do Next.js:
  orientado a server, que não temos em TS.
- **React Native CLI (bare) para o mobile** — rejeitado: sem requisito nativo custom no MVP,
  Expo managed remove toolchain e acelera release.
- **Uma só codebase universal (Expo web)** — rejeitado: o dashboard web quer densidade de
  informação e layout desktop (tabelas de chamadas, player + transcrição lado a lado) que
  saem melhores em React DOM + Tailwind do que em primitives RN traduzidas.
- **Sem mobile no MVP (manter PRD original)** — rejeitado por decisão do owner em 2026-08-13;
  registrada acima como desvio consciente.
