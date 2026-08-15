# ADR-009 — Asterisk como backend de mídia alternativo, selecionável por config

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** notas 01, 07, 16, 17 do vault `maarkN/Voice AI/`; CONTEXT.md
- **Depende de:** ADR-002, ADR-004, ADR-006

## Contexto

O ADR-004 fixou Telnyx como carrier e camada de mídia do MVP e empurrou Asterisk próprio para a
v2 (meses 8–14), como alavanca de margem. A salvaguarda de arquitetura daquele ADR — o
`telephony-gw` isola o carrier atrás da própria fronteira do serviço; o resto do sistema vê
frames de áudio e eventos de chamada, nunca a API da Telnyx — foi implementada como interfaces
de call-control e de mídia dentro do `telephony-gw`.

Com essas interfaces já de pé, o custo de *antecipar o esqueleto* do backend Asterisk caiu
drasticamente: não é mais "operar telefonia própria no MVP" (o que o ADR-004 rejeitou), e sim
adicionar uma segunda implementação atrás de interfaces existentes, desligada por default. O
owner tem know-how direto de Asterisk/PJSIP em produção (sensevox, −60% de custo vs Twilio), e
a nota 07 já descreve os padrões operacionais necessários (recuperação de canais no boot,
blue/green por app Stasis).

Antecipar o backend agora tem três motivações: (a) validar cedo que as interfaces de
call-control/mídia realmente não vazam Telnyx (a segunda implementação é o teste da abstração);
(b) reduzir o risco da migração de margem da v2 diluindo-a em incrementos; (c) permitir
laboratório de latência da perna de mídia sob nosso controle.

## Decisão

**O `telephony-gw` ganha um backend de mídia ALTERNATIVO baseado em Asterisk, atrás das
interfaces de call-control e mídia já existentes. Telnyx continua sendo o default do MVP.**

- **Seleção por config**: `MEDIA_BACKEND=telnyx|asterisk`, resolvida no boot do `telephony-gw`.
  Default: `telnyx`. Nenhum código fora do `telephony-gw` enxerga qual backend está ativo — a
  fronteira do ADR-004 permanece intacta.
- **Stack Asterisk**: **Asterisk 22 LTS** com **chan_pjsip** (chan_sip está morto). Call control
  via ARI (app Stasis); o cliente Go usa `github.com/CyCoreSystems/ari/v6`.
- **Blue/green por nome de app Stasis** (nota 07): o dialplan invoca
  `Stasis(${GLOBAL(ACTIVE_APP)})`; deploy sobe `voice-vN+1`, aponta a global e a versão antiga
  drena naturalmente (readiness `not ready`, processo vivo até esvaziar, teto `maxDrainSec`).
- **Recuperação no boot** (nota 07): a fonte da verdade do canal é o Asterisk — listar canais
  vivos do app Stasis, carregar snapshot e retomar; canal sem snapshot → transferir para humano
  (`recovered_orphan`), nunca improvisar.
- O que **não** muda: Telnyx segue como carrier de interconexão PSTN em qualquer cenário
  (CRTC, E911, STIR/SHAKEN continuam problema do carrier, como no ADR-004). O backend Asterisk
  não entra no caminho de produção do MVP; entra em ambiente de laboratório/staging até que a
  condição econômica da v2 (volume de minutos) o justifique.

## Consequências

**Positivas**
- A segunda implementação é o teste real da abstração: qualquer vazamento de Telnyx nas
  interfaces de call-control/mídia aparece agora, enquanto é barato corrigir.
- A migração de margem da v2 deixa de ser um big bang: quando o volume justificar, a troca é
  `MEDIA_BACKEND=asterisk` por tenant/ambiente, não um projeto novo.
- Latência da perna de mídia vira variável controlável em laboratório (RTP local vs WS Telnyx),
  alimentando o orçamento de turno (G1) com dados próprios.
- O know-how sensevox sai da prateleira com data de uso, sem contaminar o caminho crítico do MVP.

**Negativas / riscos assumidos**
- Superfície de código maior no `telephony-gw` — duas implementações para manter. Mitigado:
  suíte de conformidade única roda contra ambos os backends.
- Risco de distração do solo founder (o ADR-004 rejeitou Asterisk no MVP exatamente por foco).
  Mitigado: o backend Asterisk é explicitamente fora do caminho de produção do MVP; não bloqueia
  nenhum marco de produto.
- Infraestrutura de Asterisk em lab exige alguma operação (dialplan, PJSIP config) — aceito por
  ser ambiente controlado, sem SLA.

## Alternativas consideradas

1. **Manter Asterisk 100% na v2 (status quo do ADR-004).** Rejeitada como *timing*: com as
   interfaces prontas, o custo de antecipar o esqueleto é baixo e o valor de validar a abstração
   é alto. A *decisão econômica* do ADR-004 (Telnyx default, mídia própria só com volume)
   permanece.
2. **Tornar Asterisk o default já.** Rejeitada: recoloca operação de telefonia no caminho do
   MVP sem o volume que a justifica — exatamente o que o ADR-004 rejeitou.
3. **FreeSWITCH em vez de Asterisk.** Rejeitada: o know-how do owner (sensevox) é
   Asterisk/PJSIP; a nota 07 descreve os padrões operacionais em termos de Stasis/ARI.
4. **Seleção por flag de build em vez de config.** Rejeitada: `MEDIA_BACKEND` por config permite
   blue/green e rollout por ambiente/tenant sem recompilar.
