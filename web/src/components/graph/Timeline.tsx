import { useMemo } from "react";
import type { Role } from "../../types";
import { yearDensity } from "./layout";

/** How much work each year held, as a wave down the left of the profile map. Pick a year to highlight it. */
export function Timeline({ roles, year, onYear }: { roles: Role[]; year: number | null; onYear: (y: number | null) => void }) {
  const data = useMemo(() => yearDensity(roles), [roles]);
  if (data.length < 2) {
    return (
      <div className="timeline empty">
        <span>Career timeline</span>
        <p>Add a resume with dates to see it.</p>
      </div>
    );
  }
  const H = 1000, W = 70;
  const max = Math.max(...data.map((d) => d.value), 0.001);
  const y = (i: number) => 70 + (i * (H - 110)) / (data.length - 1);
  const pts = data.map((d, i) => [6 + (W - 12) * (d.value / max), y(i)] as const);
  let path = `M0 ${y(0)} L${pts[0][0]} ${pts[0][1]}`;
  for (let i = 1; i < pts.length; i++) {
    const [x0, y0] = pts[i - 1], [x1, y1] = pts[i];
    const my = (y0 + y1) / 2;
    path += ` C${x0} ${my} ${x1} ${my} ${x1} ${y1}`;
  }
  const area = `${path} L0 ${y(data.length - 1)} Z`;
  const every = data.length > 16 ? 2 : 1;
  return (
    <div className="timeline" aria-label="Career timeline: how much work each year held">
      <span className="tl-title">Work density</span>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="tl-wave" aria-hidden="true">
        <defs>
          <linearGradient id="wave" x1="0" x2="1">
            <stop offset="0" stopColor="#7fe0c3" stopOpacity="0.05" />
            <stop offset="1" stopColor="#7fe0c3" stopOpacity="0.45" />
          </linearGradient>
        </defs>
        <path d={area} fill="url(#wave)" />
        <path d={path} fill="none" stroke="#9ff0d6" strokeOpacity="0.8" strokeWidth="1.4" vectorEffect="non-scaling-stroke" />
      </svg>
      <ol className="tl-years">
        {data.map((d, i) => (
          <li key={d.year} style={{ top: `${(y(i) / H) * 100}%` }}>
            {i % every === 0 || year === d.year ? (
              <button
                type="button"
                className={year === d.year ? "on" : ""}
                onClick={() => onYear(year === d.year ? null : d.year)}
                title={`${d.roles} role${d.roles === 1 ? "" : "s"} in ${d.year}`}
              >
                {d.year}
              </button>
            ) : null}
          </li>
        ))}
      </ol>
    </div>
  );
}
