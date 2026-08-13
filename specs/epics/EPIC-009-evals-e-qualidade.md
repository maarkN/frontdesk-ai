# EPIC-009 — Evals e qualidade

> Fonte: `specs/CONTEXT.md` · nota `09 - Evals de Agentes` (canônica para este EPIC), `06 - Máquina de Estados do Turno` (harness WAV), `17 - Receptionist (Validação e Roadmap)` (M5).

## Objetivo

Suíte de avaliação em três camadas (unit, flow sim, voice regression), clientes simulados adversariais e gate de publish de versão de flow/persona. Asserções determinísticas antes de LLM-as-judge; `piiLeaks > 0` bloqueia publish sem override. "Você não tem um teste, tem uma distribuição."

## Escopo

- **Camada unit** (todo commit, segundos): harness WAV sintético contra a máquina de estados do EPIC-002 — cenários da nota 06 (barge-in, backchannel, endpointing por idioma, filler, troca de idioma, ruído sem falsos positivos).
- **Camada flow sim** (todo commit, centavos): conversa em texto com STT/TTS passthrough (EPIC-003), ~300 casos por PR, conversa inteira em ~2s; sem depender de APIs vivas no CI.
- **Camada voice regression** (noturno + pré-publish): áudio ponta a ponta contra o pipeline real.
- Clientes simulados adversariais: `caller_code_switcher`, `caller_impatient`, `caller_rambler`, `caller_off_topic`, `caller_pii_dumper`, `caller_prompt_injector`, `caller_voicemail`. Os de PII e injection são testes de segurança — gate obrigatório.
- Asserções determinísticas: `ReachedExit(...)`, `CollectedVars(...)`, `MaxTurns(8)`, `NoPIIInEvents()` (zero tolerância), `RespondedInLocale("fr-CA")`, `NoRepeatedQuestion()`, `LatencyP95Under(900ms)`.
- LLM-as-judge só depois das determinísticas, para o subjetivo (tom, naturalidade, fr-CA vs fr-FR): rodar 3× com mediana; validar contra rótulos humanos em ~50 chamadas antes de virar gate.
- Gate de publish: `evalReport` acompanha a versão do flow — `taskSuccess` vs baseline (queda >3 p.p. bloqueia, override explícito e justificado), `containment`, `piiLeaks` (>0 bloqueia SEM override), `localeAccuracy`.
- Ciclo de retroalimentação: chamada real com `disposition: escalate`, sentimento em queda ou failures na análise pós-chamada vira candidata a fixture; aprovação humana → redação → entra na suíte.
- Infra de shadow/A-B: 10% do tráfego na persona nova, persona fixada no início da chamada (como `flowVersion`); comparação de `containment`, `turnsToResolution`, `sentiment.trajectory`.

## Fora de escopo

- Implementação dos comportamentos avaliados (EPICs 002/003/006).
- Dashboards de métricas de produção (EPIC-010; este EPIC consome a análise pós-chamada para fixtures).
- Rotulagem humana em escala (processo manual leve no MVP).

## User stories

**US-9.1 — Como dev, quero que todo PR rode unit + flow sim e falhe se a distribuição regredir.**
- Aceite: CI executa harness WAV + ~300 casos flow sim por PR em tempo aceitável (<15min); relatório com pass rate por asserção e comparação com baseline da main; flake rate da suíte monitorado.

**US-9.2 — Como responsável por segurança, quero que vazamento de PII e prompt injection bloqueiem o publish sem exceção.**
- Aceite: `caller_pii_dumper` e `caller_prompt_injector` presentes em toda execução de gate; `NoPIIInEvents()` varre TODOS os eventos emitidos na simulação; `piiLeaks > 0` bloqueia sem mecanismo de override; injection bem-sucedida (desconto, quebra de guardrail) bloqueia.

**US-9.3 — Como dono do produto, quero publicar persona/flow novo só com evalReport aprovado.**
- Aceite: publish exige `evalReport` da versão exata; queda >3 p.p. em `taskSuccess` bloqueia com override explícito e justificado em registro auditável; `localeAccuracy` abaixo da meta (≥95%) bloqueia.

**US-9.4 — Como avaliador, quero um juiz LLM calibrado antes de confiar nele.**
- Aceite: juiz validado contra rótulos humanos em ~50 chamadas com concordância mínima definida; execução 3× com mediana; juiz nunca substitui asserção determinística existente.

**US-9.5 — Como time, quero que fracassos reais de produção virem fixtures.**
- Aceite: pipeline lista candidatas (escalate/sentimento/failures) com fluxo de aprovação humana; fixture aprovada entra redigida na suíte flow sim; origem rastreável ao `callId`.

## Dependências

- **EPIC-001** (contratos — `NoPIIInEvents` valida contra o envelope e as regras de redação).
- **EPIC-002** (harness WAV da camada unit).
- **EPIC-003** (modo passthrough para flow sim; grafo avaliado).
- **EPIC-006** (cenários de caos entram na suíte).
- **EPIC-010** (análise pós-chamada alimenta o ciclo de fixtures).

## Definição de pronto

- Três camadas operantes: unit + flow sim em todo commit, voice regression noturna.
- Sete callers adversariais implementados; gate de publish ativo com as regras de bloqueio.
- Juiz LLM calibrado e documentado; asserções determinísticas cobrindo guardrails, locale e latência.
- Primeiras fixtures derivadas de chamadas de piloto incorporadas à suíte.
