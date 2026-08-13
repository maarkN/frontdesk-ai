# ADR-004 — Telnyx como carrier no MVP; Asterisk próprio só na v2

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** notas 01, 16, 17 do vault `maarkN/Voice AI/`; CONTEXT.md
- **Depende de:** ADR-001

## Contexto

O produto precisa de: números locais canadenses (DID), receber chamadas PSTN, media streams em
tempo real para o `telephony-gw`, transferência a quente para o celular do dono e gravação.
Operar telefonia no Canadá traz obrigações regulatórias e operacionais reais: CRTC, E911,
STIR/SHAKEN, interconexão com carriers, NAT/RTP, transcodificação.

A arquitetura de referência (notas 02–09) nasce sobre **Asterisk próprio** — e o owner tem
know-how direto nisso (sensevox: VoIP Asterisk/PJSIP em produção, −60% de custo vs Twilio). A
tentação de começar com mídia própria é concreta.

Mas a nota 01 registra a inversão deliberada do Receptionist: começar com **Telnyx como
carrier** (sem Asterisk próprio) e adicionar Asterisk só na v2, como alavanca de margem — o
inverso da referência. A nota 15 lista Asterisk próprio explicitamente como fora de escopo do
MVP, e a nota 17 o posiciona na v2 (meses 8–14).

Meta econômica relevante: G6 — custo variável/min < 30% do preço/min cobrado (margem alvo
≥ 70% sobre o excedente de ~CAD 0,35/min).

## Decisão

**Telnyx é o carrier e a camada de mídia do MVP.** O `telephony-gw` (Go) consome media streams
por WebSocket e faz call control pela API da Telnyx; não há Asterisk, SIP trunk próprio nem
RTPengine no MVP.

Racional (nota 16): Telnyx **absorve CRTC, E911 e STIR/SHAKEN**, fornece DID canadense e media
streams por WS, e é barato em relação à Twilio. Todo o esforço de engenharia do MVP vai para o
que diferencia o produto (turno rápido, bilinguismo FR-CA, agendamento real), não para operar
telefonia.

**Asterisk/RTPengine entram na v2 (meses 8–14) como alavanca de margem**, mantendo a Telnyx só
como carrier de interconexão — o know-how do sensevox é o ativo que torna essa migração crível.
Condição implícita: volume de minutos suficiente para o ganho de margem pagar o custo
operacional de mídia própria.

Salvaguarda de arquitetura: o `telephony-gw` isola o carrier atrás da própria fronteira do
serviço (padrão "provider como interface", nota 01) — o resto do sistema vê frames de áudio e
eventos de chamada, nunca a API da Telnyx. A troca da camada de mídia na v2 fica confinada a um
serviço. A nota 17 mantém aberta a cotação real Telnyx vs Twilio vs Vonage (DID canadense +
media streams); esta ADR fixa a *categoria* da decisão (CPaaS gerenciado, sem mídia própria) e
adota Telnyx como escolha default até a cotação dizer o contrário.

## Consequências

**Positivas**
- Compliance de telefonia (CRTC, E911, STIR/SHAKEN) vira problema do carrier — de outro modo
  seria trabalho regulatório e operacional antes do primeiro cliente.
- Time-to-market: M1 do roadmap ("número Telnyx, media streams WS, call control, barge-in") é
  atingível em semanas por um solo founder; com Asterisk próprio não seria.
- O spike T1 (Telnyx → WS → Deepgram → Claude → Cartesia/ElevenLabs) valida latência e FR-CA
  sobre a mesma infraestrutura que o MVP usará — o resultado do spike transfere.
- Zero infraestrutura de mídia para operar (patches de Asterisk, NAT, monitoração de RTP).

**Negativas / riscos assumidos**
- **Margem menor no MVP**: paga-se o markup de mídia da Telnyx por minuto. Aceito porque a meta
  G6 (< 30% do preço/min) é atingível mesmo assim, e a compressão de margem tem rota de saída
  planejada (v2). Medição de custo/min no spike e no piloto é obrigatória (risco "custo come a
  margem", nota 17).
- **Dependência de fornecedor único no caminho crítico** da chamada: um incidente Telnyx derruba
  o produto. Mitigado parcialmente pela escada de degradação (failover: agente falhou → chamada
  cai no celular do dono, nunca em silêncio) e pela fronteira de serviço que permite um segundo
  carrier no futuro.
- A latência da perna de mídia (PSTN → Telnyx → WS) não está sob nosso controle; entra no
  orçamento do turno como custo fixo a medir no spike.
- O know-how de Asterisk (sensevox) fica temporariamente sem uso — é dívida estratégica
  consciente, com data de resgate (v2).

## Alternativas consideradas

1. **Asterisk próprio desde o MVP (como a referência).** Rejeitada: transforma o MVP num
   projeto de operação de telefonia — E911, STIR/SHAKEN, interconexão, NAT/RTP — antes de
   validar o produto. A referência podia nascer sobre Asterisk porque é um blueprint técnico;
   um produto pré-validação, tocado por um solo founder, não pode. O ganho de margem só importa
   com volume, que o MVP ainda não tem.
2. **Twilio.** Preterida: mesma categoria (CPaaS gerenciado), mas custo por minuto maior — o
   próprio histórico do sensevox (−60% vs Twilio) e a nota 16 ("barato vs Twilio") apontam
   Telnyx. Permanece como candidata na cotação aberta da nota 17.
3. **Vonage.** Preterida pelo mesmo motivo; entra na mesma cotação comparativa (DID canadense +
   media streams) antes do commit definitivo.
4. **Plataformas de voice-AI (Vapi/Retell) em vez de carrier + stack própria.** Rejeitada:
   terceirizam exatamente o que é o diferencial do produto (latência do turno, endpointing e
   barge-in próprios, FR-CA de verdade, guardrails) e comprometem G6 com camada extra de
   markup. A nota 15 as classifica como plataforma para devs, não como base do nosso produto.
5. **Híbrido já no MVP (Telnyx como carrier + mídia própria via RTPengine).** Rejeitada: é
   precisamente o desenho da v2; antecipá-lo recoloca o custo operacional de mídia própria no
   caminho do MVP sem o volume que o justifica.
