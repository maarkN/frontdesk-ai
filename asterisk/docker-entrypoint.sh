#!/bin/sh
# Materializa segredos vindos de env vars como arquivos #include do Asterisk.
# (Arquivos .conf do Asterisk não expandem ${ENV}; o include gerado aqui é a
# ponte entre o secret manager do ambiente e o ari.conf estático.)
set -eu

: "${ARI_USERNAME:=frontdesk}"
: "${ARI_PASSWORD:?ARI_PASSWORD é obrigatória (senha do usuário ARI)}"

cat > /etc/asterisk/ari_secret.conf <<EOF
; GERADO PELO ENTRYPOINT — não editar; fonte: env ARI_USERNAME/ARI_PASSWORD.
[${ARI_USERNAME}]
type = user
read_only = no
password = ${ARI_PASSWORD}
EOF
chmod 600 /etc/asterisk/ari_secret.conf

exec "$@"
