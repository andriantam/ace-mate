import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api";
import { Ring } from "../components/Ring";
import { pct } from "../readiness";
import { ASPECTS, type Dashboard as DashboardData } from "../types";

// Every job the user is preparing for, with readiness per job.
export function Jobs() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    api.dashboard().then(setData, (e) => setError(e.message));
  }, []);
  // Refresh while a journey is still being built.
  useEffect(() => {
    if (!data?.jobs.some((j) => j.status === "pending")) return;
    const id = window.setTimeout(() => api.dashboard().then(setData, () => {}), 4000);
    return () => window.clearTimeout(id);
  }, [data]);

  if (error) return <div className="narrow card err">{error}</div>;
  if (!data) return null;
  const color = Object.fromEntries(ASPECTS.map((a) => [a.key, a.color]));


  return (
    <div className="page wide">
      <div className="section-title">
        <div className="row" style={{ gap: 14 }}>
          <Ring value={data.average} size={56} />
          <div>
            <h1 className="page-title" style={{ margin: 0 }}>Jobs Applied</h1>
            <div className="muted" style={{ fontSize: 13 }}>
              Average readiness {pct(data.average)}% across {data.journeys} journey{data.journeys === 1 ? "" : "s"}
            </div>
          </div>
        </div>
        <div className="row">
          <Link to="/jobs/new" className="btn">+ Add Job Application</Link>
        </div>
      </div>
      {!data.profile ? (
        <div className="card muted" style={{ marginBottom: 14 }}>
          Journeys start from your profile. <Link to="/dashboard">Add your resume or links first</Link>.
        </div>
      ) : null}
      {data.jobs.length === 0 ? (
        <div className="card muted">No job applications yet. Add one to build your first journey.</div>
      ) : (
        <div className="jobs">
          {data.jobs.map((j) => (
            <Link key={j.id} to={`/jobs/${j.id}`} className="jobcard">
              <div className="head">
                <Ring value={j.overall} size={52} />
                <div style={{ minWidth: 0 }}>
                  <b>{j.title}</b>
                  <span className="muted" style={{ fontSize: 13 }}>{j.company}</span>
                  <div style={{ marginTop: 4 }}>
                    {j.status === "pending" ? <span className="badge">analysing…</span> : null}
                    {j.status === "failed" ? <span className="badge failed">failed</span> : null}
                    {j.status === "ready" ? <span className="badge">{pct(j.overall)}% ready</span> : null}
                  </div>
                </div>
              </div>
              {j.status === "ready"
                ? j.aspects.map((a) => (
                    <div key={a.key} className="mini">
                      <span>{a.label}</span>
                      <div className="bar"><i style={{ width: `${pct(a.score)}%`, background: color[a.key] }} /></div>
                      <span>{pct(a.score)}%</span>
                    </div>
                  ))
                : null}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
