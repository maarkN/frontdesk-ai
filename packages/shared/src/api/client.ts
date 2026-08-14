/**
 * Typed fetch client for the core-api REST surface (/v1, Go: internal/api).
 *
 * Framework-free: no dependency on DOM lib types, React, TanStack or any
 * runtime beyond `fetch` (web, React Native and Node >= 20 all provide one).
 * Every response is validated with zod at the edge — a payload that drifts
 * from the Go contract fails early with a useful message.
 *
 * Auth is the per-tenant API key sent as `Authorization: Bearer fdk_...`
 * (the server also accepts X-API-Key). Unknown key means 401 — there is
 * never a default tenant.
 */
import type { z } from "zod";

import {
  appointmentSchema,
  callSchema,
  callSummarySchema,
  createTenantResponseSchema,
  didSchema,
  messageSchema,
  tenantSchema,
  usageResponseSchema,
  apiErrorBodySchema,
  type AddDidRequest,
  type Appointment,
  type Call,
  type CallSummary,
  type CreateAppointmentRequest,
  type CreateMessageRequest,
  type CreateTenantRequest,
  type CreateTenantResponse,
  type DID,
  type Message,
  type Tenant,
  type UpdateModeRequest,
  type UsageResponse,
} from "../schemas";

/**
 * Structural stand-in for AbortSignal — keeps DOM/Node lib types out of this
 * package. Real AbortSignal instances satisfy it.
 */
export interface AbortSignalLike {
  readonly aborted: boolean;
}

/** Minimal structural view of a fetch Response — keeps DOM types out. */
export interface FetchResponseLike {
  readonly ok: boolean;
  readonly status: number;
  text(): Promise<string>;
}

/** The request init this client sends. */
export interface FetchRequestInit {
  method: string;
  headers: Record<string, string>;
  body?: string;
  signal?: AbortSignalLike;
}

/**
 * Minimal structural view of fetch itself. `init` is intentionally loose
 * (`any`) so that platform fetch implementations (DOM, React Native,
 * Node >= 20 undici) are directly assignable without casts; the client only
 * ever passes a `FetchRequestInit`.
 */
export type FetchLike = (
  url: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- deliberate: keeps DOM/RN/Node fetch signatures assignable.
  init?: any,
) => Promise<FetchResponseLike>;

/** Error thrown for any non-2xx response. */
export class ApiError extends Error {
  /** HTTP status code. */
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }

  /** True for 401 — missing or unknown API key. */
  get isUnauthorized(): boolean {
    return this.status === 401;
  }

  /** True for 404 — absent, or belonging to another tenant (looks identical). */
  get isNotFound(): boolean {
    return this.status === 404;
  }

  /** True for 409 — uniqueness violation (e.g. DID already provisioned). */
  get isConflict(): boolean {
    return this.status === 409;
  }
}

/** Error thrown when a 2xx payload does not match the shared zod schema. */
export class ApiContractError extends Error {
  constructor(operation: string, cause: z.ZodError) {
    super(`core-api contract violation in ${operation}: ${cause.message}`);
    this.name = "ApiContractError";
  }
}

export interface ApiClientOptions {
  /** Base URL of the core-api, e.g. "https://api.frontdesk.example". Trailing slashes are trimmed. */
  baseUrl: string;
  /** Tenant API key ("fdk_..."). Optional so onboarding can run unauthenticated. */
  apiKey?: string | undefined;
  /** Custom fetch (tests, polyfills). Defaults to globalThis.fetch. */
  fetch?: FetchLike | undefined;
}

interface RequestOptions {
  method: "GET" | "POST" | "PUT";
  path: string;
  body?: unknown;
  signal?: AbortSignalLike | undefined;
  /** Skip the Authorization header (onboarding runs before any key exists). */
  unauthenticated?: boolean;
}

/**
 * Typed client for the /v1 surface. One instance per (baseUrl, apiKey) pair;
 * `withApiKey` derives an authenticated client after onboarding.
 */
export class ApiClient {
  private readonly baseUrl: string;
  private readonly apiKey: string | undefined;
  private readonly fetchImpl: FetchLike;

