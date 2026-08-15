# Asterisk — backend de mídia alternativo (ADR-009)

Stack de laboratório/staging do backend Asterisk do `telephony-gw`. **Telnyx
continua sendo o default do MVP** (`MEDIA_BACKEND=telnyx`); este diretório
existe para validar a abstração de call-control/mídia e preparar a alavanca
de margem da v2. O lado Go vive em `go/internal/asterisk`.

## Arquivos

| Arquivo | Papel |
|---|---|
| `Dockerfile` | Asterisk 22 LTS via pacote do Debian 13 "trixie" (escolha documentada no próprio arquivo: build de fonte foi rejeitado para o lab — o pacote da distro instala em ~1min e recebe patches via apt) |
| `docker-entrypoint.sh` | Gera `ari_secret.conf` a partir de `ARI_USERNAME`/`ARI_PASSWORD` (configs do Asterisk não expandem env vars) |
| `pjsip.conf` | Transportes udp/tcp e exemplo wss; template de trunk ITSP; endpoint de teste `6001`. **chan_pjsip apenas** — chan_sip foi removido do Asterisk 21+ |
| `extensions.conf` | Entrada → `Stasis(${GLOBAL(ACTIVE_APP)})` (blue/green); contexto `transfer-owner` para o failover ao celular do dono |
| `ari.conf` | ARI habilitado; usuário/senha via include gerado pelo entrypoint |
| `http.conf` | Servidor HTTP 8088: REST+WebSocket do ARI e transporte wss |
| `rtp.conf` | Range RTP 10000–10200 (casa com o `EXPOSE` do Dockerfile) |

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

```sh
docker build -t frontdesk-asterisk asterisk/
docker run --rm -e ARI_PASSWORD=changeme \
  -p 5060:5060/udp -p 8088:8088 -p 10000-10200:10000-10200/udp \
  frontdesk-asterisk
```

Depois: `MEDIA_BACKEND=asterisk ARI_URL=http://localhost:8088/ari ...` no
`telephony-gw` (integração no main acontece na fase de verificação).
