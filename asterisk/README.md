# Asterisk — backend de mídia alternativo (ADR-009)

Stack de laboratório/staging do backend Asterisk do `telephony-gw`. **Telnyx
continua sendo o default do MVP** (`MEDIA_BACKEND=telnyx`); este diretório
existe para validar a abstração de call-control/mídia e preparar a alavanca
de margem da v2. O lado Go vive em `go/internal/asterisk`.

## Arquivos

| Arquivo | Papel |
|---|---|
| `Dockerfile` | Asterisk 22 LTS via pacote do Debian 13 "trixie" (escolha documentada no próprio arquivo: build de fonte foi rejeitado para o lab — o pacote da distro instala em ~1min e recebe patches via apt). Asserta a presença de `res_config_pgsql.so` |
| `docker-entrypoint.sh` | Gera `ari_secret.conf` (env `ARI_*`) e `res_pgsql.conf` (env `PGSQL_*`) — configs do Asterisk não expandem env vars |
| `pjsip.conf` | SÓ transportes (udp/tcp/wss). Troncos e ramais vivem no **PostgreSQL** (realtime, ver abaixo). **chan_pjsip apenas** — chan_sip foi removido do Asterisk 21+ |
| `extconfig.conf` | Mapeia famílias `ps_*` → driver `pgsql` (ARA) |
| `sorcery.conf` | Objetos PJSIP (endpoint/auth/aor/contact/identify/registration) → realtime |
| `modules.conf` | `preload = res_config_pgsql.so` (driver de pé antes do res_pjsip) |
| `extensions.conf` | Entrada → `Stasis(${GLOBAL(ACTIVE_APP)})` (blue/green); contexto `transfer-owner` para o failover ao celular do dono |
| `ari.conf` | ARI habilitado; usuário/senha via include gerado pelo entrypoint |
| `http.conf` | Servidor HTTP 8088: REST+WebSocket do ARI e transporte wss |
| `rtp.conf` | Range RTP 10000–10200 (casa com o `EXPOSE` do Dockerfile) |

## SIP realtime no PostgreSQL (troncos e ramais sem editar .conf)

Endpoints/auths/aors/identifies/registrations vivem nas tabelas `ps_*` do
Postgres do stack (schema + seeds em
`go/internal/store/migrations/0002_asterisk_realtime.sql`; o esquema segue o
alembic oficial do Asterisk, reduzido). Regras de propagação:

- **Ramais e troncos IP-auth**: `INSERT`/`UPDATE` vale na próxima
  chamada/registro — **sem reload** (sorcery sem cache de propósito).
- **`ps_registrations`** (registro outbound, ex.: Callcentric): carregado no
  boot — após mudar, `docker compose exec asterisk asterisk -rx "pjsip reload"`.
- Contatos de REGISTER persistem em `ps_contacts` (sobrevivem a restart).

### Adicionar um ramal (ex.: 6002)

```sql
insert into ps_auths (id, auth_type, username, password)
  values ('6002-auth','userpass','6002','uma-senha');
insert into ps_aors (id, max_contacts, remove_existing)
  values ('6002', 1, 'yes');
insert into ps_endpoints (id, transport, context, disallow, allow, auth, aors,
                          dtmf_mode, direct_media)
  values ('6002','transport-udp','from-test','all','slin16,ulaw',
          '6002-auth','6002','rfc4733','no');
```

### Tronco Callcentric (registro outbound + entrada autenticada)

```sql
update ps_auths set username = '17778696496', password = 'SENHA_SIP_DA_EXTENSAO'
  where id = 'trunk-primary-auth';
update ps_aors set contact = 'sip:callcentric.com' where id = 'trunk-primary';
update ps_endpoints set from_user = '17778696496', from_domain = 'callcentric.com'
  where id = 'trunk-primary';
update ps_endpoint_id_ips set match = 'callcentric.com'
  where id = 'trunk-primary-identify';
insert into ps_registrations (id, transport, outbound_auth, server_uri,
                              client_uri, retry_interval)
  values ('trunk-primary-reg','transport-udp','trunk-primary-auth',
          'sip:callcentric.com','sip:17778696496@callcentric.com',60);
-- registrations só entram no reload:
--   docker compose exec asterisk asterisk -rx "pjsip reload"
```

