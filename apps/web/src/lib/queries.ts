/**
 * TanStack Query layer: queryOptions factories per resource + invalidation
 * helpers. Server state only ever flows through here (ADR-007).
 *
 * Freshness policy:
 *  - calls: staleTime 10s + refetchInterval 30s (near-real-time list);
 *  - messages/appointments: staleTime 30s (owner triage cadence);
 *  - tenant/dids/usage: staleTime 60s (config/billing changes rarely);
 * mutations invalidate their resource keys on success.
 */
import type { ApiClient } from "@frontdesk/shared";
import { queryOptions, type QueryClient } from "@tanstack/react-query";

export const queryKeys = {
  tenant: ["tenant"] as const,
  calls: ["calls"] as const,
  call: (callId: string) => ["calls", callId] as const,
  messages: ["messages"] as const,
  appointments: ["appointments"] as const,
  dids: ["dids"] as const,
  usage: ["usage"] as const,
};

export function tenantQuery(client: ApiClient) {
  return queryOptions({
    queryKey: queryKeys.tenant,
    queryFn: ({ signal }) => client.getTenant(signal),
    staleTime: 60_000,
  });
}

export function callsQuery(client: ApiClient) {
  return queryOptions({
    queryKey: queryKeys.calls,
    queryFn: ({ signal }) => client.listCalls(signal),
    staleTime: 10_000,
    refetchInterval: 30_000,
  });
}

export function callQuery(client: ApiClient, callId: string) {
  return queryOptions({
    queryKey: queryKeys.call(callId),
    queryFn: ({ signal }) => client.getCall(callId, signal),
    staleTime: 10_000,
  });
}

export function messagesQuery(client: ApiClient) {
  return queryOptions({
    queryKey: queryKeys.messages,
    queryFn: ({ signal }) => client.listMessages(signal),
    staleTime: 30_000,
  });
}

export function appointmentsQuery(client: ApiClient) {
  return queryOptions({
    queryKey: queryKeys.appointments,
    queryFn: ({ signal }) => client.listAppointments(signal),
    staleTime: 30_000,
  });
}

export function didsQuery(client: ApiClient) {
  return queryOptions({
    queryKey: queryKeys.dids,
    queryFn: ({ signal }) => client.listDids(signal),
    staleTime: 60_000,
  });
}

export function usageQuery(client: ApiClient) {
  return queryOptions({
    queryKey: queryKeys.usage,
    queryFn: ({ signal }) => client.getUsage(signal),
    staleTime: 60_000,
  });
}

/** After a mode change the tenant document is the source of truth. */
export function invalidateTenant(qc: QueryClient): Promise<void> {
  return qc.invalidateQueries({ queryKey: queryKeys.tenant });
}

export function invalidateAppointments(qc: QueryClient): Promise<void> {
  return qc.invalidateQueries({ queryKey: queryKeys.appointments });
}

export function invalidateMessages(qc: QueryClient): Promise<void> {
  return qc.invalidateQueries({ queryKey: queryKeys.messages });
}

export function invalidateDids(qc: QueryClient): Promise<void> {
  return qc.invalidateQueries({ queryKey: queryKeys.dids });
}
