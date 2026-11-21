import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api } from "./api";
import type { Session } from "./types";

type Ctx = { session: Session | null; refresh: () => Promise<void>; setSession: (s: Session) => void };
const SessionCtx = createContext<Ctx>({ session: null, refresh: async () => {}, setSession: () => {} });
export const useSession = () => useContext(SessionCtx);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [failed, setFailed] = useState(false);
  const refresh = useCallback(async () => {
    try {
      setSession(await api.session());
      setFailed(false);
    } catch {
      setFailed(true);
    }
  }, []);
  // refresh fetches first and sets state only when the response arrives.
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => void refresh(), [refresh]);
  if (failed) return <div className="narrow card" style={{ marginTop: 60 }}>Couldn&apos;t reach the server. <button className="btn ghost" onClick={refresh}>Retry</button></div>;
  if (!session) return null;
  return <SessionCtx.Provider value={{ session, refresh, setSession }}>{children}</SessionCtx.Provider>;
}
