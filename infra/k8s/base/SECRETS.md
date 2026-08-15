# Como preencher os Secrets

Os arquivos `secrets.yaml` deste diretório são **templates** com `stringData` vazio.
Nunca comite valores reais.

## Opção 1 — dev local (kubectl direto)

```sh
kubectl -n frontdesk create secret generic frontdesk-secrets \
  --from-literal=TELNYX_API_KEY='KEY...' \
  --from-literal=TELNYX_FROM='+15145550100' \
  --from-literal=DEEPGRAM_API_KEY='...' \
  --from-literal=ANTHROPIC_API_KEY='sk-ant-...' \
  --from-literal=CARTESIA_API_KEY='...' \
  --from-literal=ELEVENLABS_API_KEY='...' \
  --from-literal=DATABASE_URL='postgres://frontdesk:pwd@postgres:5432/frontdesk?sslmode=disable' \
  --from-literal=STRIPE_SECRET_KEY='sk_test_...' \
  --from-literal=STRIPE_WEBHOOK_SECRET='whsec_...' \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n frontdesk create secret generic postgres-credentials \
  --from-literal=POSTGRES_USER=frontdesk \
  --from-literal=POSTGRES_PASSWORD='...' \
  --from-literal=POSTGRES_DB=frontdesk \
  --dry-run=client -o yaml | kubectl apply -f -
```

## Opção 2 — prod (recomendado)

Use **External Secrets Operator** apontando para o AWS Secrets Manager em
`ca-central-1` (residência de dados), ou **Sealed Secrets** se o cluster não
tiver acesso IAM. Em prod, deixe `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`
vazios e use **IRSA** (IAM Roles for Service Accounts) nos ServiceAccounts
`notifier` e `core-api`.

## Observações

- `DATABASE_URL` em prod aponta para o RDS (ver `statefulsets/postgres.yaml`).
- Rotação: os Deployments montam os secrets como env; um `kubectl rollout restart`
  é necessário após atualizar valores.
