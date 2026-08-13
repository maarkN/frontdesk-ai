# EPIC-008 — Mobile app

> Fonte: `specs/CONTEXT.md` (delta de escopo: app mobile é pedido explícito do owner; convenções TS) · nota `16 - Receptionist (Arquitetura e Requisitos)` (RF4).

## Objetivo

App mobile (Expo SDK 54 + expo-router + TanStack Query) para o dono acompanhar o negócio do bolso: chamadas com áudio/transcrição, recados, push com resumo pós-chamada e toggle de modo. O dono de trade vive em campo — o mobile é o ponto de contato diário; configuração pesada fica no web.

## Escopo

- App em `apps/mobile`, TypeScript strict, reutilizando client de API e schemas zod de `packages/shared`.
- Autenticação (mesmo backend do web; armazenamento seguro de token via SecureStore).
- Lista de chamadas com player de áudio e transcrição por turnos; pull-to-refresh e paginação.
- Recados estruturados (quem, o quê, urgência, telefone) com ação de retorno em um toque (deep link para o discador).
- Push notification com resumo pós-chamada (Expo Push): quem ligou, o quê, urgência; tap abre o detalhe da chamada; registro/rotação de push token no core-api.
- Toggle de modo transbordo ↔ sempre-IA, mesmo contrato do web (<5s de propagação).
- Agendamentos do dia/semana em lista simples.
- i18n EN/FR-CA; iOS e Android via EAS Build.

## Fora de escopo

- Onboarding completo e billing (feitos no web; o app aponta para lá).
- Edição de persona/configurações avançadas.
- Softphone/atender chamadas no app (transferência a quente toca no celular por PSTN — EPIC-002).
- Modo offline além de cache básico do TanStack Query.

## User stories

**US-8.1 — Como dono em campo, quero um push logo após cada chamada com o resumo.**
- Aceite: push entregue após `call.ended` com quem/o quê/urgência (conteúdo respeita redações de PII); tap abre o detalhe da chamada; token registrado no login e invalidado no logout; sem push duplicado para o mesmo evento.

**US-8.2 — Como dono, quero ouvir a chamada e ler a transcrição no celular.**
- Aceite: player funciona em iOS e Android (incluindo tela bloqueada/áudio em background básico); transcrição idêntica à do web; URLs de áudio assinadas e expiráveis.

**US-8.3 — Como dono, quero retornar um recado com um toque.**
- Aceite: lista de recados ordenada por urgência/recência; botão de retorno abre o discador com o número; recado marcável como resolvido, sincronizado com o web.

**US-8.4 — Como dono, quero ligar/desligar a IA do celular.**
- Aceite: mesmo comportamento e contrato do toggle web (US-7.3); estado consistente entre web e mobile após mudança em qualquer um.

**US-8.5 — Como dono francófono, quero o app em FR-CA.**
- Aceite: i18n EN/FR-CA em todas as telas; locale segue o perfil do usuário.

## Dependências

- **EPIC-001** (monorepo, `packages/shared`).
- **EPIC-004** (endpoints de auth, chamadas, recados, toggle, registro de push token).
- **EPIC-005/002-003** (eventos que disparam o push de resumo — o envio do push em si é servido pelo backend: gatilho no notifier ou core-api, a decidir na implementação).
- **EPIC-007** (convenções de API client e zod estabelecidas primeiro no web).

## Definição de pronto

- Builds EAS iOS e Android instaláveis por testadores (TestFlight/internal track).
- Push de resumo funcionando ponta a ponta em chamada real nos dois SOs.
- Chamadas, recados e toggle operando contra o core-api de staging.
- TypeScript strict; i18n EN/FR-CA; paridade de dados com o web verificada.
