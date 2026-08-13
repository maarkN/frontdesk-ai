# PRD v1.0 — FrontDesk AI (AI Voice Receptionist)

> **Status:** v1.0 — 2026-08-13 · Owner: Marco Filho (`maarkN`)
> **Fontes:** notas 15–17 do vault (`maarkN/Voice AI/`) + `specs/CONTEXT.md`. Em conflito, as notas vencem.
> Supersede o PRD v0.1 (2026-07-17). Desvios em relação ao original estão marcados na seção [Desvios do PRD original](#11-desvios-do-prd-original).

---

## 1. Visão e dor

PMEs de serviço no Canadá (encanadores, eletricistas, HVAC; clínicas como vertical 2) **perdem negócio porque não atendem o telefone** — o dono está executando o serviço quando o próximo cliente liga. Voicemail não converte: quem tem emergência liga para o próximo da lista do Google Maps. Um job médio vale centenas a milhares de CAD; perder 2–3 ligações/semana é perder milhares/mês.

Por que as soluções atuais falham:

- Voicemail não converte.
- Answering service humano é caro, engessado e não agenda no sistema real do negócio.
- "Text me instead" perde quem quer falar.
- Vapi/Retell são plataformas para devs, não produto para encanador.
- Produtos prontos (Smith.ai, Goodcall, Rosie) são US-first, sem FR-CA decente.

**O produto:** FrontDesk AI atende a linha do negócio em segundos, no idioma do cliente (EN/FR-CA), qualifica (serviço, código postal, urgência), **agenda direto no Google Calendar** ou registra recado estruturado, e envia SMS de resumo ao dono (<60s) + confirmação ao cliente.

## 2. Tese

**A cunha é a linha telefônica.** O omnichannel completo não é o MVP — é o roadmap (v3). A entrada é a linha do negócio, canal dominante no Canadá para negócio local, com receita desde o mês 1. Depois de dono da linha, adicionar SMS/e-mail/WhatsApp/Instagram é evolução natural.

**Modo padrão: transbordo.** A IA só entra se ninguém atender em X segundos. Isso remove a objeção "não quero robô atendendo meus clientes" — e este modo "só transbordo" é provavelmente o produto inteiro no início.

**Diferencial defensável:** bilíngue EN/FR-CA de verdade (a Lei 96 pressiona negócios em Québec a atender em francês; os players americanos tratam francês como segunda classe) + foco vertical com integrações de agenda dos trades (Jobber é canadense). O risco técnico do diferencial (STT/TTS em québécois) é deriscado pelo spike T1 **antes** de qualquer código de produto.

## 3. Personas

| Persona | Quem | Necessidade |
|---|---|---|
| **P1 — Dono de trade** (comprador) | Encanador/eletricista/HVAC, 2–15 pessoas | Não perder job; setup em minutos; confiança de que a IA não "queima" clientes |
| **P2 — Cliente final** (usuário involuntário) | Morador com problema, às vezes urgente, EN ou FR | Atendido rápido, no seu idioma, sair com horário marcado |
| **P3 — Gerente de clínica** (vertical 2) | Dentista/fisio/vet | Transbordo em pico e fora do expediente; confirmações |
| **P4 — Agência local** (canal, v2+) | Marketing local que atende trades | Revender/gerir múltiplas contas |

## 4. Requisitos funcionais (MVP)

### RF1 — Telefonia

**RF1.1 — Número e transbordo**
Como **P1**, quero manter meu número atual e ter a IA como transbordo, para que nenhuma ligação caia no voicemail sem eu precisar mudar meu telefone.

Critérios de aceite:
- Tenant recebe número local (DID canadense via Telnyx) ou configura ring group sobre o número existente.
- Modo **transbordo**: a IA só atende se ninguém atender em X segundos (X configurável por tenant).
- Modo **sempre-IA**: ativável manualmente ou por horário (fora do expediente).
- Tenant é resolvido pelo DID no início da chamada; DID desconhecido → hangup.

**RF1.2 — Transferência a quente**
Como **P1**, quero que emergências cheguem a mim imediatamente, para não perder o job que mais vale.

Critérios de aceite:
- Regras por palavra-chave/emergência (ex.: "flood", "burst pipe") disparam transferência a quente para o celular do dono, com contexto.
- Eventos `transfer.initiated`/`transfer.answered` registrados por chamada.

**RF1.3 — Gravação e transcrição com consentimento**
Como **P1**, quero gravação e transcrição de cada chamada, para auditar o que a IA falou.

Critérios de aceite:
- Aviso de gravação é **gate** antes de gravar e antes do agente entrar (PIPEDA/Lei 25), nos dois idiomas.
- Gravação + transcrição disponíveis por chamada no dashboard; PII redigida na emissão dos eventos.

### RF2 — Agente

**RF2.1 — Detecção de idioma**
Como **P2**, quero ser atendido no meu idioma, para resolver meu problema sem fricção.

Critérios de aceite:
- Detecção EN/FR no 1º turno com acurácia ≥95% (G3).
- Troca de idioma no meio da chamada sem reiniciar o nó do fluxo.

**RF2.2 — Fluxo de qualificação**
Como **P2**, quero sair da ligação com um horário marcado ou a certeza de que serei retornado.

Critérios de aceite:
- Fluxo: saudação → intenção → qualificação (serviço, código postal vs área de cobertura, urgência) → agendamento ou recado → confirmação.
- Código postal fora da área de cobertura → informa e registra recado, sem agendar.
- Agendamento via tool use com slots reais do Google Calendar (nunca slot inventado).
- Se não for possível agendar, recado estruturado obrigatório: quem, o quê, urgência, telefone de retorno.

**RF2.3 — Guardrails**
Como **P1**, quero garantia de que a IA não promete o que eu não configurei.

Critérios de aceite:
- Nunca cota preço fechado; nunca promete serviço/prazo fora da configuração do tenant.
- Emergência → transferir para o celular do dono (RF1.2).
- Barge-in funcional: cliente interrompe a fala da IA e é ouvido (mecânica da nota 06; histórico registra apenas o que foi de fato ouvido).

### RF3 — Pós-chamada

**RF3.1 — SMS ao dono**
Como **P1**, quero saber em menos de 1 minuto o que aconteceu em cada ligação que a IA atendeu.

Critérios de aceite:
- SMS ao dono em <60s do fim da chamada (G4), contendo: quem ligou, o quê, urgência, telefone e link para transcrição.

**RF3.2 — SMS ao cliente**
Como **P2**, quero confirmação por escrito do que ficou combinado.

Critérios de aceite:
- SMS de confirmação ao cliente com data/hora do agendamento (ou confirmação de retorno), no idioma da chamada.

**RF3.3 — Relatório semanal**
Como **P1**, quero enxergar o valor do serviço sem abrir dashboard.

Critérios de aceite:
- E-mail semanal com: chamadas atendidas, jobs agendados, recados, minutos usados.

### RF4 — Plataforma

**RF4.1 — Onboarding self-service**
Como **P1**, quero configurar tudo sozinho em minutos, sem call de vendas.

Critérios de aceite:
- Onboarding completo em ≤10 min: número, horário, área de cobertura, serviços.
- "Importar do meu site": LLM extrai FAQ/serviços a partir da URL do negócio.

**RF4.2 — Dashboard (web + mobile)**
Como **P1**, quero ver as chamadas e controlar o modo da IA de onde eu estiver.

Critérios de aceite:
- Lista de chamadas com áudio, transcrição e resultado (agendado/recado/transferido).
- Toggle transbordo ⇄ sempre-IA.
- **App web (React 19 + Vite) e app mobile (Expo SDK 54 + expo-router) desde o MVP** — ver [Desvios do PRD original](#11-desvios-do-prd-original). Paridade mínima no mobile: lista de chamadas, player/transcrição, toggle de modo, notificações.

**RF4.3 — Billing e multi-tenancy**
Como **P1**, quero testar antes de pagar; como operação, precisamos de isolamento por tenant.

Critérios de aceite:
- Stripe com trial de 14 dias; planos da seção [Pricing](#7-pricing).
- Multi-tenant com Postgres RLS (`app.tenant_id` por transação, `force row level security`); limites por tenant (chamadas simultâneas, rate limit).
- Dados em AWS **ca-central-1** (RNF).

## 5. Requisitos não-funcionais

| # | Requisito | Especificação |
|---|---|---|
| RNF1 | Latência de turno | Percebida **p50 < 1,2s, p95 < 2,5s**. Decomposição-alvo: VAD→fim de fala ~200ms · STT final ~150ms · LLM 1º token ~400ms · TTS 1º chunk ~200ms. Consequências: TTS streaming, cache de frases fixas, filler >600ms, barge-in |
| RNF2 | Disponibilidade | Uptime ≥ **99,9%** |
| RNF3 | Failover de produto | **Agente falhou → chamada cai no celular do dono, nunca em silêncio.** Escada de degradação: `degNormal → degFast → degScripted → degTransfer → degVoicemail`; hedge de LLM (não retry); banco de áudio pré-sintetizado para TTS; circuit breaker por provedor |
| RNF4 | Residência de dados | Toda infra e dados em AWS **ca-central-1**; providers com validação de região em runtime (`ErrResidencyViolation`) |
| RNF5 | Privacidade / compliance | PIPEDA + Lei 25 (Québec): consentimento de gravação como gate; PII redigida na emissão dos eventos (cartão/Luhn, SIN, código postal, data de nascimento, nos dois idiomas); crypto-shredding por chamada (KEK→DEK; exclusão = destruir DEK); DPA |
| RNF6 | Observabilidade | OTel por chamada; billing como fold sobre o stream de eventos (CallMinutes, STTSeconds, LLMTokens, TTSChars) |
| RNF7 | Qualidade contínua | Evals em três camadas (unit, flow sim ~300 casos/PR, voice regression noturno); `piiLeaks > 0` bloqueia publish |

## 6. Metas mensuráveis

| # | Objetivo | Meta |
|---|---|---|
| G1 | Conversa natural | Turno **p50 < 1,2s, p95 < 2,5s** |
| G2 | Resolver sem humano | ≥ **70%** das chamadas terminam com agendamento ou recado estruturado |
| G3 | Bilinguismo real | Detecção de idioma ≥ **95%** no 1º turno; FR-CA "natural" em teste cego com 10 nativos |
| G4 | Valor visível | SMS ao dono < **60s**; relatório semanal com jobs agendados |
| G5 | Confiabilidade | Uptime ≥ **99,9%**; failover: chamada cai no celular do dono, nunca em silêncio |
| G6 | Unit economics | Custo variável/min < **30%** do preço/min cobrado |

**Negócio (12 meses pós-MVP):** 10 pagantes em 90 dias · CAD 5k MRR em 12 meses (≈25–30 clientes) · churn < 5%/mês após o 3º mês.

## 7. Pricing

Hipótese a validar na Fase 0.

| Plano | Preço | Inclui |
|---|---|---|
| **Starter** | CAD 149/mês | 1 linha, 200 min, transbordo, SMS resumo |
| **Pro** | CAD 249/mês | 500 min, agendamento em calendário, FR+EN, relatório semanal |
| **Growth** | CAD 399/mês | 1.000 min, multi-calendário, integração Jobber (v1.x), prioridade |
| Minuto excedente | ~CAD 0,35 | Margem alvo ≥ 70% sobre custo variável |

Âncoras de venda: recepcionista part-time ≈ CAD 2.000+/mês; answering service CAD 300–800/mês sem agendar nada; 1 job de HVAC perdido ≈ CAD 400–1.500.

## 8. Fora de escopo do MVP

- Omnichannel completo (v3).
- Cold calling por IA (CASL — apenas retorno de missed call, v1.x).
- Asterisk/mídia própria (v2 — alavanca de margem).
- Verticais reguladas sensíveis: clínicas só agendamento, sem PHI/aconselhamento.
- Marketplace / white-label (v3).
- ~~App mobile~~ — **removido da lista de exclusões**; ver [Desvios do PRD original](#11-desvios-do-prd-original).

## 9. Riscos e mitigação

| Risco | Mitigação |
|---|---|
| Mercado quente (Retell, Vapi, Smith.ai, Goodcall, Rosie…) | Não vender "voice AI"; vender resultado numa vertical + FR-CA que os americanos tratam mal |
| STT/TTS fraco em québécois | Spike T1 decide **antes** de qualquer código de produto; se impraticável, diferencial vira "bilíngue best-effort" e o posicionamento muda (decisão consciente) |
| Custo/min come a margem | Medir no spike; cascata de modelos (Haiku/Sonnet); cache de áudio para frases fixas |
| Solo founder vendendo para trades a 8.000 km | Self-service + ads + parcerias (agências locais, contadores); pilotos como estudo de caso em vídeo |
| Incumbentes US adicionam FR (2–3 anos) | Velocidade + integrações locais + presença em Québec |
| CASL/PIPEDA/Lei 25 | Sem outbound frio; consentimento e residência ca-central-1 desde o dia 1 |

## 10. Fases

### Fase 0 — Validação de mercado (2–4 semanas)

Regra: **não escrever código de produto antes do critério de avanço.** Só é permitido o spike técnico T1.

- **V1 Entrevistas (sem. 1–2):** ICP = home services com 2–15 funcionários; clínicas como ICP secundário. 15–20 conversas; registrar citações literais, não resumos.
- **V2 Proposta de valor (sem. 2–3):** landing EN+FR com preço visível (CAD 199/mês) e CTA de early access; CAD 200–400 em Google Ads em 2 cidades (Calgary EN + Québec City FR); oferta-isca de auditoria ("ligamos 5x para seu negócio e mandamos o relatório").
- **V3 Spike técnico T1 (paralelo):** protótipo descartável Telnyx → WS → Deepgram → Claude → Cartesia/ElevenLabs; medir latência de turno p50/p95; **teste crítico de FR-CA (québécois)**. Meta: turno < 1,2s p50.

**Gate de avanço:** ≥5 negócios dizem "eu pagaria" com número na mesa (ou LOI/pilot) **E** latência do spike aceitável **E** ≥1 design partner por vertical para piloto gratuito de 60 dias.

### Fase 1 — MVP (6–10 semanas)

Escopo: *uma vertical, um fluxo completo, dois idiomas, zero dashboard sofisticado.*

| Milestone | Entrega |
|---|---|
| **M1** Telefonia (Go) | `telephony-gw`: número Telnyx, media streams WS, call control, barge-in |
| **M2** Agente (Py) | `agent-runtime` LangGraph; STT/LLM/TTS streaming; detecção EN/FR; guardrails (emergência → celular do dono) |
| **M3** Pós-chamada | SMS dono + cliente; Google Calendar; transcrição/gravação por link — **onde o valor aparece** |
| **M4** Multi-tenant + billing | `core-api`, onboarding 10 min, Stripe, dashboard mínimo (web + mobile, ver desvios) com toggle transbordo |
| **M5** Produção | OTel por chamada; PIPEDA/Lei 25 (ca-central-1, DPA); harness de evals determinístico no CI; **3–5 pilotos reais por 30 dias** |

**DoD:** 1 vertical · EN+FR · fluxo completo · billing · 3+ pilotos com uso real · p50 < 1,2s · custo por minuto conhecido (margem calculada).

### Fase 2+ — Evolução

- **v1.x (meses 4–8):** integrações Jobber/Housecall Pro/ServiceTitan (marketplace deles = canal de aquisição); retorno automático de missed calls; relatório semanal "a IA atendeu 34 ligações, agendou 9 jobs, valor estimado CAD X" — a arma anti-churn; vertical clínicas.
- **v2 (meses 8–14):** mídia própria (Asterisk/RTPengine) mantendo Telnyx só como carrier → margem sobe; SMS bidirecional; multi-location.
- **v3 (meses 12+):** omnichannel — inbox unificado voz+SMS+e-mail+WhatsApp+Instagram DM; IA como primeira camada em todos os canais; API pública + white-label para agências.

## 11. Desvios do PRD original

| # | Desvio | Original (v0.1, 2026-07-17) | Este PRD (v1.0) | Decisão |
|---|---|---|---|---|
| D1 | **App mobile no MVP** | "App mobile" listado como fora de escopo do MVP; dashboard citado como Next.js | O MVP **inclui app web E mobile**: web = React 19 + Vite + TanStack Router/Query + Tailwind; mobile = **Expo SDK 54** + expo-router + TanStack Query; monorepo pnpm com `packages/shared` (client de API + schemas zod) | Owner, 2026-08-13 |

Impacto do D1: o milestone **M4** passa a incluir o app mobile com paridade mínima (RF4.2); estimativa de esforço da Fase 1 deve ser revista considerando a superfície adicional. Nenhum outro requisito, meta ou fase foi alterado em relação às notas 15–17.
