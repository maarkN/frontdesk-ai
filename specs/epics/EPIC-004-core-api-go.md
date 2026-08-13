# EPIC-004 — core-api (Go)

> Fonte: `specs/CONTEXT.md` · notas `08 - Multi-tenancy`, `16 - Receptionist (Arquitetura e Requisitos)` (RF4).

## Objetivo

API de plataforma em Go: tenants, DIDs, chamadas, agendamentos, recados, onboarding self-service (≤10min), Stripe (trial 14 dias) e isolamento por Postgres RLS. É o backend de RF4 e a fonte de configuração dos demais serviços.

## Escopo

- CRUD e ciclo de vida de tenants; provisionamento/associação de DIDs Telnyx; config por tenant: persona, horário de expediente, modo (transbordo com X segundos / sempre-IA), área de cobertura, celular do dono, palavras-chave de emergência, calendário.
- Postgres com RLS: `set LOCAL app.tenant_id` dentro da transação + `force row level security` em todas as tabelas de tenant; tenant no `context.Context`, nunca como parâmetro de função.
- Projeções de leitura a partir do stream de eventos (`state = fold(events)`): chamadas (metadados, transcrição, link de áudio), agendamentos, recados — consumidor JetStream idempotente (dedup por `eventId`, ordenação por `seq`).
- Onboarding self-service ≤10min: cadastro → "importar do meu site" (LLM extrai FAQ/serviços) → revisão → número → teste de chamada; medido de ponta a ponta.
- Stripe: trial 14 dias, planos Starter/Pro/Growth, excedente por minuto; webhooks idempotentes; corte/downgrade por inadimplência.
- Limites por tenant: semáforo de chamadas simultâneas, rate limit de tokens/segundos de STT.
- Crypto-shredding: KEK por tenant → DEK por chamada → áudio+transcrição em S3; exclusão de dados = destruir DEK.
- AuthN/AuthZ para dashboard web e mobile (sessões/tokens); API consumida por `packages/shared`.
- Residência de dados: tudo em ca-central-1.

## Fora de escopo

- UI de onboarding e dashboard (EPICs 007/008 — aqui ficam os endpoints).
- Envio de SMS/e-mail (EPIC-005).
- Medição/agregação de usage para billing (EPIC-010 produz os agregados; este EPIC os expõe e cobra via Stripe).
- Call control em tempo real (EPIC-002 — consulta a config daqui).

## User stories

**US-4.1 — Como dono de trade, quero sair do cadastro com número ativo e agente configurado em ≤10 minutos.**
- Aceite: fluxo cadastro → import do site → DID → chamada de teste completo em ≤10min medidos; extração de FAQ por LLM revisável antes de ativar; sem passo manual do operador.

**US-4.2 — Como plataforma, quero isolamento forte entre tenants no banco.**
- Aceite: todas as tabelas de tenant com `force row level security`; teste de integração comprova que sem `app.tenant_id` nenhuma linha retorna e que tenant A jamais lê dados de B; nenhuma função de repositório aceita `tenantID` como parâmetro.

**US-4.3 — Como telephony-gw, quero resolver DID → tenant/config em uma chamada de baixa latência.**
- Aceite: endpoint (ou cache Redis alimentado por este serviço) responde p99 < 50ms; DID desconhecido retorna not-found explícito; alterações de config (ex.: toggle de modo) propagam em < 5s.

**US-4.4 — Como dono, quero ver chamadas, agendamentos e recados consistentes com o que aconteceu na linha.**
- Aceite: projeções construídas por fold do stream; reprocessar o stream do zero reproduz o mesmo estado; eventos duplicados/fora de ordem não corrompem projeção (teste com fixtures do EPIC-001).

**US-4.5 — Como negócio, quero cobrar via Stripe com trial de 14 dias.**
- Aceite: assinatura criada no onboarding com trial; webhooks (pagamento, falha, cancelamento) idempotentes; fim de trial sem cartão → agente desativado com aviso; excedente cobrado por minuto usando agregados do EPIC-010.

**US-4.6 — Como titular de dados, quero que a exclusão dos meus dados seja efetiva (Lei 25/PIPEDA).**
- Aceite: exclusão de chamada/tenant destrói a DEK correspondente; áudio/transcrição tornam-se irrecuperáveis (teste demonstra falha de decrypt); trilha de auditoria da exclusão.

## Dependências

- **EPIC-001** (contratos, fixtures de eventos, infra local).
- Consome eventos de **EPIC-002/003** para projeções (pode desenvolver com fixtures).
- **EPIC-010** fornece agregados de usage para a cobrança de excedente.
- **EPICs 005/007/008** dependem dos endpoints deste EPIC.

## Definição de pronto

- RLS forçado e testado em todas as tabelas de tenant; limites por tenant ativos.
- Onboarding ponta a ponta ≤10min demonstrado com tenant real.
- Stripe operante (trial, planos, webhooks idempotentes) em modo test.
- Projeções de chamadas/agendamentos/recados determinísticas sob replay; crypto-shredding demonstrado.
