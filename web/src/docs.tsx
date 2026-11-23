import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { addDocument, api, ApiError, type DocumentInput } from "./api";
import { advance, freshRun, type Run } from "./progress";
import { useSession } from "./session";
import type { DocumentRow } from "./types";

// Keeps documents and their processing progress for the whole app, so closing the upload
// window (or changing page) never stops or forgets a document being processed.

// model is the name of the AI model the user picked for this upload, shown in its progress text.
export type Pending = { key: string; name: string; kind: DocumentRow["kind"]; run: Run; id?: string; model?: string };

const GENERIC_MODEL = "The AI model";

type Ctx = {
  docs: DocumentRow[];
  pending: Pending[]; // uploads and documents still being processed, plus recently finished ones
  busy: number;
  version: number; // bumps whenever the profile may have changed
  open: boolean;
  setOpen: (v: boolean) => void;
  add: (input: DocumentInput & { name: string; kind: DocumentRow["kind"]; modelName?: string }) => void;
  remove: (id: string) => Promise<void>;
  dismiss: (key: string) => void;
  refresh: () => Promise<void>;
};

const DocsCtx = createContext<Ctx | null>(null);
export const useDocs = () => {
  const c = useContext(DocsCtx);
  if (!c) throw new Error("useDocs outside DocsProvider");
  return c;
};

export function DocsProvider({ children }: { children: ReactNode }) {
  const { session } = useSession();
  const active = Boolean(session?.user?.termsAccepted);
  const [docs, setDocs] = useState<DocumentRow[]>([]);
  const [pending, setPending] = useState<Pending[]>([]);
  const [version, setVersion] = useState(0);
  const [open, setOpen] = useState(false);
  const pendingRef = useRef(pending);
  useEffect(() => {
    pendingRef.current = pending;
  });

  const refresh = useCallback(async () => {
    if (!active) return;
    try {
      const { documents } = await api.documents();
      setDocs(documents);
      // Resume tracking anything still processing (for example after a page reload).
      setPending((cur) => {
        const known = new Set(cur.map((p) => p.id));
        const add = documents
          .filter((d) => d.status === "processing" && !known.has(d.id))
          .map((d) => ({ key: d.id, id: d.id, name: d.name, kind: d.kind, run: { ...freshRun(d.kind === "link"), stage: 1, target: 22, ceiling: 30, message: "Processing…" } }));
        return add.length ? [...cur, ...add] : cur;
      });
    } catch {
      /* shown elsewhere */
    }
  }, [active]);

  // refresh fetches first and sets state only when the response arrives.
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => void refresh(), [refresh]);

  const update = (key: string, fn: (p: Pending) => Pending) => setPending((cur) => cur.map((p) => (p.key === key ? fn(p) : p)));

  // Poll the server for progress on every document being processed.
  useEffect(() => {
    if (!active) return;
    const id = window.setInterval(async () => {
      const live = pendingRef.current.filter((p) => p.id && !p.run.done);
      for (const p of live) {
        try {
          const res = await api.documentProgress(p.id!, p.run.next);
          if (!res.events.length) continue;
          let finished = false;
          update(p.key, (cur) => {
            let run = cur.run;
            for (const ev of res.events) run = advance(run, ev, cur.model ?? GENERIC_MODEL);
            finished = run.done;
            return { ...cur, run: { ...run, next: res.next } };
          });
          if (finished || res.events.some((e) => e.type === "done" || e.type === "error")) {
            await refresh();
            setVersion((v) => v + 1);
          }
        } catch (err) {
          if (err instanceof ApiError && err.status === 404) {
            update(p.key, (cur) => ({ ...cur, run: { ...cur.run, done: true, error: "This document was removed." } }));
          }
        }
      }
    }, 900);
    return () => window.clearInterval(id);
  }, [active, refresh]);

  const add: Ctx["add"] = (input) => {
    const key = `up-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const model = input.modelName ?? GENERIC_MODEL;
    const p: Pending = { key, name: input.name, kind: input.kind, model, run: freshRun(input.kind === "link") };
    setPending((cur) => [...cur, p]);
    addDocument(input, (loaded, total) => update(key, (cur) => ({ ...cur, run: advance(cur.run, { type: "upload", loaded, total }, model) })))
      .then((id) => {
        update(key, (cur) => ({ ...cur, id, run: advance(cur.run, { type: "upload", loaded: 1, total: 1 }, model) }));
        void refresh();
      })
      .catch((err) => {
        const msg = err instanceof ApiError ? err.message : "Couldn't add that.";
        update(key, (cur) => ({ ...cur, run: advance(cur.run, { type: "error", error: msg }, model) }));
      });
  };

  const remove = async (id: string) => {
    await api.deleteDocument(id);
    setPending((cur) => cur.filter((p) => p.id !== id));
    await refresh();
    setVersion((v) => v + 1);
  };

  const dismiss = (key: string) => setPending((cur) => cur.filter((p) => p.key !== key));
  const busy = pending.filter((p) => !p.run.done).length;

  return (
    <DocsCtx.Provider value={{ docs, pending, busy, version, open, setOpen, add, remove, dismiss, refresh }}>{children}</DocsCtx.Provider>
  );
}
