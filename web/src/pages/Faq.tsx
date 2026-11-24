import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { api } from "../api";
import { Md } from "../components/journey/Md";
import type { Prep, PrepItem } from "../types";

// The profile's FAQ: the generic HR questions your jobs ask, each answered once, and the
// tasks your journeys share (tick one here or in any journey and it's ticked everywhere).

const TRACK_LABEL: Record<string, string> = {
  general: "Any role", frontend: "Frontend", backend: "Backend", fullstack: "Full stack", mobile: "Mobile", data: "Data",
  devops: "DevOps", ml: "Machine learning", security: "Security", qa: "QA", management: "Management",
};
const trackOf = (key: string) => key.split("/")[0];
const trackLabel = (t: string) => TRACK_LABEL[t] ?? t.charAt(0).toUpperCase() + t.slice(1);
const jobCount = (it: PrepItem) => new Set(it.places.map((p) => p.jobId)).size;

export function Faq() {
  const [data, setData] = useState<Prep | null>(null);
  const [error, setError] = useState("");
  const [picked, setTab] = useState<"hr" | "task" | null>(null);
  const [q, setQ] = useState("");
  const location = useLocation();

  const load = useCallback(() => api.prep().then(setData, (e) => setError(e.message)), []);
  useEffect(() => void load(), [load]);
  // While journeys are still being compared, refresh.
  useEffect(() => {
    if (!data?.unlinked) return;
    const id = window.setTimeout(load, 4000);
    return () => window.clearTimeout(id);
  }, [data, load]);
  // Opened from a journey with #<item id>: show that item's tab and scroll to it.
  const linked = data?.items.find((x) => x.id === location.hash.slice(1));
  const tab = picked ?? linked?.kind ?? "hr";
  const linkedId = linked?.id;
  useEffect(() => {
    if (!linkedId) return;
    const t = window.setTimeout(() => document.getElementById(linkedId)?.scrollIntoView({ behavior: "smooth", block: "center" }), 50);
    return () => window.clearTimeout(t);
  }, [linkedId]);

  const hr = useMemo(() => (data?.items ?? []).filter((x) => x.kind === "hr"), [data]);
  const tasks = useMemo(() => (data?.items ?? []).filter((x) => x.kind === "task"), [data]);
  const match = (it: PrepItem) => {
    const s = q.trim().toLowerCase();
    return !s || it.label.toLowerCase().includes(s) || it.answer.toLowerCase().includes(s) || it.places.some((p) => p.text.toLowerCase().includes(s) || p.title.toLowerCase().includes(s));
  };

  if (error) return <div className="narrow card err">{error}</div>;
  if (!data) return null;

  const answered = hr.filter((x) => x.answer).length;
  const sharedTasks = tasks.filter((t) => jobCount(t) > 1);
  const doneShared = sharedTasks.filter((t) => t.done).length;
  const saved = sharedTasks.reduce((n, t) => n + jobCount(t) - 1, 0);

  const update = (id: string, patch: Partial<PrepItem>) =>
    setData((d) => (d ? { ...d, items: d.items.map((x) => (x.id === id ? { ...x, ...patch } : x)) } : d));

  return (
    <div className="page narrow faq">
      <div className="section-title">
        <div>
          <h1 className="page-title" style={{ margin: 0 }}>Interview FAQ</h1>
          <div className="muted" style={{ fontSize: 13 }}>Prep that carries across your job applications: answer once, tick once.</div>
        </div>
        {data.matching ? (
          <button
            className="btn ghost"
            style={{ fontSize: 13, padding: "6px 12px" }}
            disabled={data.unlinked > 0}
            onClick={() => api.resyncPrep().then(load, (e) => setError(e.message))}
            title="Compare all your journeys again. Your answers and ticks are kept."
          >
            Find overlaps again
          </button>
        ) : null}
      </div>

      {data.unlinked ? (
        <div className="fillbar" role="status">
          <span className="row" style={{ gap: 10 }}>
            <span className="spinner" aria-hidden="true" />
            Comparing {data.unlinked} journey{data.unlinked === 1 ? "" : "s"} with your others…
          </span>
        </div>
      ) : null}

      <div className="faq-stats">
        <Stat value={answered} of={hr.length} label="HR answers written" color="var(--c-hr)" />
        <Stat value={doneShared} of={sharedTasks.length} label="shared tasks done" color="var(--c-tech)" />
        <div className="card stat">
          <b>{saved}</b>
          <span className="muted">repeat task{saved === 1 ? "" : "s"} you won&apos;t do twice</span>
        </div>
      </div>

      <div className="row" style={{ margin: "16px 0 12px" }}>
        <button className={`tab ${tab === "hr" ? "on" : ""}`} onClick={() => setTab("hr")}>HR questions · {hr.length}</button>
        <button className={`tab ${tab === "task" ? "on" : ""}`} onClick={() => setTab("task")}>Shared tasks · {sharedTasks.length}</button>
        <input type="text" placeholder="Search" value={q} onChange={(e) => setQ(e.target.value)} style={{ flex: 1, minWidth: 160, padding: "6px 12px" }} aria-label="Search the FAQ" />
      </div>

      {tab === "hr" ? (
        hr.length === 0 ? (
          <Empty unlinked={data.unlinked} matching={data.matching} what="generic HR questions" />
        ) : (
          hr.filter(match).map((it) => <HRItem key={it.id} it={it} open={location.hash === `#${it.id}`} onSaved={(answer) => update(it.id, { answer })} />)
        )
      ) : sharedTasks.length === 0 ? (
        <Empty unlinked={data.unlinked} matching={data.matching} what="tasks that appear in more than one job" />
      ) : (
        <TaskGroups items={sharedTasks.filter(match)} onDone={(id, done) => update(id, { done })} onError={setError} />
      )}
    </div>
  );
}

