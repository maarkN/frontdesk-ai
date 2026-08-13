# EPIC-007 — Web dashboard

> Fonte: `specs/CONTEXT.md` (RF4, convenções TS) · notas `16 - Receptionist (Arquitetura e Requisitos)`, `17 - Receptionist (Validação e Roadmap)` (M4).

## Objetivo

Dashboard web em React 19 + Vite + TanStack Router/Query + Tailwind: lista de chamadas com áudio e transcrição, toggle de modo (transbordo/sempre-IA) e fluxo de onboarding self-service. UI mínima e funcional — o valor do MVP está nas chamadas e no toggle, não em BI.

## Escopo

- App em `apps/web`; client de API e schemas zod em `packages/shared` (compartilhados com o mobile).
- Autenticação contra o core-api (login, sessão, recuperação de senha).
- Onboarding self-service (UI do fluxo do EPIC-004): cadastro → "importar do meu site" com revisão da FAQ extraída → escolha de número → configuração básica (horário, celular do dono, área de cobertura) → chamada de teste → cartão/trial Stripe.
- Lista de chamadas: filtros por data/resultado (agendamento, recado, transferência, perdida), player de áudio, transcrição com marcação de idioma e turnos; PII exibida conforme redação (sem acesso ao conteúdo redigido).
- Detalhe de agendamentos e recados estruturados.
- Toggle de modo por tenant: transbordo (com X segundos configurável) ↔ sempre-IA; efeito propagado ao telephony-gw em <5s (contrato do EPIC-004).
- Configurações: persona/saudação, palavras-chave de emergência, notificações, billing (portal Stripe).
- TypeScript strict; TanStack Query para dados de servidor; estados de loading/erro/vazio em todas as telas.

## Fora de escopo

- App mobile (EPIC-008).
- Analytics/BI sofisticado, funis, gráficos de tendência (pós-MVP; relatório semanal é do EPIC-005).
- Edição visual de fluxo/grafo do agente (v2+).
- Painel interno de operação (admin cross-tenant).

## User stories

**US-7.1 — Como dono, quero me cadastrar e ativar o agente sozinho pelo navegador.**
- Aceite: onboarding completo sem intervenção humana em ≤10min; FAQ extraída do site editável antes de ativar; chamada de teste disparada e ouvida na própria UI; trial Stripe iniciado ao final.

**US-7.2 — Como dono, quero ver cada chamada com áudio e transcrição.**
- Aceite: lista paginada com filtros; player reproduz o áudio da chamada (URL assinada, dados em ca-central-1); transcrição por turnos com idioma indicado; chamada nova aparece sem reload manual (polling/invalidations do TanStack Query).

**US-7.3 — Como dono, quero ligar/desligar a IA com um toggle.**
- Aceite: toggle transbordo ↔ sempre-IA com estado persistido no core-api; mudança efetiva em chamadas novas em <5s; feedback otimista com rollback em erro; X segundos do transbordo editável.

**US-7.4 — Como dono, quero ver agendamentos e recados sem abrir cada chamada.**
- Aceite: telas de agendamentos (com link ao evento do Calendar) e recados (quem, o quê, urgência, telefone) com dados idênticos aos das projeções do core-api.

**US-7.5 — Como usuário FR, quero a interface em francês.**
- Aceite: i18n EN/FR-CA em todas as telas do fluxo principal; locale padrão pelo perfil do tenant.

## Dependências

- **EPIC-001** (monorepo pnpm, `packages/shared`).
- **EPIC-004** (todos os endpoints: auth, onboarding, chamadas, toggle, Stripe).
- **EPIC-005** (configuração de notificações exposta na UI).
- **EPIC-008** reutiliza `packages/shared` deste trabalho.

## Definição de pronto

- Onboarding, lista/detalhe de chamadas e toggle funcionando contra o core-api real (não mock) em staging.
- TypeScript strict sem erros; schemas zod de `packages/shared` como única fonte de tipos de API.
- i18n EN/FR-CA no fluxo principal; estados de erro/vazio tratados.
- Testes de fluxo (onboarding e toggle) rodando no CI.
