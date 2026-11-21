import { Link, Navigate } from "react-router-dom";
import { useSession } from "../session";

export function Landing() {
  const { session } = useSession();
  if (session?.user) return <Navigate to="/dashboard" replace />;
  return (
    <div className="hero">
      <h1>Walk into every interview like you already work there.</h1>
      <p>
        Upload your resume and the jobs you&apos;re applying for. Each application gets its own journey through the tech stack,
        the industry, the people and the HR questions, with a readiness score that moves as you prepare.
      </p>
      <p style={{ marginTop: 26 }}><Link to="/signin" className="btn">Get started</Link></p>
      <div className="grid3">
        <div className="card"><b>Stack, bridged</b><p className="muted" style={{ fontSize: 14 }}>New stacks explained in terms of the ones you already know, plus a GitHub capstone project.</p></div>
        <div className="card"><b>Industry &amp; people</b><p className="muted" style={{ fontSize: 14 }}>How the business works, &quot;what would you do&quot; scenarios, and research tasks to find out who you&apos;ll face.</p></div>
        <div className="card"><b>Private by default</b><p className="muted" style={{ fontSize: 14 }}>Your data is visible only to you, enforced in the database. Export or delete it at any time.</p></div>
      </div>
    </div>
  );
}
