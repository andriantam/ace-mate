import { Link } from "react-router-dom";
import { nodeScore, pct } from "../../readiness";
import { type Journey, type JourneyNode, type People, type Progress, type SharedRef } from "../../types";
import { Md } from "./Md";
import { aspectBy, safeHref } from "./util";
import { PeopleEditor } from "./PeopleEditor";
import { Quiz, Scenario } from "./Tests";

// The detail panel for one milestone: brief, tasks, test, reading and people notes.

export function NodePanel({
  n, j, progress, toggle, go, jobId, initialPeople, shared = {}, waiting, failed, onFill, building,
}: {
  n: JourneyNode; j: Journey; progress: Progress; toggle: (item: string, done: boolean) => void; go: (id: string) => void;
  jobId: string; initialPeople: People; shared?: Record<string, SharedRef>; waiting?: boolean; failed?: boolean; onFill?: () => void; building?: boolean;
}) {
  const a = aspectBy[n.aspect];
  if (waiting) {
    return (
      <div>
        <div className="aspect" style={{ color: a.color }}>{a.label}{n.milestone ? " · ★ key milestone" : ""}</div>
        <h2>{n.title}</h2>
        {failed ? (
          <>
            <p className="err" style={{ marginTop: 6 }}>This part couldn&apos;t be written.</p>
            <p className="muted" style={{ fontSize: 13 }}>The rest of the journey is ready. Retrying writes only the missing parts.</p>
            {onFill && !building ? <button className="btn" onClick={onFill}>Retry missing parts</button> : null}
          </>
        ) : (
          <div className="row" style={{ gap: 10, marginTop: 8 }}>
            <span className="spinner" aria-hidden="true" />
            <span className="muted" style={{ fontSize: 13 }}>Being written now. It will appear here as soon as it&apos;s ready; you can work on other nodes meanwhile.</span>
          </div>
        )}
      </div>
    );
  }
  const s = nodeScore(n, progress);
  const conns = [...new Set([...n.links, ...j.nodes.filter((x) => x.links.includes(n.id)).map((x) => x.id)])]
    .map((id) => j.nodes.find((x) => x.id === id))
    .filter(Boolean) as JourneyNode[];
  const passed = Boolean(progress[`${n.id}:test`]);

  return (
    <div>
      <div className="aspect" style={{ color: a.color }}>
        {a.label}{n.milestone ? " · ★ key milestone" : ""}{n.verify ? <span className="verify">verify facts</span> : null}
      </div>
      <h2>{n.title}</h2>
      <div className="score-line">
        <div className="bar"><i style={{ width: `${pct(s)}%`, background: a.color }} /></div>
        {pct(s)}% · baseline {pct(n.baseline)}% from resume
      </div>
      <div className="brief"><div className="who">Recruiter&apos;s brief</div><Md text={n.brief} /></div>

      {n.bridge.length ? (
        <>
          <h3>Connect to what you know</h3>
          <div className="bridge">
            {n.bridge.map((b, i) => (
              <div key={i} style={{ display: "contents" }}>
                <div className="from"><Md text={b.from} /></div><div className="arr">→</div><div><Md text={b.to} /></div>
              </div>
            ))}
          </div>
        </>
      ) : null}

      {n.sections.map((sec, i) => (
        <div key={i}>
          <h3>{sec.title}</h3>
          <ol style={{ paddingLeft: 20, fontSize: 14 }}>{sec.items.map((it, k) => <li key={k} style={{ margin: "6px 0" }}><Md text={it} /></li>)}</ol>
        </div>
      ))}

      {n.project.repo ? (
        <>
          <h3>Exercise project (GitHub)</h3>
          <div className="project">
            <div className="mono">{n.project.repo}</div>
            <div className="muted" style={{ fontSize: 13, marginTop: 4 }}>{n.project.stack}</div>
            <ol>{n.project.milestones.map((ms, i) => <li key={i} style={{ margin: "5px 0" }}>{ms}</li>)}</ol>
          </div>
        </>
      ) : null}

      {n.tasks.length ? (
        <>
          <h3>Tasks</h3>
          {n.tasks.map((t, i) => {
            const key = `${n.id}:${i}`;
            const d = Boolean(progress[key]);
            return (
              <label key={key} className={`task ${d ? "done" : ""}`}>
                <input type="checkbox" checked={d} onChange={(e) => toggle(key, e.target.checked)} />
                <span>
                  <Md text={t} />
                  <SharedPill s={shared[key]} />
                </span>
              </label>
            );
          })}
        </>
      ) : null}

      {n.hrQuestions.length ? (
        <>
          <h3>The {n.hrQuestions.length} questions</h3>
          {n.hrQuestions.map((q, i) => {
            const s = shared[`${n.id}:q${i}`];
            return (
              <details key={i} className="hrq">
                <summary>{i + 1}. {q.q}{s ? <span className="pill hr" title="Asked in every job that shares it; answer it once in your FAQ">In your FAQ</span> : null}</summary>
                <p>{q.tip}</p>
                {s ? (
                  <div className="hrq-answer">
                    {s.answer ? <><b>Your answer</b><Md text={s.answer} /></> : <span className="muted">No answer yet.</span>}
                    <Link to={`/profile/faq#${s.id}`}>{s.answer ? "Edit in FAQ" : "Answer in FAQ"} →</Link>
                  </div>
                ) : null}
              </details>
            );
          })}
        </>
      ) : null}

      {n.peopleCards ? <PeopleEditor jobId={jobId} nodeId={n.id} initial={initialPeople} /> : null}

      {n.test.kind === "quiz" ? <Quiz n={n} passed={passed} onPass={() => toggle(`${n.id}:test`, true)} /> : null}
      {n.test.kind === "scenario" ? <Scenario n={n} passed={passed} setPassed={(v) => toggle(`${n.id}:test`, v)} /> : null}

      {n.reading.length ? (
        <>
          <h3>Read / watch</h3>
          <div className="links">
            {n.reading.map((r, i) => {
              const href = safeHref(r.url);
              return href ? <a key={i} href={href} target="_blank" rel="noopener noreferrer">{r.title} ↗</a> : null;
            })}
          </div>
        </>
      ) : null}

      {conns.length ? (
        <>
          <h3>Connected nodes</h3>
          <div className="conn">
            {conns.map((c) => (
              <button key={c.id} onClick={() => go(c.id)} style={{ borderColor: aspectBy[c.aspect].color }}>{c.title}</button>
            ))}
          </div>
        </>
      ) : null}
    </div>
  );
}

/** Marks a task this journey shares with other jobs: ticking it ticks them all. */
function SharedPill({ s }: { s?: SharedRef }) {
  if (!s || s.kind !== "task") return null;
  const n = s.others.length;
  const title = n ? `Also in: ${s.others.map((o) => o.title).join(", ")}. Ticking it here ticks it there too.` : "Shared task";
  return (
    <span className="pill shared" title={title}>
      {n ? `Shared · ${n} other job${n === 1 ? "" : "s"}` : "Shared"}
    </span>
  );
}
