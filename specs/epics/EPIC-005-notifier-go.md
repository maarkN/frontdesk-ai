# EPIC-005 — notifier (Go)

> Fonte: `specs/CONTEXT.md` (RF3, G4) · notas `16 - Receptionist (Arquitetura e Requisitos)`, `17 - Receptionist (Validação e Roadmap)` (M3, relatório como arma anti-churn).

## Objetivo

Serviço de notificações em Go: SMS ao dono em <60s após a chamada (quem, o quê, urgência, telefone, transcrição), SMS de confirmação ao cliente e relatório semanal por e-mail. É onde o valor do produto fica visível (G4).

## Escopo

- Consumidor JetStream dos eventos de fim de chamada (`call.ended` + projeção do resultado: agendamento/recado/transferência), idempotente (`nats.MsgId = eventId`, `on conflict do nothing` por chave natural).
- SMS ao dono <60s: resumo estruturado — quem ligou, o que quer, urgência, telefone de retorno, link para transcrição/áudio; template por locale do tenant.
- SMS ao cliente: confirmação de agendamento (data/hora, negócio) ou confirmação de recado; no idioma da chamada.
- Relatório semanal por e-mail: "a IA atendeu N ligações, agendou X jobs, registrou Y recados, valor estimado CAD Z"; agregado por fold sobre o stream/projeções.
- Envio de SMS via Telnyx (mesmo carrier); e-mail via provedor transacional; retries idempotentes com DLQ — sem duplicar mensagem ao destinatário.
- Respeito às redações de PII: conteúdo de SMS/e-mail montado a partir de eventos já redigidos; nada de PII proibida em template.
- Métrica `sms_owner_latency_seconds` (fim da chamada → entrega aceita pelo carrier) exportada para EPIC-010.

## Fora de escopo

- SMS bidirecional e retorno automático de missed calls (v1.x).
- Push notification do mobile (EPIC-008).
- Geração de transcrição/áudio (EPIC-002/004 — aqui só o link).
- UI de configuração de notificações (EPIC-007).

## User stories

**US-5.1 — Como dono, quero um SMS em menos de 60 segundos após cada chamada atendida pela IA.**
- Aceite: p95 de `sms_owner_latency_seconds` < 60s em teste de carga local; SMS contém quem/o quê/urgência/telefone/link; evento duplicado não gera segundo SMS.

**US-5.2 — Como cliente que agendou, quero receber a confirmação por SMS no meu idioma.**
- Aceite: agendamento gera SMS com data/hora e nome do negócio no locale da chamada (`language.detected/switched` decide); recado gera confirmação de que a mensagem foi registrada; opt-out básico respeitado (STOP/ARRET).

**US-5.3 — Como dono, quero um relatório semanal que mostre o que a IA fez por mim.**
- Aceite: e-mail semanal com chamadas atendidas, jobs agendados, recados e valor estimado; números batem com as projeções do core-api para o mesmo período; envio por tenant no fuso do tenant; tenant sem atividade recebe variante "sem chamadas".

**US-5.4 — Como operador, quero que falha de carrier não perca nem duplique notificações.**
- Aceite: retry idempotente com DLQ; mensagem que esgota tentativas fica visível para reprocesso; teste de caos (carrier 500 intermitente) termina sem duplicata e sem perda.

## Dependências

- **EPIC-001** (contratos, JetStream).
- **EPIC-002/003** (eventos de chamada e resultado — desenvolvível com fixtures).
- **EPIC-004** (dados de tenant: telefone do dono, locale, links de transcrição; projeções para o relatório).
- **EPIC-010** coleta as métricas de latência de SMS.

## Definição de pronto

- SMS dono e cliente funcionando ponta a ponta em chamada real, dentro do SLA de 60s.
- Relatório semanal gerado e enviado para tenant piloto com números conferidos.
- Consumo idempotente comprovado por teste (duplicatas e replay não geram reenvio).
- Templates EN e FR-CA revisados; nenhuma PII redigida vazando em mensagem.
