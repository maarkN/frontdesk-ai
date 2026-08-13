# Contratos de eventos (`contracts/events/`)

Fonte da verdade dos eventos trocados entre Go (telephony-gw, core-api, notifier),
Python (agent-runtime) e TypeScript (dashboard). Todo estado é derivado do stream:
`state = fold(events)` (ver ADR-003).

## Layout e versionamento

```
contracts/events/
  README.md
  v1/
    envelope.schema.json   # envelope obrigatório de todo evento
    catalog.schema.json    # payloads tipados do catálogo (um $def por tipo)
```

- Cada versão do contrato vive num diretório próprio (`v1/`, `v2/`, ...). Os schemas de uma
  versão publicada são **imutáveis**.
- Mudança **compatível** (adicionar campo opcional, adicionar novo tipo de evento ao catálogo)
  pode entrar na versão corrente.
- Mudança **breaking** (remover/renomear campo, mudar tipo de campo, tornar campo opcional em
  obrigatório, remover tipo do catálogo) exige um novo diretório de versão (`v2/`). O CI compara
  os schemas contra a versão publicada e bloqueia PRs que quebrem sem bump (EPIC-001, US-1.3).
- Consumidores declaram a versão que entendem; produtores publicam sempre na versão corrente.

## Envelope

Todo evento carrega o envelope de `envelope.schema.json`:

| Campo | Tipo | Nota |
|---|---|---|
| `eventId` | ULID | ordenável por tempo; chave de **dedup** (`Nats-Msg-Id`) |
| `callId` | string | agrupa o stream de uma chamada |
| `seq` | inteiro ≥ 1 | **monotônico por chamada**; dedup + ordenação no consumidor |
| `ts` | RFC 3339 | instante de emissão |
| `tenantId` | string | presente em todo evento (billing = fold por tenant) |
| `flowId` / `flowVersion` | string / int | versão do fluxo que gerou o evento |
| `type` | enum | um dos tipos do catálogo |
| `nodeId` | string | nó do fluxo (vazio quando não se aplica) |
| `payload` | objeto | tipado por `type` (ver `catalog.schema.json`) |
| `redactions` | array | spans de PII redigida: `{start, end, kind, vault}` |

## Dedup e ordenação (NATS JetStream é at-least-once, sem ordem)

- O produtor publica com `Nats-Msg-Id = eventId` (janela de dedup do JetStream).
- O consumidor é idempotente por (`callId`, `seq`): descarta `seq` repetido e ordena por `seq`
  antes do fold. O fold de referência é determinístico: mesma sequência de eventos (após dedup e
  ordenação) ⇒ mesmo snapshot, em qualquer linguagem.

## Catálogo (v1)

`call.started`, `call.answered`, `call.ended`, `consent.captured`, `language.detected`,
`language.switched`, `node.entered`, `node.exited`, `stt.final`, `agent.turn.completed`,
`tool.invoked`, `tool.completed`, `var.assigned`, `dtmf.received`, `transfer.initiated`,
`transfer.answered`, `recording.started`, `error.raised`.

## `stt.partial` NUNCA vai ao bus

Parciais de STT são emitidos dezenas de vezes por segundo e vivem apenas no caminho quente
(gateway ↔ agent-runtime). Eles **não existem neste contrato** e nenhum produtor pode
publicá-los no stream de eventos de negócio. Se um dia precisarem ser persistidos, será em
stream separado com retenção de horas — nunca aqui (ADR-003, decisão 4).

## PII

O texto entra no evento **já redigido na emissão**. `redactions[]` aponta os spans substituídos
(`start`/`end` sobre o texto original, `kind` ∈ `card | sin | postal_code | dob`, `vault` é a
referência ao conteúdo original cifrado por DEK — crypto-shredding). Detectores mínimos, nos
dois idiomas (EN e FR-CA, incluindo números por extenso normalizados pelo STT): cartão (Luhn),
SIN/NAS, código postal `A1A 1A1`, data de nascimento. Nenhum consumidor do stream vê PII crua.
