# ADR-001 — Implementar a frente Receptionist como produto

- **Status:** Aceito (2026-08-13)
- **Decisores:** Marco Filho (owner)
- **Fontes:** notas 01, 15, 16, 17 do vault `maarkN/Voice AI/`; CONTEXT.md

## Contexto

O vault documenta três frentes distintas do domínio Voice AI (nota 01):

| Frente | Natureza | Mercado | Stack | Status |
|---|---|---|---|---|
| Arquitetura de referência (notas 02–09) | Blueprint técnico completo | Canadá (en-CA + fr-CA) | Go + TypeScript sobre Asterisk | Referência escrita |
| Voz no CRM imobiliário (notas 10–14) | Adaptação a um produto existente | Brasil (pt-BR) | Go-only (CRM já é Go) | Ideação sobre base real |
| AI Voice Receptionist (notas 15–17) | Produto SaaS do zero | Canadá (EN + FR-CA) | Go (telefonia) + Python (agente) | Ideação (PRD v0.1, 2026-07-17) |

A referência é o documento-mãe: define o vocabulário e resolve os problemas difíceis do
domínio — máquina de estados do turno, resiliência/degradação, multi-tenancy, evals. As outras
duas frentes herdam dela. É preciso escolher **uma** frente para implementar.

O pedido do owner para esta implementação fixa a stack em **Go + Python** e o escopo como
produto completo (incluindo, como delta sobre o PRD original, app web e mobile em TypeScript).

## Decisão

Implementar o **AI Voice Receptionist ("FrontDesk AI", notas 15–17)** como produto, **incorporando
os padrões da arquitetura de referência** (notas 02–09) onde o Receptionist não os redefine:
orçamento de latência por turno, `state = fold(events)`, máquina de estados do turno com
barge-in, escada de degradação, multi-tenancy com RLS e crypto-shredding, consent como gate,
provider atrás de interface, evals em três camadas.

As frentes CRM e referência permanecem como material de consulta: a referência como fonte dos
padrões, o CRM como contraexemplo de decisões (ver ADR-002 e ADR-004).

## Justificativa

1. **Casa com a stack pedida.** Das três frentes, só o Receptionist já é especificado como
   Go + Python (nota 16). A referência é Go + TypeScript sobre Asterisk; o CRM é Go-only por
   restrição de base existente que não existe aqui.
2. **É a única frente implementável como produto do zero.** A referência é um blueprint, não um
   produto — não tem mercado, pricing nem requisitos funcionais próprios. O CRM pressupõe um
   codebase Go de CRM imobiliário em produção, que não faz parte deste workspace.
3. **É a frente com definição de produto completa.** As notas 15–17 trazem dor, personas,
   pricing (Starter/Pro/Growth em CAD), requisitos funcionais RF1–RF4, metas mensuráveis G1–G6,
   roadmap por fases (M1–M5) e riscos mapeados. Nenhuma outra frente tem esse nível de
   especificação de produto.
4. **Diferencial defensável definido.** Bilíngue EN/FR-CA de verdade (Lei 96 pressiona Québec;
   players US tratam francês como segunda classe) + foco vertical em trades com integrações
   locais (Jobber é canadense).
5. **Reúso máximo do domínio.** O Receptionist é independente em código, mas reaproveita todos
   os conceitos transversais da referência (nota 01) — o investimento nas notas 02–09 não é
   descartado, é incorporado.

## Consequências

**Positivas**
- Escopo de produto claro desde o dia 1: RF1–RF4, metas G1–G6, DoD do MVP (1 vertical, EN+FR,
  fluxo completo, billing, 3+ pilotos, p50 < 1,2 s, margem calculada).
- Os padrões difíceis (turno, degradação, eventos, tenancy) chegam prontos da referência, em vez
  de serem redescobertos.
- Habilita as decisões subsequentes: split Go/Python (ADR-002), eventos como fonte da verdade
  (ADR-003), Telnyx sem Asterisk (ADR-004).

**Negativas / riscos assumidos**
- Mercado quente (Retell, Vapi, Smith.ai, Goodcall, Rosie). Mitigação da nota 17: vender
  resultado numa vertical + FR-CA, não "voice AI" genérico.
- O diferencial FR-CA depende de STT/TTS em québécois — risco técnico deriscado pelo spike T1
  antes do código de produto (nota 17). Se impraticável, o posicionamento muda conscientemente.
- Incluir web **e** mobile (delta pedido pelo owner) amplia o escopo do MVP original, que
  excluía app mobile; o custo é assumido explicitamente.
- Adaptar padrões escritos para Go+TS/Asterisk a um contexto Go+Python/Telnyx exige tradução
  cuidadosa (ex.: `foldCallState` em `packages/contracts` vira contrato JSON versionado em
  `contracts/` — ver ADR-003).

## Alternativas consideradas

1. **Implementar a arquitetura de referência (notas 02–09).** Rejeitada: é blueprint, não
   produto — sem mercado, pricing ou RFs; a stack (Go + TS sobre Asterisk) não é a pedida; e
   nascer sobre Asterisk próprio contraria a estratégia de carrier do MVP (ver ADR-004).
2. **Implementar a voz no CRM imobiliário (notas 10–14).** Rejeitada: pressupõe CRM Go
   existente ("adaptação da referência a um produto existente"); a decisão central da frente é
   Go-only, incompatível com o pedido Go + Python; e as notas documentam apenas *o que muda* em
   relação à referência, não um produto completo.
3. **Implementar o Receptionist ignorando a referência (só notas 15–17).** Rejeitada: as notas
   do Receptionist remetem explicitamente à referência para a mecânica difícil (barge-in na
   nota 06, degradação na 07, tenancy na 08, evals na 09). Reimplementar sem esses padrões
   seria redescobrir o que já está resolvido.
