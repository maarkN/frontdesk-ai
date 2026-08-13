# EPIC-002 — telephony-gw (Go)

> Fonte: `specs/CONTEXT.md` · notas `03 - Arquitetura Geral e Latência`, `06 - Máquina de Estados do Turno`, `16 - Receptionist (Arquitetura e Requisitos)`.

## Objetivo

Gateway de telefonia em Go: terminação Telnyx (media streams por WS + call control), 1 goroutine por chamada rodando a máquina de estados do turno (`stListening → stEndpointing → stThinking → stSpeaking`), barge-in em dois níveis, `PlayoutTracker` e endpointing adaptativo EN/FR. É o componente que sustenta o orçamento de latência (G1: turno p50 < 1,2s, p95 < 2,5s).

## Escopo

- Integração Telnyx: webhook de call control (answer, hangup, transfer, DTMF), media WS bidirecional, jitter buffer de entrada.
- Resolução de tenant pelo DID no início da chamada (DID desconhecido → hangup) e propagação via `context.Context` — nenhuma função abaixo recebe `tenantID` como parâmetro.
- Gate de consent: aviso de gravação antes de gravar/entregar ao agente (PIPEDA/Lei 25), emitindo `consent.captured`.
- Máquina de estados do turno em loop `select`; STT recebe frames em TODOS os estados (pré-requisito do barge-in).
- Endpointing adaptativo por idioma: hesitação (~1400ms; "um/uh" EN, "euh/ben/tsé" FR-CA), frase incompleta (~900ms), ditando números (~1100ms), pergunta (~400ms), default (~600ms). Regras EN nunca aplicadas a FR.
- Barge-in em dois níveis: energia → ducking (~60ms) → confirmação por STT parcial; backchannel ("uh huh"/"ouais"/"han han") desfaz ducking sem cortar; texto real ≥ `minChars` cancela LLM, para TTS, faz flush de playback.
- `PlayoutTracker`: histórico contém apenas o que o cliente ouviu; `Spoken()` trunca na palavra via word timings do TTS, marca corte com "—", compensa offset `writtenMs` (100–200ms calibrados).
- Cancelamento em cascata via `context.Context`; produtores com `select { case ch <- tok: case <-ctx.Done(): return }` — zero vazamento de goroutine.
- Filler: LLM > 600ms → frase enlatada do banco de áudio (integração com EPIC-006).
- Timing por persona como configuração (`bargeIn.minChars`, `endpoint.min/maxSilenceMs`, `fillerAfterMs`, `playoutOffsetMs`), não código.
- Emissão de eventos de chamada ao NATS conforme `contracts/` (call.started/answered/ended, stt.final, dtmf.received, transfer.initiated/answered, recording.started...), com redação de PII na emissão.
- Transferência a quente por regra (emergência/palavra-chave → celular do dono) e modos transbordo/sempre-IA (config vinda do core-api).
- Harness de teste com WAV sintético em vez de socket real (camada unit dos evals, EPIC-009).

## Fora de escopo

- Lógica conversacional, prompts, tools (EPIC-003).
- Escada de degradação completa, circuit breakers, ring buffer STT (EPIC-006 — este EPIC deixa os pontos de acoplamento prontos).
- Asterisk/mídia própria (alavanca de margem v2, fora do MVP).
- Provisionamento de DIDs e CRUD de configuração (EPIC-004).

## User stories

**US-2.1 — Como cliente ligando, quero ser atendido em segundos com o aviso de gravação no meu idioma provável.**
- Aceite: chamada inbound atendida < 2s após decisão de transbordo; `consent.captured` emitido antes de qualquer frame ir ao agente; DID desconhecido derruba com log estruturado.

**US-2.2 — Como cliente, quero interromper o bot e ser ouvido imediatamente.**
- Aceite (harness WAV): fala por cima aos 1,2s → ducking ≤ 100ms do onset de energia, TTS cortado após confirmação do parcial e `Spoken()` bate com o que saiu na linha; "mhm" no meio da fala do bot NÃO interrompe; ruído a −30 dB por 60s → zero barge-ins falsos.

**US-2.3 — Como falante de FR-CA, quero ditar um número ou hesitar sem ser cortado.**
- Aceite (harness WAV): pausa de 900ms ditando número não corta o turno; hesitação "euh" estende a janela para ~1400ms; perfis de endpointing selecionados pelo idioma ativo da chamada.

**US-2.4 — Como agent-runtime, quero que o histórico reflita só o que o cliente ouviu.**
- Aceite: turno interrompido gera `agent.turn.completed` com texto truncado na palavra + marcador "—"; teste compara com word timings do TTS; offset de playout configurável e calibrado.

**US-2.5 — Como operador, quero garantia de que chamadas encerradas não deixam goroutines vivas.**
- Aceite: teste com `goleak` após N chamadas simuladas (incluindo hangup no meio de `stThinking` e `stSpeaking`) não detecta vazamento; cancelamento derruba LLM/TTS em cascata.

**US-2.6 — Como dono, quero que emergência ("flood", "burst pipe") transfira a quente para meu celular.**
- Aceite: palavra-chave de emergência dispara `transfer.initiated` e ponte com o celular configurado; contexto da chamada preservado no evento.

## Dependências

- **EPIC-001** (contratos, envelope, CI, NATS local).
- **EPIC-003** (interface de conversação — pode evoluir em paralelo com stub de agente eco).
- **EPIC-004** (config de tenant/DID/modo — pode começar com config estática).
- **EPIC-006** consome os hooks deste EPIC (filler, flush, pontos de degradação).

## Definição de pronto

- Chamada real via Telnyx completa o ciclo inbound → consent → turnos → hangup com eventos corretos no JetStream.
- Todos os cenários do harness WAV da nota 06 passando no CI (barge-in, backchannel, endpointing, filler, troca de idioma sem reiniciar nó).
- Latência instrumentada por turno (hooks para EPIC-010); silêncio pós-fala dentro do orçamento em bench local.
- Zero `tenantID` em assinatura de função; `stt.partial` jamais publicado no bus (teste garante).
