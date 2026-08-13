# ADR-005 — Multi-tenancy: resolução por DID, tenant no contexto e Postgres RLS

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** nota `08 - Multi-tenancy`, CONTEXT.md (§ Decisões-chave, § Conceitos transversais item 5)
- **Depende de:** ADR-001, ADR-003

## Contexto

O FrontDesk AI é um SaaS multi-tenant: várias PMEs compartilham a mesma infraestrutura
(telephony-gw, agent-runtime, core-api, Postgres). Um vazamento entre tenants aqui não é um bug
de UI — é áudio e transcrição de um cliente aparecendo para outro, com implicações diretas de
PIPEDA/Lei 25. Três problemas precisam de decisão explícita:

1. **Onde o tenant é resolvido e como ele viaja pelo código.** Numa chamada telefônica não há
   sessão de usuário: a única identidade no início é o DID discado.
2. **Como o isolamento é garantido no banco** com pool de conexões (pgx/pgbouncer), onde uma
   conexão física é reutilizada por transações de tenants diferentes.
3. **Como honrar pedidos de exclusão de dados** (Lei 25) quando áudio e transcrição já foram
   replicados para backups e réplicas que não podem ser reescritos.

Há também o problema de vizinho barulhento: um tenant com pico de chamadas não pode degradar a
latência (orçamento p50 < 1,2s) dos demais.

## Decisão

### 1. Tenant resolvido uma única vez, pelo DID, no início da chamada

No evento de início de chamada no telephony-gw, o DID discado resolve o tenant. DID desconhecido
→ **hangup imediato** — nunca "chutar" um tenant ou aceitar a chamada sem dono. A partir desse
ponto o `tenantID` entra no `context.Context` e desce por toda a cadeia (call control → eventos →
agent-runtime → core-api).

**Regra dura, com lint:** nenhuma função abaixo do ponto de resolução aceita `tenantID` como
parâmetro — ela extrai do contexto. Um analisador customizado (via `go vet`/`golangci-lint`)
proíbe `tenantID string` (e variações) em assinaturas de função fora do pacote de resolução.
A regra elimina a classe inteira de bug "passei o tenant errado adiante": só existe um lugar onde
o tenant é atribuído.

No agent-runtime (Python), o equivalente é o `tenant_id` carregado no estado do LangGraph a
partir do envelope do evento — nunca como parâmetro solto de função de negócio.

### 2. Postgres RLS com `set LOCAL` + `force row level security`

- Toda transação que toca dados de tenant executa
  `set LOCAL app.tenant_id = '<uuid>'` **dentro da transação**. `LOCAL` é obrigatório: o escopo
  é a transação, não a conexão — sem isso o pool vaza tenant entre transações.
- Tabelas de tenant recebem `enable row level security` **e** `force row level security` — o
  `force` garante que nem o role dono da tabela escapa da policy (migrações e jobs internos
  rodam sob as mesmas regras).
- Policy padrão: `using (tenant_id = current_setting('app.tenant_id')::uuid)`.
- **Teste de integração obrigatório em CI:** duas transações com tenants diferentes na mesma
  conexão física do pool, afirmando que nenhuma enxerga dados da outra. Esse teste é o guarda
  contra regressões de configuração de pool.

RLS é a defesa em profundidade: mesmo que uma query esqueça o `where tenant_id`, o banco não
retorna linhas de outro tenant.

### 3. Limites por tenant (vizinho barulhento)

- **Semáforo de chamadas concorrentes por tenant** no telephony-gw. `Acquire` com timeout de
  200 ms; estourou → responder ocupado (`486 Busy Here`). Rejeitar em 200 ms é melhor que aceitar
  a chamada e entregar silêncio.
- **Rate limit por tenant** de tokens de LLM e segundos de STT (Redis), consultado pelo
  agent-runtime antes de cada turno caro.

### 4. Crypto-shredding: KEK (por tenant) → DEK (por chamada)

```
KEK (KMS, por tenant) ──encripta──▶ DEK (por chamada) ──encripta──▶ áudio + transcrição
```

- Cada chamada gera uma DEK; áudio e transcrição são cifrados com ela antes de ir ao S3/Postgres.
- A DEK é armazenada cifrada pela KEK do tenant (AWS KMS, ca-central-1).
- **Pedido de exclusão = destruir a DEK.** O ciphertext continua existindo em backups e réplicas
  e é matematicamente inútil. É a única resposta operacionalmente honesta a um pedido de exclusão
  sob a Lei 25 — e precisa estar desenhada **antes da primeira gravação existir**, porque não é
  retrofitável.
- Encerramento de conta = agendar destruição da KEK (torna todas as DEKs do tenant irrecuperáveis).

## Consequências

- (+) Um único ponto de atribuição de tenant; o lint torna impossível reintroduzir `tenantID`
  como parâmetro "por conveniência".
- (+) RLS + teste de pool em CI dá isolamento verificável, não só convencionado.
- (+) Billing sai de graça: todo evento carrega `tenantId`, usage é um fold sobre o stream.
- (+) Crypto-shredding resolve exclusão sem reescrever backups.
- (−) `set LOCAL` por transação adiciona um round-trip; mitigado com pipelining do pgx.
- (−) O lint customizado precisa de manutenção (novos padrões de assinatura).
- (−) Crypto-shredding adiciona uma camada de KMS no caminho de gravação; a DEK é gerada e
  cacheada por chamada para não tocar o KMS por chunk de áudio.
- (−) Perda acidental de DEK = perda irreversível do dado; exige cuidado no ciclo de vida das
  chaves (a destruição é operação explícita e auditada, nunca efeito colateral).

## Alternativas consideradas

- **Isolamento só na aplicação (`where tenant_id = ?`)** — rejeitado como única defesa: um
  esquecimento vaza dados. RLS transforma o erro em resultado vazio.
- **Schema por tenant / banco por tenant no MVP** — rejeitado: custo operacional de migração ×N
  e pool fragmentado. Fica registrado como tier futuro (Shared/Isolated/Sovereign da nota 08):
  o MVP é Shared (RLS); Isolated (schema dedicado) e Sovereign (cluster dedicado) são planos
  vendáveis quando houver demanda (setor público, Québec).
- **`set session` em vez de `set LOCAL`** — rejeitado: escopo de conexão vaza tenant com pool.
- **Deleção física para pedidos de exclusão** — rejeitado: impraticável em backups imutáveis e
  réplicas; crypto-shredding é o mecanismo honesto.
- **Tenant como parâmetro explícito em todas as funções** — rejeitado: multiplica pontos onde o
  valor errado pode ser passado; o contexto com lint dá o mesmo rastro com um único ponto de
  verdade.
