-- 0002_asterisk_realtime.sql — PJSIP Realtime (ARA) do backend Asterisk (ADR-009).
--
-- Troncos e ramais SIP saem do pjsip.conf e passam a viver nestas tabelas
-- ps_* (o esquema segue o alembic oficial do Asterisk, contrib/ast-db-manage,
-- reduzido às colunas que usamos — colunas ausentes assumem o default do
-- Asterisk). O Asterisk lê via res_config_pgsql (ver asterisk/extconfig.conf
-- e asterisk/sorcery.conf): INSERT/UPDATE aqui vale na próxima chamada ou
-- registro, sem reload — exceto ps_registrations (registro outbound em
-- tronco), que exige `asterisk -rx "pjsip reload"` após mudar.
--
-- SEM RLS de propósito: são tabelas de infraestrutura lidas pelo próprio
-- Asterisk (conexão direta, sem tenant no caminho), não dados de tenant.
-- Booleans são text 'yes'/'no' — é o que o realtime do Asterisk espera.

begin;

-- Endpoint = a "cara" SIP (codecs, contexto, NAT). Um por ramal/tronco.
create table if not exists ps_endpoints (
    id               text primary key,
    transport        text,
    aors             text,
    auth             text,
    context          text,
    disallow         text,
    allow            text,
    direct_media     text,
    dtmf_mode        text,
    force_rport      text,
    rewrite_contact  text,
    rtp_symmetric    text,
    outbound_auth    text,
    from_user        text,
    from_domain      text,
    callerid         text,
    language         text,
    identify_by      text,
    media_encryption text,
    ice_support      text,
    webrtc           text,
    -- res_pjsip_mwi varre "where mailboxes != ''" no boot — a coluna precisa
    -- existir mesmo sem voicemail configurado.
    mailboxes        text not null default ''
);

create table if not exists ps_auths (
    id        text primary key,
    auth_type text,
    username  text,
    password  text,
    realm     text
);

create table if not exists ps_aors (
    id                text primary key,
    contact           text,
    max_contacts      integer,
    qualify_frequency integer,
    remove_existing   text
);

-- Contatos dinâmicos (REGISTER de softphone): o Asterisk ESCREVE aqui via
-- realtime — o conjunto de colunas precisa cobrir tudo que ele grava.
create table if not exists ps_contacts (
    id                  text primary key,
    uri                 text,
    endpoint            text,
    expiration_time     bigint,
    qualify_frequency   integer,
    qualify_timeout     double precision,
    outbound_proxy      text,
    path                text,
    user_agent          text,
    reg_server          text,
    authenticate_qualify text,
    via_addr            text,
    via_port            integer,
    call_id             text,
    prune_on_boot       text,
    unique (id, reg_server)
);

-- identify: casa INVITE entrante com um endpoint pelo IP de origem (troncos
-- IP-auth, ex.: Telnyx).
create table if not exists ps_endpoint_id_ips (
    id           text primary key,
    endpoint     text,
    match        text,
    srv_lookups  text,
    match_header text
);

-- Registro OUTBOUND em tronco (ex.: Callcentric). Carregado no boot/reload do
-- res_pjsip_outbound_registration — após INSERT/UPDATE: `pjsip reload`.
create table if not exists ps_registrations (
    id                       text primary key,
    transport                text,
    outbound_auth            text,
    server_uri               text,
    client_uri               text,
    contact_user             text,
    retry_interval           integer,
    forbidden_retry_interval integer,
    max_retries              integer,
    expiration               integer,
    auth_rejection_permanent text,
    support_path             text,
    fatal_retry_error_codes  text,
    line                     text,
    endpoint                 text
);

-- ─── Seeds de laboratório (espelham o antigo pjsip.conf estático) ───

-- Softphone de teste 6001 (senha fraca de propósito: rede de lab isolada).
insert into ps_auths (id, auth_type, username, password)
values ('test-6001-auth', 'userpass', '6001', 'lab-only-6001')
on conflict (id) do nothing;

insert into ps_aors (id, max_contacts, remove_existing)
values ('test-6001', 1, 'yes')
on conflict (id) do nothing;

insert into ps_endpoints (id, transport, context, disallow, allow, auth, aors,
                          dtmf_mode, direct_media)
values ('test-6001', 'transport-udp', 'from-test', 'all', 'slin16,ulaw',
        'test-6001-auth', 'test-6001', 'rfc4733', 'no')
on conflict (id) do nothing;

-- Tronco ITSP (placeholder, mesmo papel do antigo template [trunk-primary]):
-- UPDATE nestas linhas com as credenciais reais ativa o tronco — sem reload
-- para chamadas; `pjsip reload` só se usar ps_registrations. Exemplos prontos
-- (Callcentric/Telnyx) em asterisk/README.md.
insert into ps_auths (id, auth_type, username, password)
values ('trunk-primary-auth', 'userpass', 'CHANGE_ME', 'CHANGE_ME')
on conflict (id) do nothing;

insert into ps_aors (id, contact, qualify_frequency)
values ('trunk-primary', 'sip:sip.example-itsp.ca:5060', 30)
on conflict (id) do nothing;

insert into ps_endpoints (id, transport, context, disallow, allow, aors,
                          outbound_auth, dtmf_mode, direct_media, rtp_symmetric,
                          force_rport, rewrite_contact, language)
values ('trunk-primary', 'transport-udp', 'from-trunk', 'all', 'ulaw,slin16',
        'trunk-primary', 'trunk-primary-auth', 'rfc4733', 'no', 'yes',
        'yes', 'yes', 'en')
on conflict (id) do nothing;

insert into ps_endpoint_id_ips (id, endpoint, match)
values ('trunk-primary-identify', 'trunk-primary', 'sip.example-itsp.ca')
on conflict (id) do nothing;

commit;