  constructor(options: ApiClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/+$/, "");
    this.apiKey = options.apiKey;
    const fetchImpl =
      options.fetch ?? (globalThis as { fetch?: FetchLike }).fetch;
    if (fetchImpl === undefined) {
      throw new Error("ApiClient: no fetch implementation available");
    }
    this.fetchImpl = fetchImpl;
  }

  /** Returns a new client bound to the given API key (same baseUrl/fetch). */
  withApiKey(apiKey: string): ApiClient {
    return new ApiClient({
      baseUrl: this.baseUrl,
      apiKey,
      fetch: this.fetchImpl,
    });
  }

  // -- Tenants ------------------------------------------------------------

  /** Self-service onboarding. The returned apiKey is shown exactly once. */
  createTenant(
    req: CreateTenantRequest,
    signal?: AbortSignalLike,
  ): Promise<CreateTenantResponse> {
    return this.request(createTenantResponseSchema, "createTenant", {
      method: "POST",
      path: "/v1/tenants",
      body: req,
      signal,
      unauthenticated: true,
    });
  }

  /** The authenticated tenant. */
  getTenant(signal?: AbortSignalLike): Promise<Tenant> {
    return this.request(tenantSchema, "getTenant", {
      method: "GET",
      path: "/v1/tenants/me",
      signal,
    });
  }

  /** Toggle overflow / always-AI; returns the updated tenant. */
  updateMode(req: UpdateModeRequest, signal?: AbortSignalLike): Promise<Tenant> {
    return this.request(tenantSchema, "updateMode", {
      method: "PUT",
      path: "/v1/tenants/me/mode",
      body: req,
      signal,
    });
  }

  // -- DIDs ---------------------------------------------------------------

  /** The tenant's provisioned numbers. */
  listDids(signal?: AbortSignalLike): Promise<DID[]> {
    return this.request(didSchema.array(), "listDids", {
      method: "GET",
      path: "/v1/dids",
      signal,
    });
  }

  /** Provision a number (409 when it is already provisioned). */
  addDid(req: AddDidRequest, signal?: AbortSignalLike): Promise<DID> {
    return this.request(didSchema, "addDid", {
      method: "POST",
      path: "/v1/dids",
      body: req,
      signal,
    });
  }

  // -- Calls --------------------------------------------------------------

  /** Call summaries, most recent first. */
  listCalls(signal?: AbortSignalLike): Promise<CallSummary[]> {
    return this.request(callSummarySchema.array(), "listCalls", {
      method: "GET",
      path: "/v1/calls",
      signal,
    });
  }

  /** Full folded snapshot (transcript, latencies, usage) + recording URL. */
  getCall(callId: string, signal?: AbortSignalLike): Promise<Call> {
    return this.request(callSchema, "getCall", {
      method: "GET",
      path: `/v1/calls/${encodeURIComponent(callId)}`,
      signal,
    });
  }

  // -- Appointments -------------------------------------------------------

  /** Appointments ordered by start time. */
  listAppointments(signal?: AbortSignalLike): Promise<Appointment[]> {
    return this.request(appointmentSchema.array(), "listAppointments", {
      method: "GET",
      path: "/v1/appointments",
      signal,
    });
  }

  /** Book a visit manually from the dashboard. */
  createAppointment(
    req: CreateAppointmentRequest,
    signal?: AbortSignalLike,
  ): Promise<Appointment> {
    return this.request(appointmentSchema, "createAppointment", {
      method: "POST",
      path: "/v1/appointments",
      body: req,
      signal,
    });
  }

  // -- Messages -----------------------------------------------------------

  /** Structured messages, most recent first. */
  listMessages(signal?: AbortSignalLike): Promise<Message[]> {
    return this.request(messageSchema.array(), "listMessages", {
      method: "GET",
      path: "/v1/messages",
      signal,
    });
  }

  /** Record a structured message (who / what / urgency / callback). */
  createMessage(
    req: CreateMessageRequest,
    signal?: AbortSignalLike,
  ): Promise<Message> {
    return this.request(messageSchema, "createMessage", {
      method: "POST",
      path: "/v1/messages",
      body: req,
      signal,
    });
  }

  // -- Usage --------------------------------------------------------------

  /** Current-period usage (fold over the event stream) + invoice preview. */
  getUsage(signal?: AbortSignalLike): Promise<UsageResponse> {
    return this.request(usageResponseSchema, "getUsage", {
      method: "GET",
      path: "/v1/usage",
      signal,
    });
  }

  // -- Plumbing -----------------------------------------------------------

  private async request<Schema extends z.ZodType>(
    schema: Schema,
    operation: string,
    options: RequestOptions,
  ): Promise<z.output<Schema>> {
    const headers: Record<string, string> = {
      Accept: "application/json",
    };
    if (options.body !== undefined) {
      headers["Content-Type"] = "application/json";
    }
    if (options.unauthenticated !== true) {
      if (this.apiKey === undefined) {
        throw new ApiError(401, `${operation}: no API key configured`);
      }
      headers["Authorization"] = `Bearer ${this.apiKey}`;
    }

    const init: FetchRequestInit = { method: options.method, headers };
    if (options.body !== undefined) {
      init.body = JSON.stringify(options.body);
    }
    if (options.signal !== undefined) {
      init.signal = options.signal;
    }

    const response = await this.fetchImpl(this.baseUrl + options.path, init);

    const text = await response.text();
    if (!response.ok) {
      throw new ApiError(response.status, errorMessage(text, response.status));
    }

    let payload: unknown;
    try {
      payload = JSON.parse(text) as unknown;
    } catch {
      throw new ApiError(
        response.status,
        `${operation}: invalid JSON in response`,
      );
    }

    const parsed = schema.safeParse(payload);
    if (!parsed.success) {
      throw new ApiContractError(operation, parsed.error);
    }
    return parsed.data;
  }
}

/** Extracts the uniform `{ "error": "..." }` body, falling back to the status. */
function errorMessage(text: string, status: number): string {
  try {
    const parsed = apiErrorBodySchema.safeParse(JSON.parse(text) as unknown);
    if (parsed.success) {
      return parsed.data.error;
    }
  } catch {
    // Not JSON — fall through.
  }
  return `HTTP ${status}`;
}
