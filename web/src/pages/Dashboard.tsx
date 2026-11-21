import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api";
import { fmtSpan, type GNode } from "../components/graph/layout";
import { ProfileGraph } from "../components/graph/ProfileGraph";
import { Timeline } from "../components/graph/Timeline";
import { useDocs } from "../docs";
import { useSession } from "../session";
import type { Dashboard as DashboardData, Profile } from "../types";
import { Avatar } from "./Account";
import { ProfileTree } from "../components/ProfileTree";
import { useMobile } from "../useMobile";

const EMPTY: Profile = { displayName: "", headline: "", location: "", workRights: [], stacks: [], industries: [], flags: [], experience: [] };

// The profile dashboard: a map of the person, their career timeline and their background.
export function Dashboard() {
  const { session } = useSession();
  const { docs, version, setOpen, busy } = useDocs();
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<GNode | null>(null);
  const [year, setYear] = useState<number | null>(null);
  const mobile = useMobile();

  useEffect(() => {
    api.dashboard().then(setData, (e) => setError(e.message));
  }, [version]);
  // A new version of the profile clears the selection (adjusted during render).
  const [seen, setSeen] = useState(version);
  if (seen !== version) {
    setSeen(version);
    setSelected(null);
  }

  const user = session?.user;
  const profile = data?.profile?.data ?? EMPTY;
  const name = user?.name || profile.displayName || "You";
  const roles = useMemo(() => profile.experience ?? [], [profile]);

  if (error) return <div className="narrow card err">{error}</div>;
  if (!data || !user) return null;


  const background = (
        <section className="glass">
          <div className="row" style={{ gap: 12, alignItems: "center" }}>
            <Avatar name={name} image={user.image} size={46} />
            <div style={{ minWidth: 0 }}>
              <div className="muted upper">Your background</div>
              <b className="pd-name">{name}</b>
            </div>
          </div>
          {data.profile ? (
            <>
              <p className="pd-headline">{profile.headline}</p>
              {profile.location || profile.workRights.length ? (
                <p className="muted" style={{ fontSize: 12.5, margin: "0 0 10px" }}>
                  {profile.location}
                  {profile.workRights.length ? `${profile.location ? " · " : ""}Work rights: ${profile.workRights.join(", ")}` : ""}
                </p>
              ) : null}
              <div className="chips">
                {profile.stacks.map((s) => (
                  <span key={s.name} className={`chip ${s.depth}`} title={`${s.depth}, about ${s.years} years${s.lastUsed ? `, last used ${s.lastUsed}` : ""}`}>
                    {s.name}
                  </span>
                ))}
              </div>
            </>
          ) : (
            <p className="muted" style={{ fontSize: 13 }}>Nothing here yet. Add a document to build your background.</p>
          )}
          <div className="row" style={{ marginTop: 14, justifyContent: "space-between" }}>
            <button className="btn" onClick={() => setOpen(true)}>{busy ? `Add document · ${busy} processing` : "Add document"}</button>
            {docs.length ? (
              <button type="button" className="linkbtn" onClick={() => setOpen(true)}>
                {docs.length} document{docs.length === 1 ? "" : "s"}
              </button>
            ) : null}
          </div>
        </section>
  );

  if (mobile) {
    return (
      <div className="pmobile">
        {background}
        {data.profile ? (
          <ProfileTree profile={profile} docs={docs} />
        ) : (
          <section className="glass" style={{ textAlign: "center" }}>
            <b>Map your experience</b>
            <p className="muted" style={{ fontSize: 13, marginBottom: 0 }}>Add your resume, your GitHub profile or your personal site. Each one adds to your profile.</p>
          </section>
        )}
        {data.profile ? (
          <p className="muted" style={{ fontSize: 12, textAlign: "center" }}>
            Ready to prepare? <Link to="/jobs">Go to Jobs Applied</Link>
          </p>
        ) : null}
      </div>
    );
  }

  return (
    <div className="pdash">
      <Timeline
        roles={roles}
        year={year}
        onYear={(y) => {
          setYear(y);
          setSelected(null);
        }}
      />

      <div className="pd-canvas">
        <ProfileGraph profile={profile} docs={docs} name={name} image={user.image} selected={selected} onSelect={setSelected} year={year} />
        {!data.profile ? (
          <div className="pd-empty">
            <b>Map your experience</b>
            <p>Add your resume, your GitHub profile or your personal site. Each one adds to this map.</p>
            <div className="row" style={{ justifyContent: "center" }}>
              <button className="btn" onClick={() => setOpen(true)}>Add document</button>
            </div>
          </div>
        ) : null}
        {year !== null ? (
          <button type="button" className="year-chip" onClick={() => setYear(null)}>
            Showing {year} · clear
          </button>
        ) : null}
      </div>

      <aside className="pd-side">
        {background}

        <Detail node={selected} profile={profile} hasProfile={Boolean(data.profile)} />
        {data.profile ? (
          <p className="muted" style={{ fontSize: 12, textAlign: "center" }}>
            Ready to prepare? <Link to="/jobs">Go to Jobs Applied</Link>
          </p>
        ) : null}
      </aside>
    </div>
  );
}

