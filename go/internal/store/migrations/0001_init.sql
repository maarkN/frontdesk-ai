-- 0001_init.sql — esquema inicial do core-api com isolamento por RLS (ADR-005).
--
-- Regras:
--   * Toda tabela de tenant tem ENABLE + FORCE ROW LEVEL SECURITY — o FORCE
--     garante que nem o role dono da tabela escapa da policy.
--   * A policy usa current_setting('app.tenant_id', true): sem o setting a
--     expressão vira NULL e NENHUMA linha retorna (nunca erro silencioso com
--     tenant default).
--   * O valor é definido por transação via set_config(..., true) — escopo
--     LOCAL, obrigatório com pool de conexões.
--   * tenant_api_keys é a borda de resolução (API key -> tenant) e por isso
--     NÃO tem RLS: é o único lugar autorizado a mapear credencial em tenant.

begin;

create table if not exists tenants (
    id               text primary key,
    name             text not null,
    plan             text not null,
    mode             text not null,
    overflow_seconds integer not null default 0,
    hours            jsonb not null default '{}'::jsonb,
    coverage         jsonb not null default '{}'::jsonb,
    owner_mobile     text not null default '',
    default_locale   text not null,
    trial_ends_at    timestamptz,
    created_at       timestamptz not null
);

alter table tenants enable row level security;
alter table tenants force row level security;

create policy tenant_isolation on tenants
    using (id = current_setting('app.tenant_id', true))
    with check (id = current_setting('app.tenant_id', true));

-- Borda de resolução: sem RLS, contém apenas hash da chave -> tenant.
create table if not exists tenant_api_keys (
    key_hash   text primary key,
    tenant_id  text not null references tenants (id) on delete cascade,
    created_at timestamptz not null
);

create table if not exists dids (
    number     text primary key, -- unicidade global entre tenants
    tenant_id  text not null references tenants (id) on delete cascade,
    provider   text not null,
    active     boolean not null default true,
    created_at timestamptz not null
);

alter table dids enable row level security;
alter table dids force row level security;

create policy tenant_isolation on dids
    using (tenant_id = current_setting('app.tenant_id', true))
    with check (tenant_id = current_setting('app.tenant_id', true));

create table if not exists calls (
    call_id       text not null,
    tenant_id     text not null references tenants (id) on delete cascade,
    started_at    timestamptz,
    snapshot      jsonb not null,
    recording_url text not null default '',
    primary key (tenant_id, call_id)
);

alter table calls enable row level security;
alter table calls force row level security;

create policy tenant_isolation on calls
    using (tenant_id = current_setting('app.tenant_id', true))
    with check (tenant_id = current_setting('app.tenant_id', true));

create index if not exists calls_started_at_idx on calls (tenant_id, started_at desc);

create table if not exists appointments (
    id             text not null,
    tenant_id      text not null references tenants (id) on delete cascade,
    call_id        text not null default '',
    customer_name  text not null,
    customer_phone text not null default '',
    service        text not null,
    postal_code    text not null default '',
    starts_at      timestamptz not null,
    ends_at        timestamptz not null,
    notes          text not null default '',
    created_at     timestamptz not null,
    primary key (tenant_id, id)
);

alter table appointments enable row level security;
alter table appointments force row level security;

create policy tenant_isolation on appointments
    using (tenant_id = current_setting('app.tenant_id', true))
    with check (tenant_id = current_setting('app.tenant_id', true));

create table if not exists messages (
    id             text not null,
    tenant_id      text not null references tenants (id) on delete cascade,
    call_id        text not null default '',
    who            text not null,
    what           text not null,
    urgency        text not null,
    callback_phone text not null,
    created_at     timestamptz not null,
    primary key (tenant_id, id)
);

alter table messages enable row level security;
alter table messages force row level security;

create policy tenant_isolation on messages
    using (tenant_id = current_setting('app.tenant_id', true))
    with check (tenant_id = current_setting('app.tenant_id', true));

commit;
