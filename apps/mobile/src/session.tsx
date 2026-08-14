import * as SecureStore from "expo-secure-store";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactElement,
  type ReactNode,
} from "react";

import { ApiClient } from "@frontdesk/shared";

import { apiBaseUrl } from "./env";

const API_KEY_STORE = "frontdesk.apiKey";

export type SessionStatus = "loading" | "signedOut" | "signedIn";

export interface Session {
  status: SessionStatus;
  /** The stored tenant API key; undefined while signed out. */
  apiKey: string | undefined;
  /**
   * Typed core-api client bound to the current API key. While signed out the
   * client is unauthenticated — screens behind the auth gate never see it.
   */
  client: ApiClient;
  /** Validates the key against GET /v1/tenants/me, then persists it. */
  signIn: (apiKey: string) => Promise<void>;
  signOut: () => Promise<void>;
}

const SessionContext = createContext<Session | undefined>(undefined);

/**
 * API-key session backed by expo-secure-store (Keychain/Keystore). The key is
 * the tenant's `fdk_...` secret from onboarding — treated like a password.
 */
export function SessionProvider({ children }: { children: ReactNode }): ReactElement {
  const [status, setStatus] = useState<SessionStatus>("loading");
  const [apiKey, setApiKey] = useState<string | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      let stored: string | null = null;
      try {
        stored = await SecureStore.getItemAsync(API_KEY_STORE);
      } catch {
        // Keychain unavailable — treat as signed out.
      }
      if (cancelled) {
        return;
      }
      if (stored !== null && stored !== "") {
        setApiKey(stored);
        setStatus("signedIn");
      } else {
        setStatus("signedOut");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const signIn = useCallback(async (candidateKey: string) => {
    const trimmed = candidateKey.trim();
    const candidate = new ApiClient({ baseUrl: apiBaseUrl, apiKey: trimmed });
    // Throws ApiError(401) on a bad key — surfaced by the sign-in screen.
    await candidate.getTenant();
    await SecureStore.setItemAsync(API_KEY_STORE, trimmed);
    setApiKey(trimmed);
    setStatus("signedIn");
  }, []);

  const signOut = useCallback(async () => {
    await SecureStore.deleteItemAsync(API_KEY_STORE);
    setApiKey(undefined);
    setStatus("signedOut");
  }, []);

  const value = useMemo<Session>(
    () => ({
      status,
      apiKey,
      client: new ApiClient({ baseUrl: apiBaseUrl, apiKey }),
      signIn,
      signOut,
    }),
    [status, apiKey, signIn, signOut],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const session = useContext(SessionContext);
  if (session === undefined) {
    throw new Error("useSession must be used inside <SessionProvider>");
  }
  return session;
}
