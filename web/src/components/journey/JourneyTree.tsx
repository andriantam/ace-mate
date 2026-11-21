import { aspectScore, isDone, nextIn, nodeScore, pct } from "../../readiness";
import { ASPECTS, type Journey, type Progress } from "../../types";

/** The phone version of the journey map: each lane is an expandable list of its milestones. */
export function JourneyTree({
  j, progress, onSelect, waiting, failed,
}: { j: Journey; progress: Progress; onSelect: (id: string) => void; waiting: Set<string>; failed: string[] }) {
  return (
    <div className="jtree">
      {ASPECTS.map((a) => {
        const nodes = j.nodes.filter((n) => n.aspect === a.key);
        if (!nodes.length) return null;
        const next = nextIn(j, a.key, progress);
        const done = nodes.filter((n) => !waiting.has(n.id) && isDone(n, progress)).length;
        return (
          <details key={a.key} open>
            <summary style={{ borderLeftColor: a.color }}>
              <span className="lane" style={{ color: a.color }}>{a.label}</span>
              <span className="muted">{done}/{nodes.length} done · {pct(aspectScore(j, a.key, progress))}%</span>
            </summary>
            <ol>
              {nodes.map((n) => {
                const ph = waiting.has(n.id);
                const bad = failed.includes(n.id);
                const s = ph ? 0 : nodeScore(n, progress);
                const fin = !ph && isDone(n, progress);
                return (
                  <li key={n.id}>
                    <button type="button" className={`jrow ${ph ? "ph" : ""} ${next?.id === n.id ? "here" : ""}`} onClick={() => onSelect(n.id)}>
                      <span className={`jdot ${fin ? "fin" : ""} ${bad ? "bad" : ""}`} style={{ borderColor: a.color, background: fin ? a.color : undefined }}>
                        {fin ? "✓" : bad ? "!" : ph ? "…" : ""}
                      </span>
                      <span className="jt">
                        <span className="jtitle-row">
                          {n.title}
                          {n.milestone ? <span style={{ color: a.color }}> ★</span> : null}
                        </span>
                        {next?.id === n.id ? <span className="you" style={{ color: a.color }}>You are here</span> : null}
                        {ph ? (
                          <span className="muted" style={{ fontSize: 12 }}>{bad ? "Couldn't be written" : "Being written…"}</span>
                        ) : (
                          <span className="jbar"><i style={{ width: `${pct(s)}%`, background: a.color }} /></span>
                        )}
                      </span>
                      <span className="jpct">{ph ? "" : `${pct(s)}%`}</span>
                    </button>
                  </li>
                );
              })}
            </ol>
          </details>
        );
      })}
    </div>
  );
}
