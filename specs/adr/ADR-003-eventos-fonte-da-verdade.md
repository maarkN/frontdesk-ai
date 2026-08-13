# ADR-003 — Eventos como fonte da verdade (`state = fold(events)`)

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** notas 05, 08 do vault `maarkN/Voice AI/`; CONTEXT.md
- **Depende de:** ADR-001, ADR-002

## Contexto

Uma chamada de voz gera dados para consumidores muito diferentes: o runtime precisa do estado
corrente (nó atual, variáveis, transcrição parcial); a depuração precisa de replay; o dashboard
precisa de timeline; o billing precisa de usage por tenant; a análise pós-chamada precisa da
conversa inteira. Modelar estado e eventos como duas coisas separadas produz divergência entre
elas e N pipelines de dados.

Restrições concretas do domínio:

- O bus escolhido (NATS JetStream) entrega **at-least-once e sem garantia de ordem** — o
  consumidor precisa deduplicar e ordenar sozinho.
- STT streaming emite parciais dezenas de vezes por segundo — volume incompatível com um stream
  de eventos de negócio.
- As chamadas contêm PII falada (cartão, SIN, código postal, data de nascimento), em dois
  idiomas, sob PIPEDA/Lei 25 — qualquer cópia não redigida do texto é passivo de compliance.
- Billing por uso (minutos, STT, tokens, TTS) precisa ser auditável por tenant.

## Decisão

**Os eventos são a fonte da verdade; todo estado é derivado: `state = fold(events)`.** A função
de fold é pura e determinística, e é a mesma para runtime, replay, análise pós-chamada e billing.

Regras que compõem a decisão:

1. **Envelope obrigatório** em todo evento: `eventId` (ULID, ordenável por tempo), `callId`,
   **`seq` monotônico por chamada**, `ts`, `tenantId`, `flowId` + `flowVersion`, `type`,
   `nodeId`, `payload`. O `seq` existe porque NATS é at-least-once e sem ordem: ele é o
   mecanismo de dedup e de ordenação no consumidor — não é opcional.
2. **Transporte: NATS JetStream**, semântica at-least-once assumida; consumidores são
   idempotentes por (`callId`, `seq`).
3. **Catálogo de eventos** (nota 05): `call.started/answered/ended`, `consent.captured`,
   `language.detected/switched`, `node.entered/exited`, `stt.final`, `agent.turn.completed`,
   `tool.invoked/completed`, `var.assigned`, `dtmf.received`, `transfer.initiated/answered`,
   `recording.started`, `error.raised`.
4. **`stt.partial` nunca vai ao bus.** Parciais ficam no caminho quente
   (gateway ↔ agent-runtime); se algum dia precisarem ser persistidos, será em stream separado
   com retenção de horas — nunca no stream de eventos de negócio.
5. **PII é redigida na emissão, não depois.** O texto entra no evento já redigido, com
   `redactions: [{start, end, kind, vault}]` apontando para o conteúdo original guardado no
   vault (cifrado por DEK — ver crypto-shredding, nota 08). Detectores mínimos: cartão (Luhn),
   SIN/NAS, código postal `A1A 1A1`, data de nascimento — rodando nos **dois idiomas** (números
   falados em francês não caem em regex de dígitos).
6. **Billing é um fold sobre o stream.** Como todo evento carrega `tenantId`, o usage
   (`CallMinutes`, `STTSeconds`, `LLMTokensIn/Out`, `TTSChars`) sai do mesmo stream que
   alimenta o resto — sem pipeline de billing separado.

O snapshot produzido pelo fold (`status`, `locale`, `cursor`, `vars`, `transcript` com redações,
`path`, `budget`) é também o mecanismo de reidratação de chamadas após restart.

## Consequências

**Positivas**
- Um único stream alimenta runtime, replay, timeline do dashboard, análise pós-chamada e
  billing; não há divergência possível entre "o que aconteceu" e "o que foi cobrado".
- Reidratação após restart e replay de bugs vêm de graça (fold determinístico).
- Compliance por construção: nenhum consumidor jamais vê PII crua, porque ela não entra no
  stream; exclusão sob Lei 25/PIPEDA = destruir a DEK do vault.
- `flowVersion` no envelope permite análise agregada por versão de fluxo ("onde o fluxo
  sangra") e reprocessamento comparável (`analysisVersion` fixo).

**Negativas / riscos assumidos**
- Idempotência e ordenação viram responsabilidade de todo consumidor (buffer por `seq`,
  descarte de duplicatas) — mais código e mais teste do que um bus exactly-once idealizado.
- O fold precisa ser mantido **puro e determinístico** e compatível com o histórico: mudanças de
  schema de evento exigem versionamento (`contracts/`) e disciplina de evolução.
- Redação na emissão coloca os detectores de PII no caminho da chamada; um detector caro demais
  pressiona o orçamento de latência, um detector fraco vira vazamento (o gate dos evals —
  `piiLeaks > 0` bloqueia publish — é a rede de segurança).
- Como o fold é compartilhado entre Go e Python (ADR-002), o contrato do envelope e a semântica
  do fold precisam de testes de conformidade nos dois lados.

## Alternativas consideradas

1. **Estado mutável em banco + tabela de auditoria.** Rejeitada: estado e histórico divergem
   (a auditoria vira cidadã de segunda classe), replay não é confiável e billing exige pipeline
   próprio. É exatamente o antipadrão que a nota 05 abre condenando.
2. **Exactly-once via Kafka + transações.** Rejeitada para o MVP: a nota 17 já aponta NATS como
   provavelmente suficiente e mais leve; exactly-once real continuaria exigindo consumidores
   idempotentes na prática, então o custo operacional de Kafka não compra a garantia que
   aparenta.
3. **`stt.partial` no bus com filtro no consumidor.** Rejeitada: dezenas de eventos por segundo
   por chamada inflam o stream, o storage do JetStream e todo consumidor, para dados que perdem
   valor em milissegundos.
4. **Redigir PII em pós-processamento (worker sobre o stream).** Rejeitada: entre a emissão e o
   worker existe uma janela em que PII crua está persistida e replicada no bus — sob Lei
   25/PIPEDA isso é passivo real, e nenhum "delete depois" cobre backups e réplicas. Redação na
   emissão + vault cifrado é a única ordem que fecha.
5. **Serviço de billing com medição própria (contadores nos serviços).** Rejeitada: cria uma
   segunda fonte da verdade que precisa ser reconciliada com os eventos; o fold sobre o stream
   dá o mesmo número auditável sem pipeline novo.
