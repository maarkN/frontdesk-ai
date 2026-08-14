/**
 * API-key auth. The key lives in localStorage ("frontdesk.apiKey") and is an
 * explicit trade-off recorded in ADR-007 (SPA without a BFF). The context
 * exposes a memoized ApiClient bound to the key; signing out clears the key
 * and the query cache.
 */
import { ApiClient } from "@frontdesk/shared";
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

const STORAGE_KEY = "frontdesk.apiKey";

/** Same-origin base: Vite proxies /v1 in dev, CDN/host routes it in prod. */
const BASE_URL = "";

export interface AuthState {
  /** Present when signed in. */
  readonly apiKey: string | null;
  /** Client bound to the current key (unauthenticated when signed out). */
  readonly client: ApiClient;
  signIn(apiKey: string): void;
  signOut(): void;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [apiKey, setApiKey] = useState<string | null>(
    () => localStorage.getItem(STORAGE_KEY),
  );

  const signIn = useCallback((key: string) => {
    localStorage.setItem(STORAGE_KEY, key);
    setApiKey(key);
  }, []);

  const signOut = useCallback(() => {
    localStorage.removeItem(STORAGE_KEY);
    setApiKey(null);
  }, []);

  const value = useMemo<AuthState>(
    () => ({
      apiKey,
      client: new ApiClient({
        baseUrl: BASE_URL || window.location.origin,
        apiKey: apiKey ?? undefined,
      }),
      signIn,
      signOut,
    }),
    [apiKey, signIn, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (ctx === null) {
    throw new Error("useAuth must be used inside <AuthProvider>");
  }
  return ctx;
}
