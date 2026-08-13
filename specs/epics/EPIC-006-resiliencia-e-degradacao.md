# EPIC-006 — Resiliência e degradação

> Fonte: `specs/CONTEXT.md` · nota `07 - Resiliência e Degradação` (canônica para este EPIC), `06 - Máquina de Estados do Turno` (filler).

## Objetivo

Implementar a escada de degradação (`degNormal → degFast → degScripted → degTransfer → degVoicemail`) e os padrões de falha do pipeline de voz: ring buffer de STT com reconexão rápida, hedge de LLM, banco de áudio pré-sintetizado e circuit breaker por provedor. Princípio: orçamento de falha medido em milissegundos de silêncio — nunca retry com backoff exponencial. Garantia de produto: agente falhou → chamada cai no celular do dono, nunca em silêncio (G5).

## Escopo

- Escada de degradação por chamada (decisão temporária, informada pela saúde global do provedor); cada nível funcional, só que pior. `degScripted` usa respostas enlatadas + DTMF; `degTransfer` envia a chamada ao celular do dono com contexto; `degVoicemail` como último degrau.
- Janelas de reação da nota 07: STT desconecta ~200ms; STT sem resultado 1,5s → reprompt bilíngue; LLM 429/timeout ~1,2s → escada; TTS falha 0ms → banco de áudio.
- STT com ring buffer: buffer recebe sempre, mesmo desconectado; reconexão 3 tentativas 0/80/200ms; fallback de fornecedor; replay do que foi falado; falha final → `degScripted` (a chamada vive). DTMF chega por canal independente do pipeline de áudio.
- LLM hedge, não retry: secundário disparado após `hedgeAfter` (~700ms ≈ p90 do primário), vence quem responder primeiro; `ErrAllProvidersFailed` nunca chega ao cliente — vira frase enlatada + `degTransfer`.
- Banco de áudio pré-sintetizado: ~15 frases por locale (greeting, one_moment, didnt_catch, transferring, technical_issue, goodbye…), geradas no build e versionadas com a persona; cobre `degScripted`/`degTransfer` e serve de filler ao loop do turno.
- Circuit breaker POR PROVEDOR (nunca por chamada), meio-aberto com 1 sonda a cada 5s; compartilhado entre todas as chamadas do processo.
- Métricas: `silence_gap_ms` (p99), `degradation_level` (distribuição), `barge_in_false_positive_rate`, `provider_breaker_state`; alerta: `degradation_level > degNormal` por >60s em >5% das chamadas ativas (exportadas via EPIC-010).
- Testes de caos no harness: matar STT/LLM/TTS em cada estado do turno e medir silêncio percebido.

## Fora de escopo

- Reinício com chamadas ativas/rehidratação de snapshot e blue/green de app de mídia (padrões Asterisk/ARI da referência; com Telnyx no MVP, ficam registrados como dívida consciente para a v2 de mídia própria).
- Failover de infraestrutura (multi-AZ, K8s) — operação, não produto.
- Conteúdo das frases enlatadas por persona (config de tenant, EPIC-004); aqui entra o mecanismo de geração/versionamento/playback.

## User stories

**US-6.1 — Como cliente, quero que a queda do STT seja imperceptível.**
- Aceite (caos no harness): desconexão de STT em `stListening` → reconexão com replay do ring buffer; nenhum áudio perdido na transcrição final; silêncio adicional < 300ms; falha das 3 tentativas + fallback → chamada segue em `degScripted` com DTMF funcional.

**US-6.2 — Como cliente, quero resposta mesmo quando o LLM primário está lento ou fora.**
- Aceite: primário >700ms dispara hedge; resposta usada é a do primeiro a completar; custo duplicado registrado por turno (esperado ~10%); todos os provedores falhando → frase enlatada + `degTransfer`, silêncio total < 1,2s.

**US-6.3 — Como cliente, quero ouvir uma voz mesmo com o TTS fora do ar.**
- Aceite: falha de TTS responde com frase do banco de áudio em 0ms de espera adicional; banco cobre os dois locales; build falha se alguma das ~15 frases obrigatórias faltar para um locale/persona; troca de persona regenera e versiona o banco.

**US-6.4 — Como operador, quero que a falha de um provedor não vire tempestade de reconexões.**
- Aceite: com 200 chamadas simuladas e provedor caído, o breaker abre e o total de tentativas fica limitado à sonda (1/5s) — não 600 tentativas em 200ms; estado do breaker exportado como métrica; fechamento gradual via meio-aberto.

**US-6.5 — Como dono, quero a garantia de que nenhuma chamada morre em silêncio.**
- Aceite: teste de caos derrubando agent-runtime inteiro no meio da chamada termina com a chamada no celular do dono (`degTransfer`) ou voicemail — nunca hangup silencioso; evento `error.raised` + trilha de degradação registrada.

## Dependências

- **EPIC-001** (contratos, eventos de erro/degradação).
- **EPIC-002** (pontos de acoplamento no loop do turno: filler, flush, estados).
- **EPIC-003** (providers atrás de interfaces — hedge e fallback compõem sobre elas).
- **EPIC-010** (exportação das métricas e alertas).
- **EPIC-009** valida os cenários de caos como parte da suíte.

## Definição de pronto

- Escada completa exercitada por testes de caos automatizados no CI (unit/flow) e verificada em chamada real com falha injetada.
- `silence_gap_ms` p99 dentro do orçamento em todos os cenários de falha simulados.
- Banco de áudio versionado com a persona e integrado ao filler do turno.
- Breakers por provedor com métricas e alerta configurados; nenhuma chamada termina em silêncio em nenhum teste.