Diagnóstico: `pjsip show endpoint trunk-primary` (realtime é on-demand — o
`pjsip show endpoints` sem argumento não lista objetos que nunca foram usados),
`pjsip show registrations`, `realtime load ps_endpoints id trunk-primary`.

> Volume do Postgres já inicializado não roda a migration 0002 de novo —
> aplicar manualmente:
> `docker compose exec -T postgres psql -U frontdesk -d frontdesk < go/internal/store/migrations/0002_asterisk_realtime.sql`

## Como a mídia flui (External Media do ARI)

Não usamos AudioSocket: o **External Media** do ARI cobre o mesmo caso com
RTP puro e formato negociado — menos partes móveis no lab.

```
PSTN ── ITSP trunk ──▶ canal PJSIP ──▶ Stasis(frontdesk-v1)   [call control: ARI]
                                          │
                telephony-gw cria um canal externalMedia via ARI:
                POST /channels/externalMedia
                     external_host = <ip-gw>:<porta-udp>
                     format = slin16   encapsulation = rtp   transport = udp
                                          │
        canal PJSIP ◀── bridge ──▶ canal externalMedia
                                          │  RTP/UDP, PCM slin16
                                          │  (16 kHz, 20 ms = 320 amostras/pacote)
                                          ▼
                telephony-gw: ExternalMediaServer (go/internal/asterisk)
                  ├─ inbound:  RTP → decimação 16k→8k → media.Frame de 20 ms
                  │            (mesmo contrato do backend Telnyx)
                  └─ outbound: TTS 8k → interpolação 8k→16k → RTP slin16
                               de volta ao endereço de origem do canal
```

- **Formato no fio**: `slin16` = PCM linear 16-bit, 16 kHz, mono, big-endian
  no payload RTP (convenção L16). Um pacote a cada 20 ms → 640 bytes de
  payload + 12 de cabeçalho RTP.
- **Por que 16 kHz no fio e 8 kHz no gateway**: o contrato interno do
  `telephony-gw` (`internal/media`) é a perna PSTN a 8 kHz; slin16 evita
  transcode dentro do Asterisk (trunk `allow = slin16`) e dá headroom para o
  STT. A conversão (decimação/interpolação) acontece no `ExternalMediaServer`.
- **DTMF** chega por evento ARI (`ChannelDtmfReceived`, rfc4733 no trunk) —
  independente do pipeline de áudio, como exige a nota 07.

## Blue/green e recuperação (nota 07)

- Deploy novo = novo nome de app Stasis (`frontdesk-v2`) + apontar a global
  `ACTIVE_APP`. Chamadas novas entram no app novo; o processo antigo fica
  not-ready e vivo até esvaziar (`Drainer`, teto `maxDrainSec`).
- No boot o gateway **rehidrata do Asterisk** (a fonte da verdade do canal é
  o Asterisk): lista canais vivos do app, canal com snapshot retoma com frase
  de reancoragem do audiobank; canal sem snapshot → transfer
  `recovered_orphan` para o celular do dono. Ver
  `go/internal/asterisk/rehydrate.go`.

## Subir o lab

O jeito suportado é via compose (o realtime precisa do Postgres do stack):

```sh
make up PROFILES="--profile asterisk"
# ou: docker compose --profile asterisk up -d asterisk
```

Standalone (exige um Postgres alcançável com as tabelas ps_*):

```sh
docker build -t frontdesk-asterisk asterisk/
docker run --rm -e ARI_PASSWORD=changeme \
  -e PGSQL_HOST=host.docker.internal -e PGSQL_PASSWORD=frontdesk \
  -p 5060:5060/udp -p 8088:8088 -p 10000-10200:10000-10200/udp \
  frontdesk-asterisk
```

Depois: `MEDIA_BACKEND=asterisk ARI_URL=http://localhost:8088/ari ...` no
`telephony-gw` (integração no main acontece na fase de verificação).
