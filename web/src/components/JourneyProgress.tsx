import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api, ApiError, type JobEvent } from "../api";
import { freshRun, retryLine, type Run } from "../progress";
import { useSession } from "../session";
import { sameModel } from "../types";
import { PrivacyPill, RunBar } from "./AddDocument";

const STAGES = ["Reading the job", "Assessing your fit", "Writing your journey", "Saving", "Done"] as const;
const LANE: Record<string, string> = { tech: "tech stack", industry: "industry", people: "the people", hr: "HR questions" };

/** Turns real journey-building events into progress. */
function advance(r: Run, ev: JobEvent, model: string): Run {
  const n: Run = { ...r, log: [...r.log] };
  const say = (m: string) => {
    n.message = m;
    n.log.push(m);
  };
  if (ev.type === "done") {
    n.stage = 4;
    n.target = n.ceiling = 100;
    n.done = true;
    say("Done. Opening your journey…");
    return n;
  }
  if (ev.type === "error") {
    n.done = true;
    n.error = ev.error;
    say(ev.error);
    return n;
  }
  switch (ev.step) {
    case "retrying":
      say(retryLine(ev));
      break;
    case "queued":
      n.target = 2;
      n.ceiling = 6;
      say("Queued. Starting shortly…");
      break;
    case "fetch":
      n.target = 6;
      n.ceiling = 12;
      say("Reading the job posting…");
      break;
    case "fetched":
      n.target = 12;
      n.ceiling = 14;
      say(`Job description ready (${Math.round((ev.chars ?? 0) / 1000)}k characters).`);
      break;
    case "outline":
      n.stage = 1;
      n.target = 15;
      n.ceiling = 33; // one long call: the bar creeps while the model thinks
      say(`${model} is assessing your fit and outlining the journey. This is the longest single step.`);
      break;
    case "outline_done":
      n.stage = 2;
      n.target = 35;
      n.expected = ev.total ?? 1;
      n.finished = 0;
      n.ceiling = 35 + 58 / n.expected - 1;
      say(`Outlined ${ev.nodes ?? ""} milestones${ev.detail ? ` for ${ev.detail}` : ""}. Writing them in ${ev.total} batches…`);
      break;
    case "batch_done":
      n.stage = 2;
      n.finished = ev.done ?? r.finished + 1;
      n.expected = ev.total ?? r.expected;
      n.target = 35 + (58 * n.finished) / n.expected;
      n.ceiling = n.finished >= n.expected ? 97 : n.target + 58 / n.expected - 1;
      if (n.finished >= n.expected) n.stage = 3;
      say(`Wrote ${n.finished} of ${n.expected} batches (latest: ${LANE[ev.aspect ?? ""] ?? ev.aspect}).`);
      break;
  }
  return n;
}

export function JourneyProgress({
  jobId,
  onFinished,
  onUpdate,
  compact,
}: {
  jobId: string;
  onFinished: () => void;
  /** Called when new parts of the journey are ready to show. */
  onUpdate?: () => void;
  compact?: boolean;
}) {
  const { session } = useSession();
  const engine = session?.models.journeys;
  const shared = session ? sameModel(session.models.journeys, session.models.documents) : false;
  const model = engine?.displayName || engine?.model || "The AI model";
  const [run, setRun] = useState<Run>({ ...freshRun(), message: "Starting…" });
  const [started] = useState(() => Date.now());
  const ref = useRef(run);
  useEffect(() => {
    ref.current = run;
  });

  useEffect(() => {
    let stop = false;
    const tick = async () => {
      if (stop || ref.current.done) return;
      try {
        const res = await api.jobProgress(jobId, ref.current.next);
        let r = ref.current;
        for (const ev of res.events) r = advance(r, ev, model);
        r = { ...r, next: res.next };
        setRun(r);
        if (res.events.some((e) => e.type === "progress" && (e.step === "outline_done" || e.step === "batch_done"))) onUpdate?.();
        if (r.done) {
          window.setTimeout(onFinished, r.error ? 0 : 700);
          return;
        }
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) return;
      }
      if (!stop) window.setTimeout(tick, 1200);
    };
    void tick();
    return () => {
      stop = true;
    };
  }, [jobId, model, onFinished, onUpdate]);

  if (compact) {
    const pctDone = run.expected ? Math.round((100 * run.finished) / run.expected) : 0;
    return (
      <div className="build-strip" role="status" aria-live="polite">
        <span className="spinner" aria-hidden="true" />
        <div style={{ minWidth: 0, flex: 1 }}>
          <b>Still writing your journey</b>{" "}
          <span className="muted">
            {run.finished} of {run.expected} parts ready. Open any finished node now; the rest appear as they&apos;re written.
          </span>
          <div className="pbar" style={{ marginTop: 6 }}><i style={{ width: `${Math.max(4, pctDone)}%` }} /></div>
        </div>
        <span className="muted" style={{ fontSize: 12, whiteSpace: "nowrap" }}>
          {model} <PrivacyPill privacy={engine?.privacy} />
        </span>
      </div>
    );
  }

  return (
    <div className="narrow card journey-progress">
      <h2 style={{ margin: "0 0 4px" }}>Building your journey</h2>
      <p className="muted" style={{ marginTop: 0, fontSize: 13 }}>
        Built by <b>{model}</b> <PrivacyPill privacy={engine?.privacy} />
        {engine?.host ? <> via <code>{engine.host}</code></> : null}{shared ? ", the same model that reads your documents" : ""}.
      </p>
      <RunBar run={run} started={started} stages={STAGES} />
      {run.log.length > 1 ? (
        <details style={{ marginTop: 8 }}>
          <summary className="muted" style={{ fontSize: 12, cursor: "pointer" }}>What happened</summary>
          <ul className="steplog">{run.log.map((l, i) => <li key={i}>{l}</li>)}</ul>
        </details>
      ) : null}
      <p className="muted" style={{ fontSize: 12, marginBottom: 0 }}>
        A journey takes about ten AI calls. Free and reasoning models can need several minutes. You can leave this page: it keeps building, and it will be on{" "}
        <Link to="/jobs">Jobs Applied</Link> when it&apos;s ready.
      </p>
    </div>
  );
}
