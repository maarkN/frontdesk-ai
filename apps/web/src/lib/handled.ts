/**
 * Client-side "handled" state for messages. The core-api /v1 surface has no
 * handled field on domain.Message yet, so the dashboard keeps the triage
 * state per device in localStorage (keyed by message id). When the API gains
 * the field, this module is the single place to swap for a mutation.
 */
import { useCallback, useSyncExternalStore } from "react";

const STORAGE_KEY = "frontdesk.handledMessages";

type Listener = () => void;
const listeners = new Set<Listener>();

function read(): ReadonlySet<string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw === null) {
      return new Set();
    }
    const parsed: unknown = JSON.parse(raw);
    return new Set(
      Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === "string") : [],
    );
  } catch {
    return new Set();
  }
}

let snapshot: ReadonlySet<string> = read();

function write(next: Set<string>): void {
  snapshot = next;
  localStorage.setItem(STORAGE_KEY, JSON.stringify([...next]));
  for (const l of listeners) {
    l();
  }
}

export function useHandledMessages(): {
  handled: ReadonlySet<string>;
  toggle: (messageId: string) => void;
} {
  const handled = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => snapshot,
  );

  const toggle = useCallback((messageId: string) => {
    const next = new Set(snapshot);
    if (next.has(messageId)) {
      next.delete(messageId);
    } else {
      next.add(messageId);
    }
    write(next);
  }, []);

  return { handled, toggle };
}