function Stat({ value, of, label, color }: { value: number; of: number; label: string; color: string }) {
  return (
    <div className="card stat">
      <b>{value}<span className="muted" style={{ fontSize: 15 }}> / {of}</span></b>
      <span className="muted">{label}</span>
      <div className="bar"><i style={{ width: `${of ? (100 * value) / of : 0}%`, background: color }} /></div>
    </div>
  );
}

function Empty({ unlinked, matching, what }: { unlinked: number; matching: boolean; what: string }) {
  return (
    <div className="card muted">
      {unlinked
        ? "Your journeys are being compared. This list fills in as each one is done."
        : !matching
          ? "Finding shared prep needs an AI model. See the README to configure one."
          : `No ${what} yet. They appear here as you add job applications.`}
    </div>
  );
}

function HRItem({ it, open, onSaved }: { it: PrepItem; open: boolean; onSaved: (answer: string) => void }) {
  const [draft, setDraft] = useState(it.answer);
  const [state, setState] = useState<"" | "saving" | "saved" | "error">("");
  const n = jobCount(it);
  const save = () => {
    if (draft.trim() === it.answer) return;
    setState("saving");
    api.setPrepAnswer(it.id, draft).then(
      () => {
        setState("saved");
        onSaved(draft.trim());
      },
      () => setState("error"),
    );
  };
  return (
    <details id={it.id} className="faq-item" open={open || undefined}>
      <summary>
        <span className={`dot ${it.answer ? "on" : ""}`} aria-label={it.answer ? "Answered" : "Not answered"} />
        <span className="q">{it.label}</span>
        {n ? <span className="pill hr">Asked in {n} job{n === 1 ? "" : "s"}</span> : <span className="pill">Kept from a deleted job</span>}
      </summary>
      <div className="faq-body">
        <label className="f" htmlFor={`a-${it.id}`} style={{ marginTop: 0 }}>Your answer</label>
        <textarea
          id={`a-${it.id}`}
          value={draft}
          maxLength={5000}
          placeholder="Write the answer you'll give. A short story (situation, what you did, the result) works best."
          onChange={(e) => {
            setDraft(e.target.value);
            setState("");
          }}
          onBlur={save}
        />
        <div className="row" style={{ justifyContent: "space-between", fontSize: 12.5 }}>
          <span className={state === "error" ? "err" : "muted"} style={{ margin: 0 }}>
            {state === "saving" ? "Saving…" : state === "saved" ? "Saved. Every job that asks this uses it." : state === "error" ? "Couldn't save. Check your connection." : "Saves when you click away."}
          </span>
          <button className="btn ghost" style={{ fontSize: 12.5, padding: "4px 10px" }} onClick={save} disabled={draft.trim() === it.answer}>Save</button>
        </div>
        {it.places.length ? (
          <>
            <h3>Tips from each job</h3>
            {it.places.map((p, i) => (
              <div key={i} className="faq-tip">
                <Link to={`/jobs/${p.jobId}`}>{p.title}</Link>
                {p.text && p.text !== it.label ? <div className="muted" style={{ fontSize: 13 }}>&ldquo;{p.text}&rdquo;</div> : null}
                {p.tip ? <Md text={p.tip} /> : null}
              </div>
            ))}
          </>
        ) : null}
      </div>
    </details>
  );
}

function TaskGroups({ items, onDone, onError }: { items: PrepItem[]; onDone: (id: string, done: boolean) => void; onError: (m: string) => void }) {
  const groups = new Map<string, PrepItem[]>();
  for (const it of items) {
    const t = trackOf(it.key);
    groups.set(t, [...(groups.get(t) ?? []), it]);
  }
  const toggle = (it: PrepItem, done: boolean) => {
    onDone(it.id, done);
    api.setPrepDone(it.id, done).catch(() => {
      onDone(it.id, !done);
      onError("Couldn't save. Check your connection.");
    });
  };
  return (
    <>
      {[...groups].map(([track, list]) => (
        <div key={track}>
          <h3>{trackLabel(track)} <span className="muted" style={{ fontWeight: 400 }}>· {list.filter((x) => x.done).length}/{list.length} done</span></h3>
          {list.map((it) => (
            <label key={it.id} id={it.id} className={`task ${it.done ? "done" : ""}`}>
              <input type="checkbox" checked={it.done} onChange={(e) => toggle(it, e.target.checked)} />
              <span>
                <span>{it.label}</span>
                <span className="faq-jobs">
                  {[...new Map(it.places.map((p) => [p.jobId, p])).values()].map((p) => (
                    <Link key={p.jobId} className="pill job" to={`/jobs/${p.jobId}`} onClick={(e) => e.stopPropagation()}>{p.title}</Link>
                  ))}
                </span>
              </span>
            </label>
          ))}
        </div>
      ))}
    </>
  );
}
