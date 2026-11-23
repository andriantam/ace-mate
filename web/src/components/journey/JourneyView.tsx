import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../../api";
import { aspectScore, bestNext, overallScore, pct, unwritten } from "../../readiness";
import { ASPECTS, type Journey, type People, type Progress, type SharedRef } from "../../types";
import { Ring } from "../Ring";
import { useMobile } from "../../useMobile";
import { JourneyMap } from "./JourneyMap";
import { JourneyTree } from "./JourneyTree";
import { NodePanel } from "./NodePanel";
import { aspectBy, safeHref } from "./util";

// One job's journey: four lanes of milestones (a map on desktop, lists on phones), the
// readiness scores, and the panel for the selected node.

type Props = {
  jobId: string;
  url: string | null;
  journey: Journey;
  initialProgress: Progress;
  initialPeople: People;
  /** Tasks and HR questions shared with the user's other jobs, keyed by ref. */
  shared?: Record<string, SharedRef>;
  /** Shown above the journey while parts of it are still being written. */
  banner?: React.ReactNode;
  /** Retries the parts that couldn't be written. */
  onFill?: () => void;
  building?: boolean;
};

export function JourneyView({ jobId, url, journey: j, initialProgress, initialPeople, shared = {}, banner, onFill, building }: Props) {
  const [progress, setProgress] = useState<Progress>(initialProgress);
  const [sel, setSel] = useState<string | null>(null);
  const [fitOpen, setFitOpen] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const navigate = useNavigate();

  const nodeById = useMemo(() => Object.fromEntries(j.nodes.map((n) => [n.id, n])), [j]);
  const waiting = useMemo(() => unwritten(j), [j]);
  const mobile = useMobile();
  const failed = j.failed ?? [];
  const o = overallScore(j, progress);
  const nb = bestNext(j, progress);

  function toggle(item: string, done: boolean) {
    const prev = progress[item] ?? false;
    setProgress((p) => ({ ...p, [item]: done }));
    api.setProgress(jobId, item, done).then(
      ({ changed }) => {
        setSaveError(null);
        // A shared task can appear more than once in this journey: tick every copy.
        const here = (changed ?? []).filter((c) => c.jobId === jobId);
        if (here.length) setProgress((p) => ({ ...p, ...Object.fromEntries(here.map((c) => [c.item, done])) }));
      },
      () => {
        setProgress((p) => ({ ...p, [item]: prev }));
        setSaveError("Couldn't save. Check your connection.");
      },
    );
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setSel(null);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const m = j.meta;
  const metaLine = [m.company, m.band, m.location, m.workType, m.closes && `closes ${m.closes}`].filter(Boolean).join(" · ");

  return (
    <>
      {banner}
      {!building && failed.length ? (
        <div className="fillbar" role="status">
          <span>
            {failed.length} part{failed.length === 1 ? "" : "s"} of this journey couldn&apos;t be written. Everything else is ready to use.
          </span>
          {onFill ? <button className="btn" onClick={onFill}>Retry missing parts</button> : null}
        </div>
      ) : null}
      <div className="jhead">
        <div className="jtitle">
          <h1>{m.title}</h1>
          <div className="meta">
            {metaLine}
            {url && safeHref(url) ? <> · <a href={safeHref(url)} target="_blank" rel="noopener noreferrer">JD ↗</a></> : null}
          </div>
          {nb ? (
            <div className="next">
              <span style={{ fontSize: 22 }}>➜</span>
              <div>
                <small>Your best next step</small>
                <b>{nb.title}</b>
                <small>{aspectBy[nb.aspect].label} is your lowest-scoring aspect</small>
              </div>
              <button className="btn" style={{ marginLeft: "auto" }} onClick={() => setSel(nb.id)}>Open</button>
            </div>
          ) : (
            <div className="next"><b>All milestones complete. You&apos;re interview-ready.</b></div>
          )}
        </div>
        <div>
          <div className="rings">
            {ASPECTS.map((a) => (
              <div key={a.key} className="ring">
                <Ring value={aspectScore(j, a.key, progress)} color={a.color} />
                <b>{pct(aspectScore(j, a.key, progress))}%</b>
                <div className="lbl">{a.label}</div>
              </div>
            ))}
          </div>
          <div className="row" style={{ justifyContent: "flex-end", marginTop: 10, fontSize: 13 }}>
            <span className="muted">Overall readiness{waiting.size ? " (partial)" : ""}</span>
            <b style={{ fontSize: 20 }}>{pct(o)}%</b>
          </div>
        </div>
      </div>

      <details className="fitwrap" open={fitOpen} onToggle={(e) => setFitOpen((e.target as HTMLDetailsElement).open)}>
        <summary>Resume ↔ JD fit analysis · baseline match {j.fit.match}%</summary>
        <div className="fit">
          {([["Strengths", j.fit.strengths, "var(--good)"], ["Gaps", j.fit.gaps, "var(--warn)"], ["Risks", j.fit.risks, "var(--bad)"]] as const).map(([t, list, c]) => (
            <div key={t} className="card">
              <h4 style={{ color: c }}>{t}</h4>
              <ul>{list.map((s, i) => <li key={i}>{s}</li>)}</ul>
            </div>
          ))}
        </div>
      </details>

      <div className="mapwrap">
        {mobile ? (
          <JourneyTree j={j} progress={progress} onSelect={setSel} waiting={waiting} failed={failed} />
        ) : (
          <JourneyMap j={j} progress={progress} sel={sel} onSelect={setSel} waiting={waiting} failed={failed} />
        )}
        <div className="legend">
          <span><i />milestone order</span>
          <span><i className="d" />connected knowledge</span>
          <span>◉ pulsing = you are here (next in lane)</span>
          <span>ring = node readiness</span>
          <span>★ = key milestone</span>
        </div>
      </div>
      <div className="row" style={{ justifyContent: "space-between", marginTop: 10 }}>
        <p className="muted" style={{ fontSize: 13, margin: 0 }}>
          The four lanes run in parallel. Each node starts with credit for what your resume already shows and fills up as you finish tasks and pass its test.
        </p>
        <button
          className="btn ghost"
          style={{ fontSize: 13, padding: "6px 12px" }}
          onClick={async () => {
            if (!confirm("Delete this journey and its progress?")) return;
            await api.deleteJob(jobId).catch(() => {});
            navigate("/jobs");
          }}
        >
          Delete journey
        </button>
      </div>
      {saveError ? <div className="err">{saveError}</div> : null}

      <aside className={`panel ${sel ? "open" : ""}`} aria-hidden={!sel}>
        <button className="panel-close" onClick={() => setSel(null)} aria-label="Close">✕</button>
        {sel && nodeById[sel] ? (
          <NodePanel
            key={sel}
            n={nodeById[sel]}
            j={j}
            progress={progress}
            toggle={toggle}
            go={setSel}
            jobId={jobId}
            initialPeople={initialPeople}
            shared={shared}
            waiting={waiting.has(sel)}
            failed={failed.includes(sel)}
            onFill={onFill}
            building={building}
          />
        ) : null}
      </aside>
      <div className={`scrim ${sel ? "open" : ""}`} onClick={() => setSel(null)} />
    </>
  );
}
