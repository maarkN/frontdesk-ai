# EPIC-003 — agent-runtime (Python)

> Fonte: `specs/CONTEXT.md` · notas `04 - Grafo da URA`, `16 - Receptionist (Arquitetura e Requisitos)`, `06 - Máquina de Estados do Turno` (troca de idioma).

## Objetivo

Runtime do agente em Python 3.12: grafo LangGraph do fluxo do Receptionist (saudação → intenção → qualificação → agendamento/recado → confirmação), providers STT/LLM/TTS atrás de `Protocol`, cascata de custo Haiku/Sonnet, guardrails e detecção/troca de idioma EN/FR.

## Escopo

- Grafo LangGraph do fluxo RF2: saudação, intenção, qualificação (serviço, código postal vs área de cobertura, urgência), agendamento (Google Calendar com slots reais) ou recado estruturado, confirmação.
- Providers atrás de `Protocol` (STT: Deepgram streaming; LLM: Claude; TTS: Cartesia ou ElevenLabs Flash com word timings obrigatórios), factory por tenant (BYOK futuro) e validação de região com `ErrResidencyViolation` em runtime.
- Cascata de LLM: Haiku para turnos simples, Sonnet para raciocínio/tool use; critério de roteamento explícito e testável.
- Detecção de idioma no 1º turno (meta ≥95%) e troca EN↔FR no meio da chamada sem reiniciar o nó (troca voz TTS + prompt; emite `language.detected`/`language.switched`).
- Guardrails: não cotar preço fechado, não prometer fora da config do tenant, emergência → sinalizar transferência ao telephony-gw.
- Tool use: Google Calendar (disponibilidade + criação de evento), registro de recado estruturado, consulta de área de cobertura; efeitos colaterais com `idempotencyKey: "{callId}:{nodeId}:{attempt}"`.
- Emissão de eventos conforme `contracts/` (node.entered/exited, agent.turn.completed, tool.invoked/completed, var.assigned, language.*), com redação de PII na emissão (Luhn, SIN, código postal `A1A 1A1`, data de nascimento, nos dois idiomas).
- Modo passthrough de STT/TTS (texto puro) para a camada flow sim dos evals (EPIC-009).
- pydantic v2 para modelos, mypy strict, pytest.

## Fora de escopo

- Máquina de estados do turno, barge-in, endpointing (EPIC-002 — o runtime recebe turnos prontos).
- Hedge de LLM, circuit breaker, banco de áudio (EPIC-006 — este EPIC deixa os providers atrás de interfaces para o EPIC-006 compor).
- Suíte de evals em si (EPIC-009); aqui entra apenas o modo passthrough.
- Envio de SMS/e-mail (EPIC-005).

## User stories

**US-3.1 — Como cliente, quero falar em francês e ser atendido em FR-CA do primeiro turno ao fim.**
- Aceite: detecção de idioma no 1º turno ≥95% na suíte de fixtures EN/FR; `language.detected` emitido; prompts e voz corretos para o locale.

**US-3.2 — Como cliente bilíngue, quero trocar de idioma no meio da chamada sem que o agente recomece.**
- Aceite: troca no 3º turno mantém o nó atual e as variáveis coletadas; `language.switched` emitido; flow sim cobre o cenário (caller_code_switcher).

**US-3.3 — Como dono, quero que o agente qualifique e agende de verdade no meu Google Calendar.**
- Aceite: fluxo completo coleta serviço, código postal (validado contra área de cobertura), urgência; oferece slots reais; evento criado com `idempotencyKey` — replay do evento não duplica agendamento; fora da área de cobertura → recado estruturado com o motivo.

**US-3.4 — Como dono, quero que o agente nunca feche preço nem prometa o que não configurei.**
- Aceite: asserções determinísticas nos evals: pedido de orçamento retorna faixa/estimativa conforme config, nunca valor fechado; pedido fora da config resulta em recado ou transferência, nunca promessa.

**US-3.5 — Como operador, quero trocar de fornecedor de STT/LLM/TTS sem tocar no grafo.**
- Aceite: providers implementam `Protocol`; teste sobe o grafo com providers fake; factory por tenant escolhe implementação; provider fora de ca-central-1 para tenant com residência exigida levanta `ErrResidencyViolation`.

**US-3.6 — Como responsável por custo, quero a cascata Haiku/Sonnet com roteamento auditável.**
- Aceite: turnos simples (confirmação, coleta de slot) vão ao Haiku; raciocínio/tool use vai ao Sonnet; decisão registrada por turno (evento/atributo OTel); distribuição verificável na suíte flow sim.

## Dependências

- **EPIC-001** (contratos, modelos pydantic, CI).
- **EPIC-002** (transporte de turnos e áudio; desenvolvimento paralelo possível via passthrough de texto).
- **EPIC-004** (config de tenant: persona, área de cobertura, calendário, guardrails).
- **EPIC-006** e **EPIC-009** consomem as interfaces deste EPIC.

## Definição de pronto

- Fluxo RF2 completo rodando ponta a ponta com telephony-gw em EN e FR.
- Detecção de idioma ≥95% e troca sem reinício de nó, cobertas por evals.
- Guardrails cobertos por asserções determinísticas; `stt.partial` nunca emitido; PII redigida na emissão com testes nos dois idiomas.
- mypy strict e ruff limpos no CI; grafo executável em modo passthrough (< ~2s por conversa simulada).
