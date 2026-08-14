/**
 * "Mark handled" for messages. The core-api Message model (Go: domain.Message)
 * has no handled flag yet, so this is device-local state persisted in
 * SecureStore; it will migrate to the API when the contract grows one.
 */
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

const HANDLED_STORE = "frontdesk.handledMessageIds";
/** SecureStore values should stay small — keep only the most recent ids. */
const MAX_IDS = 200;

interface HandledMessages {
  isHandled: (messageId: string) => boolean;
  markHandled: (messageId: string) => void;
}

const HandledContext = createContext<HandledMessages | undefined>(undefined);

export function HandledMessagesProvider({
  children,
}: {
  children: ReactNode;
}): ReactElement {
  const [ids, setIds] = useState<readonly string[]>([]);

  useEffect(() => {
    let cancelled = false;
    void SecureStore.getItemAsync(HANDLED_STORE)
      .then((stored) => {
        if (cancelled || stored === null || stored === "") {
          return;
        }
        setIds(stored.split(","));
      })
      .catch(() => {
        // Store unavailable — start empty.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const markHandled = useCallback((messageId: string) => {
    setIds((previous) => {
      if (previous.includes(messageId)) {
        return previous;
      }
      const next = [messageId, ...previous].slice(0, MAX_IDS);
      void SecureStore.setItemAsync(HANDLED_STORE, next.join(",")).catch(() => {
        // Persistence is best-effort; in-memory state still updates.
      });
      return next;
    });
  }, []);

  const value = useMemo<HandledMessages>(
    () => ({
      isHandled: (messageId) => ids.includes(messageId),
      markHandled,
    }),
    [ids, markHandled],
  );

  return <HandledContext.Provider value={value}>{children}</HandledContext.Provider>;
}

export function useHandledMessages(): HandledMessages {
  const value = useContext(HandledContext);
  if (value === undefined) {
    throw new Error("useHandledMessages must be used inside <HandledMessagesProvider>");
  }
  return value;
}
