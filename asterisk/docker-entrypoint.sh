#!/bin/sh
# Materializa segredos vindos de env vars como arquivos de config do Asterisk.
# (Arquivos .conf do Asterisk não expandem ${ENV}; os arquivos gerados aqui são
# a ponte entre o secret manager do ambiente e as configs estáticas.)
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

# Conexão do realtime (res_config_pgsql) — troncos/ramais nas tabelas ps_*.
# Mesmo Postgres do stack; defaults casam com o docker-compose.yml.
: "${PGSQL_HOST:=postgres}"
: "${PGSQL_PORT:=5432}"
: "${PGSQL_DBNAME:=frontdesk}"
: "${PGSQL_USER:=frontdesk}"
: "${PGSQL_PASSWORD:?PGSQL_PASSWORD é obrigatória (realtime no Postgres)}"

cat > /etc/asterisk/res_pgsql.conf <<EOF
; GERADO PELO ENTRYPOINT — não editar; fonte: env PGSQL_*.
[general]
dbhost = ${PGSQL_HOST}
dbport = ${PGSQL_PORT}
dbname = ${PGSQL_DBNAME}
dbuser = ${PGSQL_USER}
dbpass = ${PGSQL_PASSWORD}
; Coluna faltando na tabela ps_* vira warning no log, não boot abortado.
requirements = warn
EOF
chmod 600 /etc/asterisk/res_pgsql.conf

exec "$@"
