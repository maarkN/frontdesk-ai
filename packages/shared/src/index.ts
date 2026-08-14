/**
 * @frontdesk/shared — the single source of frontend truth for the core-api
 * contract: zod schemas mirroring the Go domain, the typed /v1 fetch client
 * and the EN/FR-CA UI dictionaries. Consumed by source (no build step) from
 * apps/web and apps/mobile.
 */
export * from "./schemas/index";
export * from "./api/client";
export * from "./i18n/index";
