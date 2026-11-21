import { useMemo } from "react";
import type { DocumentRow, Profile, Stack } from "../types";
import { fmtSpan, keyOf, ym, yearDensity } from "./graph/layout";

// The phone version of the profile map: the same relationships as an expandable tree.

const DEPTHS: { key: Stack["depth"]; label: string }[] = [
  { key: "strong", label: "Strong" },
  { key: "working", label: "Working" },
  { key: "basic", label: "Basic" },
  { key: "none", label: "Not yet" },
];

export function ProfileTree({ profile, docs }: { profile: Profile; docs: DocumentRow[] }) {
  const roles = useMemo(
    () => [...(profile.experience ?? [])].sort((a, b) => (ym(b.start) ?? 0) - (ym(a.start) ?? 0)),
    [profile],
  );
  const density = useMemo(() => yearDensity(roles), [roles]);
  const maxD = Math.max(0.001, ...density.map((d) => d.value));
  const plus = profile.flags.filter((f) => f.kind === "plus");
  const risk = profile.flags.filter((f) => f.kind === "risk");
  const sources = docs.filter((d) => d.status === "ready");
  const rolesFor = (industry: string) =>
    roles.filter((r) => r.industry && (keyOf(r.industry).includes(keyOf(industry)) || keyOf(industry).includes(keyOf(r.industry))));

  return (
    <div className="ptree">
      {roles.length ? (
        <details open>
          <summary>
            <span className="dotkey role" /> Career <span className="count">{roles.length}</span>
          </summary>
          {density.length > 1 ? (
            <div className="yearbars" aria-label="How much work each year held">
              {density.map((d) => (
                <div key={d.year} className="yb" title={`${d.roles} role${d.roles === 1 ? "" : "s"} in ${d.year}`}>
                  <i style={{ height: `${Math.max(6, (d.value / maxD) * 100)}%` }} />
                  <span>{String(d.year).slice(2)}</span>
                </div>
              ))}
            </div>
          ) : null}
          <ul>
            {roles.map((r, i) => (
              <li key={i}>
                <details>
                  <summary>
                    <span className="t-main">
                      <b>{r.title}</b>
                      <span className="muted">{r.company}{fmtSpan(r) ? ` · ${fmtSpan(r)}` : ""}</span>
                    </span>
                  </summary>
                  <div className="t-body">
                    {r.industry ? <div className="muted" style={{ fontSize: 12.5 }}>{r.industry}</div> : null}
                    {r.highlights.length ? <ul className="flaglist">{r.highlights.map((h, k) => <li key={k}>{h}</li>)}</ul> : null}
                    {r.skills.length ? <div className="chips" style={{ marginTop: 8 }}>{r.skills.map((s) => <span key={s} className="chip">{s}</span>)}</div> : null}
                  </div>
                </details>
              </li>
            ))}
          </ul>
        </details>
      ) : null}

      <details open={!roles.length}>
        <summary>
          <span className="dotkey skill" /> Skills <span className="count">{profile.stacks.length}</span>
        </summary>
        {DEPTHS.map(({ key, label }) => {
          const list = profile.stacks.filter((s) => s.depth === key);
          if (!list.length) return null;
          return (
            <details key={key} className="nested" open={key === "strong"}>
              <summary>{label} <span className="count">{list.length}</span></summary>
              <ul className="t-rows">
                {list.map((s) => (
                  <li key={s.name}>
                    <span>{s.name}</span>
                    <span className="muted">about {s.years} yr{s.years === 1 ? "" : "s"}{s.lastUsed ? ` · ${s.lastUsed}` : ""}</span>
                  </li>
                ))}
              </ul>
            </details>
          );
        })}
      </details>

      {profile.industries.length ? (
        <details>
          <summary>
            <span className="dotkey industry" /> Industries <span className="count">{profile.industries.length}</span>
          </summary>
          <ul className="t-rows">
            {profile.industries.map((ind) => {
              const rs = rolesFor(ind);
              return (
                <li key={ind}>
                  <span>{ind}</span>
                  <span className="muted">{rs.length ? rs.map((r) => r.company).filter((c, i, a) => a.indexOf(c) === i).join(", ") : ""}</span>
                </li>
              );
            })}
          </ul>
        </details>
      ) : null}

      {plus.length || risk.length ? (
        <details>
          <summary>
            <span className="dotkey leaf" /> What interviewers will notice <span className="count">{plus.length + risk.length}</span>
          </summary>
          {plus.length ? <ul className="flaglist plus">{plus.map((f, i) => <li key={i}>{f.text}</li>)}</ul> : null}
          {risk.length ? <ul className="flaglist risk">{risk.map((f, i) => <li key={i}>{f.text}</li>)}</ul> : null}
        </details>
      ) : null}

      {sources.length ? (
        <details>
          <summary>
            <span className="dotkey source" /> Sources <span className="count">{sources.length}</span>
          </summary>
          <ul>
            {sources.map((d) => (
              <li key={d.id}>
                <details>
                  <summary>
                    <span className="t-main">
                      <b>{d.name}</b>
                      <span className="muted">{d.kind === "link" ? "Website" : d.kind.toUpperCase()}{d.verification && d.verification.methods.length > 1 ? ` · ${d.verification.agreement}% agreement` : ""}</span>
                    </span>
                  </summary>
                  <div className="t-body">
                    {d.url ? <a href={d.url} target="_blank" rel="noopener noreferrer" style={{ fontSize: 12.5, wordBreak: "break-all" }}>{d.url}</a> : null}
                    {d.extracted?.stacks.length ? (
                      <div className="chips" style={{ marginTop: 8 }}>{d.extracted.stacks.map((s) => <span key={s.name} className="chip">{s.name}</span>)}</div>
                    ) : <p className="muted" style={{ fontSize: 12.5 }}>No skills from this source.</p>}
                  </div>
                </details>
              </li>
            ))}
          </ul>
        </details>
      ) : null}
    </div>
  );
}
