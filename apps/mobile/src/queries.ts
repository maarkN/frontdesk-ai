/**
 * TanStack Query hooks over the shared ApiClient — the same server-state
 * pattern as apps/web (ADR-007). Lists poll: the owner watches calls land in
 * near-real-time.
 */
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from "@tanstack/react-query";

import {
  ApiError,
  type Appointment,
  type Call,
  type CallSummary,
  type Message,
  type Tenant,
  type UpdateModeRequest,
} from "@frontdesk/shared";

import { useSession } from "./session";

export const queryKeys = {
  tenant: ["tenant"] as const,
  calls: ["calls"] as const,
  call: (id: string) => ["calls", id] as const,
  messages: ["messages"] as const,
  appointments: ["appointments"] as const,
};

/** Never retry auth/not-found errors; give transient failures two more tries. */
export function retryPolicy(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && (error.isUnauthorized || error.isNotFound)) {
    return false;
  }
  return failureCount < 2;
}

export function useTenantQuery(): UseQueryResult<Tenant, Error> {
  const { client } = useSession();
  return useQuery({
    queryKey: queryKeys.tenant,
    queryFn: ({ signal }) => client.getTenant(signal),
  });
}

export function useCallsQuery(): UseQueryResult<CallSummary[], Error> {
  const { client } = useSession();
  return useQuery({
    queryKey: queryKeys.calls,
    queryFn: ({ signal }) => client.listCalls(signal),
    refetchInterval: 15_000,
  });
}

export function useCallQuery(callId: string): UseQueryResult<Call, Error> {
  const { client } = useSession();
  return useQuery({
    queryKey: queryKeys.call(callId),
    queryFn: ({ signal }) => client.getCall(callId, signal),
    enabled: callId !== "",
  });
}

export function useMessagesQuery(): UseQueryResult<Message[], Error> {
  const { client } = useSession();
  return useQuery({
    queryKey: queryKeys.messages,
    queryFn: ({ signal }) => client.listMessages(signal),
    refetchInterval: 30_000,
  });
}

export function useAppointmentsQuery(): UseQueryResult<Appointment[], Error> {
  const { client } = useSession();
  return useQuery({
    queryKey: queryKeys.appointments,
    queryFn: ({ signal }) => client.listAppointments(signal),
    refetchInterval: 60_000,
  });
}

/** The product's #1 control: overflow vs always-AI. */
export function useUpdateModeMutation(): UseMutationResult<
  Tenant,
  Error,
  UpdateModeRequest
> {
  const { client } = useSession();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: UpdateModeRequest) => client.updateMode(request),
    onSuccess: (tenant) => {
      queryClient.setQueryData(queryKeys.tenant, tenant);
    },
  });
}
