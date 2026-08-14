# @frontdesk/web

FrontDesk AI owner dashboard — React 19 + Vite SPA over the Go core-api
(`/v1`), per ADR-007. Types, zod schemas, the API client and the EN/FR-CA
dictionaries all come from `@frontdesk/shared` (consumed by source).

## Run

```sh
pnpm --filter @frontdesk/web dev        # Vite dev server; proxies /v1 -> localhost:8080
pnpm --filter @frontdesk/web typecheck  # tsc --noEmit (strict)
pnpm --filter @frontdesk/web build      # typecheck + vite build (static bundle)
```

## Decisions

- **Code-based routing** (`src/router.tsx`, `createRoute`): the tree is small
  and this avoids the file-based codegen plugin, which is not part of the
  VERSIONS.md pinned set. Route types are registered via `Register`, so
  params and search params stay fully typed.
- **Search-param filters** on `/calls` are zod-validated (`callsSearchSchema`)
  and applied client-side — `GET /v1/calls` exposes no query params yet. The
  urgency filter joins calls to their structured message via `callId`.
- **"Mark as handled"** on messages is a device-local flag
  (`src/lib/handled.ts`): `domain.Message` has no handled field. Single swap
  point once the API grows one.
- **Dashboard p50 turn latency** folds `perceivedMs` over the turn latencies
  of up to 10 recent call details; **estimated job value** is
  `upcoming appointments x CAD 250` (documented heuristic — no price in the
  domain).
- **Settings**: only the answering mode (`PUT /v1/tenants/me/mode`) and
  numbers (`POST /v1/dids`) are mutable server-side; hours / coverage /
  owner mobile / default language render read-only from the tenant document.
- **i18n**: shared dictionaries fill the default namespace; web-only strings
  live in the `app` namespace (`src/i18n/app-{en,fr}.ts`, FR typed against
  EN so a missing key fails typecheck).
- **Dark mode** via `prefers-color-scheme`: semantic tokens on `:root`
  re-mapped in `@theme inline` (`src/styles.css`); components only ever use
  semantic utilities (`bg-surface`, `text-muted`, …).
