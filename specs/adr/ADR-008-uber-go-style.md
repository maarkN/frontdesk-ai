# ADR-008 — Adoção integral do uber-go/guide para o código Go

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** CONTEXT.md (§ Convenções de código), [uber-go/guide](https://github.com/uber-go/guide), notas `06 - Máquina de Estados do Turno`, `07 - Resiliência e Degradação`, `08 - Multi-tenancy`
- **Depende de:** ADR-002

## Contexto

Os serviços Go (telephony-gw, core-api, notifier) concentram exatamente o tipo de código onde
Go dá mais corda para se enforcar: goroutines de longa duração por chamada, streams de áudio,
cancelamento em cascata, integração com três provedores externos. Precisamos de um guia de
estilo que seja opinativo sobre **concorrência e ciclo de vida**, não só sobre formatação —
e que possa ser aplicado por lint, não por review heroico.

## Decisão

Adotar o **uber-go/guide integralmente** como guia de estilo dos serviços Go, com enforcement
via `golangci-lint` (+ analisadores customizados onde o guia não tem lint pronto). Divergências
pontuais, se surgirem, exigem registro em ADR próprio.

Abaixo, as regras de maior impacto neste projeto e como se aplicam ao domínio de voz.

### 1. Toda goroutine tem dono e ciclo de vida — 1 goroutine por chamada

Regra do guia: nunca "fire and forget"; quem inicia uma goroutine sabe quando e como ela
termina. No nosso domínio isso é a arquitetura em si: **a máquina de estados do turno é 1
goroutine por chamada** (loop `select` sobre `stListening → stEndpointing → stThinking →
stSpeaking`), dona de todas as goroutines auxiliares da chamada (leitor de STT, escritor de
TTS, timer de hedge). O término da chamada cancela o `context.Context` raiz e o dono espera
(`sync.WaitGroup`) todos os filhos. Produtores seguem o padrão que impede vazamento:

```go
select {
case ch <- tok:
case <-ctx.Done():
    return
}
```

Vazamento de goroutine aqui não é abstrato: 200 chamadas/dia × 1 goroutine vazada com stream
de provedor aberto = conexões e dinheiro vazando.

### 2. Channels com tamanho 0 ou 1

Regra do guia: buffer de canal deve ser 0 ou 1; qualquer outro número exige justificativa por
escrito. Em pipeline de áudio, buffer grande é **latência escondida**: um canal de frames com
buffer 64 significa até 64 × 20 ms de áudio represado que o cliente ainda vai ouvir depois de
um barge-in. Buffer 0/1 força o backpressure a aparecer no design (descartar frame, duckar,
cancelar) em vez de se acumular em silêncio ou fala atrasada. A exceção deliberada — o ring
buffer de replay do STT — não é um channel, é uma estrutura própria com semântica de descarte
explícita (ADR-006).

### 3. Erros com `%w`, `errors.Is/As`

Wrapping com `%w` preserva a cadeia; a escada de degradação decide o próximo degrau com
`errors.Is(err, ErrProviderTimeout)` / `errors.As` sobre erros tipados dos providers
(`ErrAllProvidersFailed`, `ErrResidencyViolation`). Sem cadeia de erro confiável, a decisão
"degFast ou degScripted?" viraria matching de string. Mensagens sem "failed to" redundante,
sentinelas exportadas só quando o consumidor precisa decidir com base nelas.

### 4. Interfaces pequenas, definidas no consumidor

Regra do guia: accept interfaces, return structs; a interface mora no pacote que a **consome**.
O loop do turno declara o que precisa (`Transcriber`, `Synthesizer`, `Completer` — 1 a 3
métodos), e os pacotes de provedor (deepgram, cartesia, anthropic) retornam structs concretas
que as satisfazem. É o que torna baratos: fallback de fornecedor (ADR-006), factory por tenant
com BYOK e validação de região (ADR-005), e fakes determinísticos nos evals (flow sim com
passthrough de STT/TTS). Compliance verificada em compile time:
`var _ turn.Transcriber = (*deepgram.Client)(nil)`.

### 5. Zero-value útil

Structs projetadas para funcionar sem construtor quando possível (`var b breaker.Breaker` já é
um breaker fechado; mutex por valor, nunca embutido em struct pública). Em código de recuperação
— rehidratar chamadas após restart, worker processando evento antigo — objetos parcialmente
inicializados aparecem; zero-value útil transforma "esqueci de inicializar" de pânico em
comportamento neutro.

### 6. Functional options para configuração

Construtores dos componentes ajustáveis usam functional options:
`turn.New(deps, turn.WithHedgeAfter(700*time.Millisecond), turn.WithEndpointing(lang.FRCA))`.
O domínio tem dezenas de knobs (janelas de endpointing por idioma, hedgeAfter, limites por
tenant, timeouts de reconexão) e eles crescem a cada aprendizado de produção — options permitem
adicionar knob sem quebrar call sites, com defaults seguros e sem structs de config gigantes
meio-preenchidas.

### 7. Sem `init()`

Nada de estado global montado em `init()` (registries implícitos, clients singleton, leitura de
env). Toda inicialização é explícita em `main()`/construtores, com dependências passadas por
parâmetro. Consequências práticas: os serviços têm ordem de boot determinística (importante no
reinício com chamadas ativas — primeiro reconectar ao carrier, depois rehidratar snapshots),
e os testes montam o mundo que precisam sem des-fazer efeitos de import.

### Demais regras do guia

Aplicam-se integralmente sem adaptação de domínio: guard clauses/early return, nomes de pacote
curtos sem `util`, não capturar variáveis de loop indevidamente, `time.Time`/`time.Duration` em
vez de ints, evitar `panic` em código de servidor, etc. Layout:
`go/cmd/{core-api,telephony-gw,notifier}` + `go/internal/...`, Go 1.22+.

## Consequências

- (+) Review discute domínio, não estilo; o lint resolve o resto.
- (+) As regras de concorrência do guia coincidem com os invariantes críticos do produto
  (sem goroutine órfã, sem latência escondida em buffer, cancelamento em cascata).
- (+) Interfaces no consumidor mantêm baixo o custo de trocar/hedgear provedores — que é rotina
  neste produto, não exceção.
- (−) Curva para quem vem de estilos mais permissivos (channels bufferizados "por via das
  dúvidas", `init()` para registrar drivers).
- (−) Functional options adicionam boilerplate por componente; aceito pelo ganho de evolução
  de API.
- (−) Algumas regras não têm lint pronto (dono de goroutine, justificativa de buffer) — cobertas
  por convenção de review + `goleak` nos testes de integração.

## Alternativas consideradas

- **Só Effective Go + Code Review Comments** — rejeitado: bons princípios, mas pouco opinativos
  em concorrência e ciclo de vida, que é onde este projeto mais sangra.
- **Google Go Style Guide** — rejeitado: forte em legibilidade, menos prescritivo em goroutines,
  channels e options; o uber-go/guide encaixa melhor no perfil do código (serviços concorrentes
  de longa duração).
- **Guia próprio da casa** — rejeitado: custo de escrever e manter, sem o battle-testing e a
  familiaridade que um guia público dá a quem entra no projeto.
- **Adoção parcial (cherry-picking)** — rejeitado: "quais regras valem" vira discussão por PR;
  adoção integral com exceções via ADR inverte o ônus corretamente.
