# ADR-010 — Observabilidade: slog + OTel → collector → Prometheus/Loki/Tempo → Grafana como código

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** notas 05, 07, 16 do vault `maarkN/Voice AI/`; CONTEXT.md; EPIC-010
- **Depende de:** ADR-002, ADR-003, ADR-006

## Contexto

O produto é uma conversa em tempo real: quando algo degrada, o cliente ouve silêncio antes de
qualquer erro aparecer em log. A nota 07 é explícita — a métrica que importa é
**`silence_gap_ms` (p99)**: "a saúde do produto; sobe antes de qualquer erro". Observabilidade
genérica de microserviço (CPU, 5xx, latência de HTTP) não captura isso; precisamos de métricas
de *produto* como cidadãs de primeira classe, correlacionáveis por chamada através de quatro
serviços em duas linguagens (Go e Python).

Requisitos concretos: rastrear um turno ponta a ponta (Telnyx → telephony-gw → NATS →
agent-runtime → providers → TTS → playout) para decompor o orçamento de latência (G1);
acompanhar a escada de degradação do ADR-006 em produção; logs estruturados com `callId`/
`tenantId` sem PII (ADR-003: PII é redigida na emissão); e uptime ≥99,9% (G5) auditável.
Stack single-vendor de APM (Datadog etc.) conflita com custo (G6) e com residência de dados
em ca-central-1.

## Decisão

**Pipeline padrão OTel com backends open-source, e dashboards/alertas versionados como código.**

- **Logs**: Go usa `log/slog` com handler JSON (Python: `logging` estruturado equivalente).
  Todo log de caminho de chamada carrega `callId`, `tenantId`, `flowId`, `traceId`. Nunca PII
  (a redação do ADR-003 acontece antes de qualquer emissão).
- **Traces e métricas**: OpenTelemetry SDK nos quatro serviços (Go: `go.opentelemetry.io/otel`;
  Python: `opentelemetry-sdk`), exportando OTLP/gRPC para um **otel-collector** central.
  Contexto de trace propagado via NATS headers (W3C `traceparent`) — um turno é um trace.
- **Backends**: collector roteia **métricas → Prometheus**, **logs → Loki**,
  **traces → Tempo**. Correlação log↔trace pelo `traceId` no log.
- **Grafana provisionado como código**: datasources, dashboards e alertas em arquivos
  versionados no repo (provisioning), nunca editados à mão na UI.
- **Métricas de produto de 1ª classe** (nomes canônicos, obrigatórias desde o MVP):
  - `silence_gap_ms` (histograma; olhar o **p99**) — silêncio percebido pelo cliente no turno;
  - `degradation_level` (gauge por chamada / distribuição) — posição na escada do ADR-006;
  - `barge_in_false_positive_rate` — cortes de fala do agente sem fala real do cliente;
  - `recovery_orphan_rate` — canais recuperados no boot sem snapshot (nota 07);
  - `provider_breaker_state` — estado do circuit breaker por provedor (STT/LLM/TTS);
  - `sms_owner_latency_seconds` — chamada encerrada → SMS ao dono (meta G4: <60s).
- **Alerta-ouro**: `degradation_level > degNormal` por **>60s** em **>5% das chamadas ativas**
  → página o owner. É o alerta de "o produto está ruim agora", acima de qualquer alerta de
  infraestrutura.

## Consequências

**Positivas**
- Um turno lento vira um trace decomponível (VAD/STT/LLM/TTS/playout) — o orçamento G1 deixa de
  ser estimativa e vira medição contínua.
- Métricas de produto detectam degradação antes do cliente reclamar; o alerta-ouro dispara sobre
  experiência real, não sobre sintoma de infra.
- Stack open-source auto-hospedada em ca-central-1: sem conflito de residência de dados, custo
  previsível (G6).
- Dashboards como código: revisáveis em PR, reproduzíveis em qualquer ambiente, sem drift de UI.
- Billing por fold (ADR-003) e observabilidade compartilham a mesma disciplina de eventos —
  `traceId` no envelope liga os dois mundos.

**Negativas / riscos assumidos**
- Operar Prometheus/Loki/Tempo/Grafana é trabalho nosso (retenção, storage, upgrades). Aceito:
  é operação padronizada e barata comparada a APM SaaS; começa com single-node.
- Cardinalidade: métricas nunca levam `callId` como label (só `tenantId` com parcimônia);
  detalhe por chamada vive em traces/logs. Disciplina obrigatória em code review.
- Amostragem de traces terá de ser calibrada com volume (tail-based sampling no collector fica
  como evolução; MVP começa com 100%).

## Alternativas consideradas

1. **APM SaaS (Datadog, New Relic, Honeycomb).** Rejeitada: custo por host/ingest ataca G6,
   e ingestão fora de ca-central-1 cria risco de residência para metadados de chamada.
2. **Só logs estruturados, sem traces.** Rejeitada: decompor o orçamento de turno através de 4
   serviços exige correlação causal; grep de logs não reconstrói a linha do tempo do turno.
3. **Métricas direto no Prometheus (client libs), sem OTel.** Rejeitada: perderia o pipeline
   único (traces+métricas+propagação) e amarraria a instrumentação ao backend; OTel deixa o
   backend trocável no collector.
4. **ELK para logs em vez de Loki.** Rejeitada: custo operacional e de storage maior; Loki
   indexa só labels e integra nativamente com Grafana/Tempo (correlação log↔trace).
