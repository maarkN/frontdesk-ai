# EPIC-010 — Observabilidade e billing

> Fonte: `specs/CONTEXT.md` · notas `05 - Eventos e Estado de Runtime` (billing por fold), `07 - Resiliência e Degradação` (métricas), `16/17 - Receptionist` (M5, G6).

## Objetivo

Observabilidade por chamada com OTel (trace ponta a ponta Go↔Python), métricas de saúde do produto (`silence_gap_ms`, `degradation_level` e correlatas) e medição de usage por fold sobre o stream de eventos (CallMinutes, STTSeconds, LLMTokensIn/Out, TTSChars) — insumo do billing de excedente (EPIC-004) e da meta G6 (custo variável/min < 30% do preço/min).

## Escopo

- Tracing OTel por chamada: trace único do webhook Telnyx ao último evento, com spans por turno (VAD→STT final→LLM 1º token→TTS 1º chunk→playout) atravessando telephony-gw (Go) e agent-runtime (Python) via propagação de contexto; `callId`/`tenantId` como atributos.
- Métricas de produto: `silence_gap_ms` (p99 — sobe antes de qualquer erro), `degradation_level` (distribuição; `degScripted` >1% é sinal ruim), `barge_in_false_positive_rate`, `recovery_orphan_rate`, `provider_breaker_state`, `sms_owner_latency_seconds`, latência de turno p50/p95 (G1), taxa de contenção (G2), acurácia de idioma (G3).
- Alertas mínimos: `degradation_level > degNormal` por >60s em >5% das chamadas ativas; `silence_gap_ms` p99 acima do orçamento; SMS SLA; breaker aberto.
- Usage por fold: consumidor JetStream idempotente que agrega por chamada/tenant/período — CallMinutes, STTSeconds, LLMTokensIn/Out (com atribuição Haiku vs Sonnet e custo de hedge), TTSChars; reprocessável do zero com resultado idêntico.
- Custo por chamada e por minuto calculado a partir do usage × tabela de preços de provedores; painel/consulta de margem por tenant e por plano (G6).
- API/tabelas de usage consumidas pelo core-api para cobrança de excedente Stripe (~CAD 0,35/min) e pelo dashboard.
- Análise pós-chamada (disposition, failures, sentimento) exportada como eventos/atributos — insumo do ciclo de fixtures do EPIC-009.
- Logs estruturados com `callId`/`tenantId`; retenção e residência em ca-central-1; PII nunca em métrica, log ou atributo de span (mesmas regras de redação dos eventos).

## Fora de escopo

- Emissão dos eventos e das medições brutas (responsabilidade de cada serviço; este EPIC define convenções e agrega).
- Cobrança em si e webhooks Stripe (EPIC-004 — daqui saem os agregados).
- Dashboards de negócio para o dono (EPIC-005 relatório semanal; EPIC-007 UI).
- Uptime/monitoração de infraestrutura genérica (operação; aqui só a instrumentação de aplicação).

## User stories

**US-10.1 — Como operador, quero abrir o trace de uma chamada e ver onde o tempo foi gasto em cada turno.**
- Aceite: dado um `callId`, o trace mostra spans de VAD/STT/LLM/TTS/playout por turno com durações; propagação de contexto Go→NATS→Python comprovada; overhead de instrumentação sem violar o orçamento de latência.

**US-10.2 — Como operador, quero detectar degradação antes de o cliente reclamar.**
- Aceite: `silence_gap_ms` p99 e `degradation_level` emitidos em tempo real por chamada; alerta da nota 07 (>60s em >5% das chamadas ativas) dispara em teste de caos do EPIC-006; falso alarme calibrado em staging.

**US-10.3 — Como negócio, quero saber o custo variável de cada chamada e a margem por tenant.**
- Aceite: fold produz CallMinutes/STTSeconds/LLMTokens/TTSChars por chamada; custo calculado bate com faturas de provedores em amostra de reconciliação (tolerância definida); consulta de custo/min vs preço/min por tenant responde à meta G6.

**US-10.4 — Como core-api, quero agregados de usage confiáveis para cobrar excedente.**
- Aceite: agregado mensal por tenant idempotente sob replay (eventos duplicados/fora de ordem não alteram o total); fechamento de período versionado; divergência entre reprocesso e valor publicado gera alerta, não sobrescrita silenciosa.

**US-10.5 — Como responsável por privacidade, quero garantia de que telemetria não vaza PII.**
- Aceite: teste automatizado varre spans, logs e métricas gerados por chamadas sintéticas com PII (cartão, SIN, código postal, DOB nos dois idiomas) e não encontra nenhum valor bruto; convenção de atributos documentada e lintada.

## Dependências

- **EPIC-001** (envelope/eventos — o fold e as convenções derivam dos contratos).
- **EPICs 002/003/005/006** (emissores de spans, métricas e eventos de medição).
- **EPIC-004** consome os agregados para Stripe; **EPIC-009** consome a análise pós-chamada.

## Definição de pronto

- Trace ponta a ponta por chamada funcionando em staging com chamada real.
- Métricas e alertas da nota 07 ativos; metas G1–G3 mensuráveis a partir da telemetria.
- Pipeline de usage por fold reprocessável, reconciliado contra fatura de provedor em amostra, e servindo o excedente do EPIC-004.
- Verificação automatizada de ausência de PII em telemetria no CI.
