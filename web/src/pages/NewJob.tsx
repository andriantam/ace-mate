import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError } from "../api";
import { PrivacyPill } from "../components/AddDocument";
import { useSession } from "../session";
import { ModelPicker } from "../components/ModelPicker";
import { resolveEngine, sameModel } from "../types";

export function NewJob() {
  const { session } = useSession();
  const navigate = useNavigate();
  const engine = session?.models.journeys.enabled ?? false;
  const [hasProfile, setHasProfile] = useState<boolean | null>(null);
  const [url, setUrl] = useState("");
  const [jdText, setJdText] = useState("");
  const [notes, setNotes] = useState("");
  const [choice, setChoice] = useState(""); // the model the user picked; "" is the pre-selected one
  const pick = session ? resolveEngine(session.models.journeys, choice) : undefined;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => void api.dashboard().then((d) => setHasProfile(Boolean(d.profile)), () => setHasProfile(false)), []);

  return (
    <div className="narrow">
      <h1 style={{ marginBottom: 4 }}>Add Job Application</h1>
      <p className="muted" style={{ marginTop: 0 }}>Each application becomes its own journey, built from the posting, your notes and your background.</p>
      <form
        className="card"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            const { id } = await api.createJob({ url, jdText, notes, engine: pick?.id });
            navigate(`/jobs/${id}`);
          } catch (err) {
            setError(err instanceof ApiError ? err.message : "Something went wrong.");
            setBusy(false);
          }
        }}
      >
        {!engine ? <p className="err" style={{ marginTop: 0 }}>No AI model is configured for journeys on this site (see the README).</p> : null}
        {engine && hasProfile === false ? <p className="err" style={{ marginTop: 0 }}>Add your resume or links on your profile first.</p> : null}
        <label className="f">Job application link</label>
        <input type="url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://careers.example.com/job/…" />
        <label className="f">Or paste the job description (use this if the site blocks reading)</label>
        <textarea value={jdText} onChange={(e) => setJdText(e.target.value)} placeholder="Paste the full job description…" />
        <label className="f">What do you know about this job? People, team, location, band, salary, what the recruiter said…</label>
        <textarea value={notes} onChange={(e) => setNotes(e.target.value)} placeholder="e.g. Hiring manager is…, team is highly technical, senior grade, needs work rights…" />
        {engine && session ? <ModelPicker engine={session.models.journeys} value={choice} onChange={setChoice} /> : null}
        {engine && session && pick ? (
          <p className="muted" style={{ fontSize: 12.5, margin: "14px 0 0" }}>
            ⓘ <b>{pick.displayName || pick.model}</b> <PrivacyPill privacy={pick.privacy} /> builds the journey
            {pick.host ? <> at <code>{pick.host}</code></> : null}
            {sameModel(pick, resolveEngine(session.models.documents, "")) ? ", the same model that reads your documents" : ""}. It receives the job description, your notes and your profile summary. It takes about ten AI calls, so allow a few minutes.
          </p>
        ) : null}
        <div className="row" style={{ marginTop: 16, justifyContent: "flex-end" }}>
          <button className="btn" disabled={busy || !engine || !hasProfile}>{busy ? "Starting…" : "Build my journey →"}</button>
        </div>
        {error ? <div className="err">{error}</div> : null}
      </form>
    </div>
  );
}