function Detail({ node, profile, hasProfile }: { node: GNode | null; profile: Profile; hasProfile: boolean }) {
  if (!hasProfile) return null;
  if (!node || node.kind === "me") {
    const plus = profile.flags.filter((f) => f.kind === "plus");
    const risk = profile.flags.filter((f) => f.kind === "risk");
    return (
      <section className="glass">
        <div className="muted upper">What interviewers will notice</div>
        {plus.length ? <ul className="flaglist plus">{plus.map((f, i) => <li key={i}>{f.text}</li>)}</ul> : null}
        {risk.length ? <ul className="flaglist risk">{risk.map((f, i) => <li key={i}>{f.text}</li>)}</ul> : null}
        {!plus.length && !risk.length ? <p className="muted" style={{ fontSize: 13 }}>Nothing flagged yet.</p> : null}
        <p className="muted" style={{ fontSize: 12, marginBottom: 0 }}>Select a node on the map to see its details. Pick a year on the left to see that period.</p>
      </section>
    );
  }
  const title = { role: "Role", skill: "Skill", industry: "Industry", source: "Source", leaf: "Highlight", me: "" }[node.kind];
  return (
    <section className={`glass detail ${node.kind}`}>
      <div className="muted upper">{title}</div>
      <b className="pd-name" style={{ fontSize: 16 }}>{node.label}</b>
      {node.role ? (
        <>
          <div className="muted" style={{ fontSize: 13 }}>
            {node.role.company}
            {fmtSpan(node.role) ? ` · ${fmtSpan(node.role)}` : ""}
            {node.role.industry ? ` · ${node.role.industry}` : ""}
          </div>
          {node.role.highlights.length ? <ul className="flaglist">{node.role.highlights.map((h, i) => <li key={i}>{h}</li>)}</ul> : null}
          {node.role.skills.length ? (
            <div className="chips" style={{ marginTop: 8 }}>{node.role.skills.map((s) => <span key={s} className="chip">{s}</span>)}</div>
          ) : null}
        </>
      ) : null}
      {node.stack ? (
        <p style={{ fontSize: 13 }}>
          <span className={`chip ${node.stack.depth}`}>{node.stack.depth}</span> about {node.stack.years} year{node.stack.years === 1 ? "" : "s"}
          {node.stack.lastUsed ? `, last used ${node.stack.lastUsed}` : ""}.
          <span className="muted" style={{ display: "block", fontSize: 12, marginTop: 6 }}>Years and depth are the AI&apos;s estimates from your documents.</span>
        </p>
      ) : null}
      {node.doc ? (
        <div style={{ fontSize: 13 }}>
          <div className="muted">
            {node.doc.kind === "link" ? "Website" : node.doc.kind.toUpperCase()} · added {new Date(node.doc.createdAt).toLocaleDateString()}
          </div>
          {node.doc.url ? <a href={node.doc.url} target="_blank" rel="noopener noreferrer">{node.doc.url}</a> : null}
          {node.doc.verification && node.doc.verification.methods.length > 1 ? (
            <p className="muted" style={{ fontSize: 12 }}>Read two ways, {node.doc.verification.agreement}% agreement.</p>
          ) : null}
          {node.doc.extracted?.stacks.length ? (
            <div className="chips" style={{ marginTop: 8 }}>{node.doc.extracted.stacks.map((s) => <span key={s.name} className="chip">{s.name}</span>)}</div>
          ) : null}
        </div>
      ) : null}
      {node.kind === "leaf" && node.text ? <p style={{ fontSize: 13 }}>{node.text}</p> : null}
    </section>
  );
}
