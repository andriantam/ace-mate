import { aspectScore, isDone, nextIn, nodeScore, overallScore, pct } from "../../readiness";
import { ASPECTS, type Journey, type Progress } from "../../types";
import { aspectBy } from "./util";

export function JourneyMap({
  j, progress, sel, onSelect, waiting, failed,
}: { j: Journey; progress: Progress; sel: string | null; onSelect: (id: string) => void; waiting: Set<string>; failed: string[] }) {
  const laneH = 120, top = 46, left = 150, gap = 175;
  const lanes = ASPECTS.map((a) => j.nodes.filter((n) => n.aspect === a.key));
  const maxCol = Math.max(1, ...lanes.map((l) => l.length));
  const W = left + maxCol * gap + 150;
  const H = top + ASPECTS.length * laneH + 10;
  const pos: Record<string, { x: number; y: number; lane: number }> = {};
  lanes.forEach((ns, li) => ns.forEach((n, ci) => (pos[n.id] = { x: left + ci * gap, y: top + li * laneH + 40, lane: li })));
  const fin = { x: W - 95, y: top + (ASPECTS.length * laneH) / 2 };
  const nexts = new Set(ASPECTS.map((a) => nextIn(j, a.key, progress)?.id).filter(Boolean) as string[]);
  const related = new Set(sel ? [...(j.nodes.find((n) => n.id === sel)?.links ?? []), ...j.nodes.filter((n) => n.links.includes(sel)).map((n) => n.id)] : []);

  const cross: { a: string; b: string }[] = [];
  const seen = new Set<string>();
  for (const n of j.nodes)
    for (const l of n.links) {
      const k = [n.id, l].sort().join("|");
      if (seen.has(k) || !pos[l]) continue;
      seen.add(k);
      cross.push({ a: n.id, b: l });
    }

  return (
    <svg style={{ width: "100%", minWidth: 1100, height: "auto" }} viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Journey map">
      {ASPECTS.map((a, i) => {
        const y = top + i * laneH;
        return (
          <g key={a.key}>
            <rect x={8} y={y} width={W - 200} height={laneH - 12} rx={12} fill="rgba(255,255,255,.015)" stroke="var(--line)" />
            <text className="lane-label" x={22} y={y + 22} style={{ fill: a.color }}>{a.label}</text>
            <text x={22} y={y + 40} style={{ fill: "var(--muted)", fontSize: 12 }}>{pct(aspectScore(j, a.key, progress))}%</text>
          </g>
        );
      })}
      {lanes.map((ns) =>
        ns.map((n, i) => {
          const p = pos[n.id];
          const lit = isDone(n, progress) ? "edge lit" : "edge";
          if (i < ns.length - 1) {
            const q = pos[ns[i + 1].id];
            return <line key={`s-${n.id}`} className={lit} x1={p.x + 30} y1={p.y} x2={q.x - 30} y2={q.y} />;
          }
          return <path key={`s-${n.id}`} className={lit} d={`M${p.x + 30},${p.y} C${p.x + 120},${p.y} ${fin.x - 110},${fin.y} ${fin.x - 46},${fin.y}`} />;
        }),
      )}
      {cross.map(({ a, b }) => {
        const p = pos[a], q = pos[b];
        const same = p.lane === q.lane;
        const hl = sel === a || sel === b;
        const dy = same ? -30 : 0;
        return (
          <path
            key={`x-${a}-${b}`}
            className={`edge x ${hl ? "lit" : ""}`}
            style={{ opacity: hl ? 1 : 0.5, strokeWidth: hl ? 2.5 : 2 }}
            d={`M${p.x},${p.y + dy} Q${(p.x + q.x) / 2},${(p.y + q.y) / 2 + (same ? -48 : 0)} ${q.x},${q.y + dy}`}
          />
        );
      })}
      {j.nodes.map((n) => {
        const p = pos[n.id];
        const a = aspectBy[n.aspect];
        const s = nodeScore(n, progress);
        const r = 26, c = 2 * Math.PI * r;
        const isNext = nexts.has(n.id);
        const dim = sel && sel !== n.id && !related.has(n.id);
        const [l1, l2] = wrap(n.title);
        if (waiting.has(n.id)) {
          const bad = failed.includes(n.id);
          return (
            <g
              key={n.id}
              className={`node placeholder ${bad ? "failed" : "writing"} ${sel === n.id ? "sel" : ""}`}
              opacity={dim ? 0.45 : 1}
              tabIndex={0}
              role="button"
              aria-label={`${n.title}, ${bad ? "couldn't be written" : "being written"}`}
              onClick={() => onSelect(n.id)}
              onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && onSelect(n.id)}
            >
              <circle className="bg" cx={p.x} cy={p.y} r={r} stroke={bad ? "var(--bad)" : a.color} />
              {bad ? null : <circle className="spin" cx={p.x} cy={p.y} r={r} stroke={a.color} strokeDasharray={`${c * 0.22} ${c}`} />}
              <text className="pct" x={p.x} y={p.y + 4} textAnchor="middle">{bad ? "!" : "…"}</text>
              {n.milestone ? <text x={p.x + 20} y={p.y - 18} style={{ fill: a.color, fontSize: 13, opacity: 0.6 }}>★</text> : null}
              <text x={p.x} y={p.y + 44} textAnchor="middle" className="ph-label">{l1}</text>
              {l2 ? <text x={p.x} y={p.y + 58} textAnchor="middle" className="ph-label">{l2}</text> : null}
            </g>
          );
        }
        return (
          <g
            key={n.id}
            className={`node ${isNext ? "next" : ""} ${sel === n.id ? "sel" : ""}`}
            opacity={dim ? 0.45 : 1}
            tabIndex={0}
            role="button"
            aria-label={`${n.title}, ${pct(s)}% ready`}
            onClick={() => onSelect(n.id)}
            onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && onSelect(n.id)}
          >
            {isNext ? <circle className="halo" cx={p.x} cy={p.y} r={30} fill="none" stroke={a.color} strokeWidth={2} /> : null}
            <circle className="bg" cx={p.x} cy={p.y} r={r} stroke={isDone(n, progress) ? a.color : "var(--line)"} />
            <circle className="prog" cx={p.x} cy={p.y} r={r} stroke={a.color} strokeDasharray={c} strokeDashoffset={c * (1 - s)} />
            <text className="pct" x={p.x} y={p.y + 4} textAnchor="middle">{isDone(n, progress) ? "✓" : pct(s)}</text>
            {n.milestone ? <text x={p.x + 20} y={p.y - 18} style={{ fill: a.color, fontSize: 13 }}>★</text> : null}
            {isNext ? <text className="you" x={p.x} y={p.y - 36} textAnchor="middle" style={{ fill: a.color }}>YOU ARE HERE</text> : null}
            <text x={p.x} y={p.y + 44} textAnchor="middle">{l1}</text>
            {l2 ? <text x={p.x} y={p.y + 58} textAnchor="middle">{l2}</text> : null}
          </g>
        );
      })}
      <circle cx={fin.x} cy={fin.y} r={44} fill="var(--bg2)" stroke="var(--accent)" strokeWidth={2} />
      <text x={fin.x} y={fin.y - 4} textAnchor="middle" style={{ fill: "var(--text)", fontWeight: 700, fontSize: 18 }}>{pct(overallScore(j, progress))}%</text>
      <text x={fin.x} y={fin.y + 14} textAnchor="middle" style={{ fill: "var(--muted)", fontSize: 11 }}>Interview day</text>
    </svg>
  );
}

function wrap(title: string): [string, string] {
  let l1 = "", l2 = "";
  for (const w of title.split(" ")) {
    if (!l2 && (l1 + " " + w).trim().length <= 20) l1 = (l1 + " " + w).trim();
    else l2 = (l2 + " " + w).trim();
  }
  if (l2.length > 22) l2 = l2.slice(0, 21) + "…";
  return [l1, l2];
}
