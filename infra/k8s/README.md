# FrontDesk AI — Manifests Kubernetes

Estrutura kustomize: `base/` (workloads, statefulsets, observabilidade, asterisk)
e `overlays/{dev,prod}`.

```
base/
  namespace, serviceaccounts, configmap (config não-secreta),
  secrets (templates VAZIOS — ver base/SECRETS.md),
  telephony-gw, core-api, notifier, agent-runtime, web
  statefulsets/   postgres (dev/staging; prod = RDS), nats (JetStream + PVC)
  observability/  otel-collector, prometheus, loki, tempo, grafana
  asterisk/       placeholder v2 (replicas: 0), notas sobre RTP/hostNetwork
overlays/
  dev/   1 réplica, recursos baixos, ingress local (*.localtest.me)
  prod/  réplicas + HPA (core-api, agent-runtime), PDBs,
         topologySpreadConstraints, ingress TLS, ca-central-1
```

## Como aplicar

```sh
# dev
kubectl apply -k infra/k8s/overlays/dev

# prod
kubectl apply -k infra/k8s/overlays/prod
```

**Atenção — dashboards do Grafana**: o `configMapGenerator` em
`base/observability/kustomization.yaml` referencia arquivos FORA da raiz da
kustomization (`infra/observability/grafana/dashboards/*.json`). O kustomize
bloqueia isso por padrão (load restrictor); nesse caso use:

```sh
kustomize build --load-restrictor LoadRestrictionsNone infra/k8s/overlays/dev \
  | kubectl apply -f -
```

Alternativa sem flag: copiar/symlinkar os JSONs para dentro de
`base/observability/` e ajustar os paths.

## O que é placeholder

- **Imagens** `ghcr.io/maarkn/frontdesk/*:0.1.0` — nenhuma pipeline publica
  essas tags ainda; ajuste para o registry real.
- **Secrets** — todos com `stringData` vazio; preencher conforme
  `base/SECRETS.md` (dev: kubectl; prod: External Secrets + AWS Secrets
  Manager em ca-central-1, IRSA para AWS).
- **Dashboards** — a lista de arquivos no `configMapGenerator`
  (`latency-budget.json`, `call-outcomes.json`, `provider-health.json`) deve
  ser mantida em sincronia com `infra/observability/grafana/dashboards/`.
- **Hosts de ingress** — `*.frontdesk.example` (prod) e
  `*.frontdesk.localtest.me` (dev, resolve para 127.0.0.1). Webhook da Telnyx
  em dev exige túnel (ngrok/cloudflared).
- **Probes** — `notifier` (`:8081 /healthz`) e o `/readyz` do `telephony-gw`
  assumem endpoints que os binários ainda vão expor; `agent-runtime` usa
  probe TCP até o bridge ganhar HTTP de health.
- **Postgres em prod** — o StatefulSet fica com 0 réplicas no overlay prod;
  usar RDS (Multi-AZ, criptografado, ca-central-1) e apontar `DATABASE_URL`.
- **Observabilidade** — Prometheus/Loki/Tempo usam `emptyDir` (dev-grade);
  em prod migrar para PVC/S3 ou serviços gerenciados na região.
- **Asterisk** — não faz parte do MVP (Telnyx carrega as chamadas). O
  Deployment existe como esqueleto v2 com `replicas: 0` e notas sobre por que
  Asterisk normalmente vive FORA do cluster (range UDP de RTP 10000–20000,
  NAT/SDP, latência) ou com `hostNetwork`/rede dedicada em nós próprios.

## Notas operacionais

- **telephony-gw é stateful por chamada**: shutdown drena (readiness fica
  not-ready, chamadas ativas terminam dentro de `DRAIN_TIMEOUT`,
  `terminationGracePeriodSeconds: 1800`). Por isso **não há HPA** para ele em
  prod — escala manual/lenta; o PDB limita evictions a 1 pod por vez.
- **HPA** só nas camadas stateless: core-api (3–10) e agent-runtime (3–12).
- **Região**: tudo em `ca-central-1` (PIPEDA/Lei 25); exemplo de
  `nodeSelector` por região comentado no overlay prod.
