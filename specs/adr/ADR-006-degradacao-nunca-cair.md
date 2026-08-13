# ADR-006 — Degradação controlada: a chamada nunca cai em silêncio

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** nota `07 - Resiliência e Degradação`, nota `16 - Receptionist (Arquitetura e Requisitos)` (meta G5), CONTEXT.md (§ Conceitos transversais item 4)
- **Depende de:** ADR-001, ADR-004

## Contexto

O produto atende a linha telefônica de uma PME. Uma chamada perdida é um job perdido — o pitch
inteiro do produto. A meta G5 é explícita: uptime ≥ 99,9% **e** "agente falhou → chamada cai no
celular do dono, nunca em silêncio".

O ambiente é hostil a padrões convencionais de resiliência: dependemos de três provedores
externos de streaming (STT, LLM, TTS), qualquer um pode falhar no meio de um turno, e o
orçamento de reação **é medido em milissegundos de silêncio, não em número de tentativas**.
Acima de ~1s de silêncio o cliente acha que a ligação caiu. Retry com backoff exponencial —
o default de qualquer biblioteca de resiliência — é catastrófico aqui: enquanto o backoff
espera, o cliente ouve silêncio e desliga.

Janelas de reação por tipo de falha (nota 07):

| Falha | Janela | Resposta |
|---|---|---|
| STT desconecta | ~200 ms | Reconectar + replay do ring buffer |
| STT sem resultado | 1,5 s | Reprompt bilíngue |
| LLM 429 / timeout | ~1,2 s | Modelo menor → enlatado → humano |
| TTS falha | 0 ms | Banco de áudio pré-sintetizado |

## Decisão

### 1. Escada de degradação `degNormal → degVoicemail`

```
degNormal → degFast → degScripted → degTransfer → degVoicemail
```

- **degNormal** — pipeline completo (STT streaming, LLM em cascata, TTS streaming).
- **degFast** — modelo menor, prompt reduzido; sacrifica qualidade de raciocínio por latência.
- **degScripted** — respostas enlatadas do banco de áudio + navegação por DTMF. Todo nó `agent`
  do fluxo é obrigado a declarar `fallbackDTMF` (gate no compilador do grafo).
- **degTransfer** — transferência a quente para humano, com contexto do que já foi coletado.
- **degVoicemail** — último degrau funcional: gravar recado estruturado.

Cada nível é **funcional, só que pior**. A decisão de nível é **por chamada e temporária**,
informada pela saúde global do provedor (circuit breaker) — uma chamada degradada não arrasta
as demais, e a recuperação do provedor devolve as chamadas novas ao `degNormal`.

### 2. LLM: hedge, não retry

Ao chamar o LLM primário, um timer dispara o provedor secundário após `hedgeAfter` (~700 ms ≈
p90 do primário); usa-se **quem responder primeiro** e cancela-se o outro (via
`context.Context`). Custo: duplica a chamada em ~10% dos turnos — exatamente os turnos que
arruinariam a conversa. Retry sequencial é proibido no caminho da chamada: esperar o primário
falhar para então tentar o secundário soma as latências em vez de sobrepô-las.
`ErrAllProvidersFailed` nunca chega ao cliente — vira frase enlatada + `degTransfer`.

STT segue regra análoga adaptada a streaming: ring buffer que recebe frames **sempre** (mesmo
desconectado), reconexão em 3 tentativas rápidas (0/80/200 ms, sem backoff exponencial), replay
do buffer, fallback de fornecedor; se tudo falhar, `degScripted` — a chamada vive. DTMF chega
pelo caminho de sinalização, independente do pipeline de áudio.

### 3. Circuit breaker por provedor, nunca por chamada

O estado do breaker (fechado/aberto/meio-aberto) é **global por provedor**. Com 200 chamadas
ativas, cada goroutine reconectando por conta própria faz 600 tentativas em 200 ms e transforma
uma degradação parcial em queda total (retry storm). Meio-aberto: **1 sonda a cada 5 s**, para o
provedor inteiro. As chamadas consultam o breaker antes de tentar; breaker aberto → pula direto
para o fallback do seu nível de degradação.

### 4. Banco de áudio pré-sintetizado como infraestrutura crítica

~15 frases por locale (EN, FR-CA): greeting, one_moment, didnt_catch, transferring,
technical_issue, goodbye, etc. Geradas **no build**, versionadas junto com a persona (mudou a
voz da persona → regera o banco), servidas do disco local do telephony-gw (TTS falhou → resposta
em 0 ms). O banco cobre `degScripted` e `degTransfer` inteiros e serve de filler (>600 ms de
processamento → "um momento…") no loop do turno. É tratado como infraestrutura crítica: sua
ausência ou dessincronização com a persona é erro de build, não warning.

### 5. Failover de produto: o celular do dono

Se o agente falhou de forma irrecuperável (todos os degraus técnicos esgotados, ou telephony-gw
sem resposta), a chamada é **transferida para o celular do dono** configurado no onboarding.
Isso inclui o nível de plataforma: a regra de failover é configurada no carrier (Telnyx) para
que mesmo a indisponibilidade total do nosso backend resulte em ring no celular do dono, nunca
em silêncio ou tom de ocupado. É a versão-produto da escada de degradação e é critério de
aceite da meta G5.

### Métricas de saúde

- **`silence_gap_ms` p99** — a métrica-mestra; sobe antes de qualquer erro aparecer.
- `degradation_level` (distribuição; `degScripted` > 1% das chamadas é sinal ruim).
- `provider_breaker_state`, `barge_in_false_positive_rate`, `recovery_orphan_rate`.
- Alerta: `degradation_level > degNormal` por mais de 60 s em mais de 5% das chamadas ativas.

## Consequências

- (+) Nenhum modo de falha termina em silêncio; o pior caso audível é "vou transferir você".
- (+) Hedge compra p99 de LLM ao custo de ~10% de chamadas duplicadas — custo previsível e
  contabilizado no G6 (custo variável/min).
- (+) Breaker global impede retry storm; degradação parcial não vira queda total.
- (−) Cinco níveis de degradação × transições exigem teste dedicado (evals de flow sim com
  falha injetada de provedor).
- (−) Banco de áudio acopla build de release à persona (regeneração de TTS no CI).
- (−) Hedge exige contrato de idempotência nos providers de LLM e cancelamento limpo do perdedor
  (sem vazar goroutine/stream).
- (−) Failover para o celular do dono depende de configuração correta no onboarding — telefone
  do dono validado (test call) antes de ativar o número.

## Alternativas consideradas

- **Retry com backoff exponencial** — rejeitado explicitamente: otimiza para taxa de sucesso
  eventual, não para milissegundos de silêncio; cada espera é silêncio audível.
- **Retry sequencial de LLM (primário falhou → secundário)** — rejeitado: soma latências;
  o hedge sobrepõe.
- **Circuit breaker por chamada** — rejeitado: sem visão global, N chamadas geram N sondas e
  amplificam a falha do provedor.
- **TTS de fallback online (segundo provedor) em vez de banco de áudio** — rejeitado como única
  defesa: o segundo provedor também pode estar fora; o banco local responde em 0 ms e não tem
  dependência de rede. O fallback de provedor TTS existe, mas o banco é o piso.
- **"Fail fast e desligar" quando tudo falha** — rejeitado: viola G5; o piso do produto é o
  celular do dono, não o hangup.
