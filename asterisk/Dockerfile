# Asterisk 22 LTS — backend de mídia alternativo do telephony-gw (ADR-009).
#
# Escolha da imagem (documentada): Debian 13 "trixie" empacota Asterisk 22 LTS
# (chan_pjsip nativo, chan_sip já removido upstream). Instalar o pacote da
# distro em vez de compilar da fonte corta o build de ~30min para ~1min,
# recebe patches de segurança via apt e é suficiente para o ambiente de
# laboratório/staging do ADR-009 (o backend Asterisk NÃO está no caminho de
# produção do MVP). Se um módulo fora do pacote for necessário no futuro
# (ex.: res_audiosocket custom), trocar para build da fonte em multi-stage.
FROM debian:trixie-slim

# Pin da série 22 (LTS). O sufixo exato de patch fica a cargo do apt para
# receber correções; a série é garantida pela distro.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        asterisk \
        ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && asterisk -V | grep -q "Asterisk 22"

# Configuração do gateway: substitui a config default da distro.
COPY --chown=asterisk:asterisk pjsip.conf extensions.conf ari.conf http.conf rtp.conf /etc/asterisk/
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh \
    && install -d -o asterisk -g asterisk /var/run/asterisk /var/lib/asterisk /var/log/asterisk /var/spool/asterisk

# SIP (UDP/TCP), HTTP/WS (ARI + wss), RTP (range de rtp.conf).
EXPOSE 5060/udp 5060/tcp 8088/tcp 10000-10200/udp

USER asterisk

# O entrypoint materializa segredos (ARI) a partir de env vars antes do boot —
# arquivos de config do Asterisk não expandem variáveis de ambiente.
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["asterisk", "-f", "-p"]
