import { useState } from "react";
import { Link } from "react-router-dom";
import { api, ApiError } from "../api";
import { useSession } from "../session";

// Shown after sign-in until the current Terms and Privacy Policy are accepted.
// Marketing is a separate, optional choice that starts unticked.
export function Consent() {
  const { session, refresh } = useSession();
  const [agree, setAgree] = useState(false);
  const [marketing, setMarketing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  if (!session?.user) return null;
  const l = session.legal;
  const name = session.user.givenName || session.user.name;

  return (
    <div className="narrow" style={{ maxWidth: 560, marginTop: 40 }}>
      <div className="card">
        <h1 style={{ marginTop: 0, fontSize: 24 }}>{name ? `Welcome, ${name}` : "Welcome"}</h1>
        <p className="muted" style={{ fontSize: 14 }}>Before you start, please read and accept how this works.</p>
        <ul style={{ fontSize: 14, paddingLeft: 18 }}>
          <li>We store your sign-in details, the AI summary of your resume and what you enter. Your resume file is not stored.</li>
          <li>Your resume is read by an AI model run by another company. You&apos;ll see which model, and whether it&apos;s secure, before every upload.</li>
          <li>Your data is stored on servers hosted by {l.hosting}. Only you can see it, and you can export or delete it at any time.</li>
        </ul>
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            if (!agree) return;
            setBusy(true);
            setError("");
            try {
              await api.acceptTerms(l.version, marketing);
              await refresh();
            } catch (err) {
              setError(err instanceof ApiError ? err.message : "Something went wrong.");
              setBusy(false);
            }
          }}
        >
          <label className="tick">
            <input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} required />
            <span>
              I have read and agree to the{" "}
              <Link to="/terms" target="_blank" rel="noopener">Terms of Service</Link> and{" "}
              <Link to="/privacy" target="_blank" rel="noopener">Privacy Policy</Link>, and I am at least {l.minAge}.
            </span>
          </label>
          <label className="tick" style={{ marginTop: 10 }}>
            <input type="checkbox" checked={marketing} onChange={(e) => setMarketing(e.target.checked)} />
            <span>
              Email me product updates and offers from {l.operator || "this site"}. <span className="muted">Optional. You can change this any time.</span>
            </span>
          </label>
          {error ? <div className="err">{error}</div> : null}
          <div className="row" style={{ marginTop: 16, justifyContent: "space-between" }}>
            <button
              type="button"
              className="btn ghost"
              onClick={async () => {
                await api.logout().catch(() => {});
                await refresh();
              }}
            >
              Sign out
            </button>
            <button className="btn" disabled={!agree || busy}>{busy ? "Saving…" : "Agree and continue"}</button>
          </div>
        </form>
      </div>
    </div>
  );
}
