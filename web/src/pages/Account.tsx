import { useState } from "react";
import { Link } from "react-router-dom";
import { api, ApiError } from "../api";
import { useDocs } from "../docs";
import { useSession } from "../session";

// "Your data": consent choices, export and deletion.
export function Account() {
  const { session } = useSession();
  const { docs, setOpen } = useDocs();
  const user = session?.user;
  if (!user) return null;
  return (
    <div className="narrow page">
      <h1 className="page-title">Your data</h1>

      <section className="card">
        <h3>Account</h3>
        <div className="row" style={{ gap: 12 }}>
          <Avatar name={user.name} image={user.image} size={44} />
          <div>
            <b>{user.name || user.email}</b>
            <div className="muted" style={{ fontSize: 13 }}>{user.email}</div>
          </div>
        </div>
      </section>

      <section className="card">
        <h3>Privacy</h3>
        <p className="muted" style={{ fontSize: 13 }}>
          Only you can see your data. Access is enforced by row-level security in the database. Files you add are read in memory and never stored: we keep only what the AI extracted. You have {docs.length} document{docs.length === 1 ? "" : "s"} on your profile.{" "}
          <button type="button" className="linkbtn" onClick={() => setOpen(true)}>Manage documents</button>
        </p>
        <MarketingToggle />
        <p className="muted" style={{ fontSize: 12, margin: "10px 0 0" }}>
          <Link to="/privacy">Privacy Policy</Link> · <Link to="/terms">Terms of Service</Link>
        </p>
      </section>

      <section className="card">
        <h3>Export</h3>
        <p className="muted" style={{ fontSize: 13 }}>Download everything we hold about you: your account, consent choices, documents, profile and journeys.</p>
        <a className="btn ghost" href="/api/export" download>Export my data (JSON)</a>
      </section>

      <section className="card danger-zone">
        <h3>Delete account</h3>
        <DeleteAccount onDeleted={() => (window.location.href = "/")} />
      </section>
    </div>
  );
}

export function Avatar({ name, image, size = 32 }: { name: string; image: string; size?: number }) {
  const initials = name.split(/\s+/).map((w) => w[0]).filter(Boolean).slice(0, 2).join("").toUpperCase() || "?";
  return image ? (
    <img className="avatar" src={image} alt="" referrerPolicy="no-referrer" style={{ width: size, height: size }} />
  ) : (
    <span className="avatar initials" style={{ width: size, height: size, fontSize: size * 0.38 }}>{initials}</span>
  );
}

function MarketingToggle() {
  const { session, refresh } = useSession();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const on = session?.user?.marketingOptIn ?? false;
  return (
    <div>
      <label className="tick" style={{ fontSize: 13 }}>
        <input
          type="checkbox"
          checked={on}
          disabled={busy}
          onChange={async (e) => {
            setBusy(true);
            setError("");
            try {
              await api.setMarketing(e.target.checked);
              await refresh();
            } catch (err) {
              setError(err instanceof ApiError ? err.message : "Couldn't save that.");
            } finally {
              setBusy(false);
            }
          }}
        />
        <span>Email me product updates and offers (optional)</span>
      </label>
      {error ? <div className="err">{error}</div> : null}
    </div>
  );
}

function DeleteAccount({ onDeleted }: { onDeleted: () => void }) {
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  return (
    <form
      onSubmit={async (e) => {
        e.preventDefault();
        try {
          await api.deleteAccount(text);
          onDeleted();
        } catch (err) {
          setError(err instanceof ApiError ? err.message : "Something went wrong.");
        }
      }}
    >
      <p className="muted" style={{ fontSize: 13, marginTop: 0 }}>Deletes your account, documents, profile, journeys and consent record. This can&apos;t be undone.</p>
      <label className="f">Type DELETE to confirm</label>
      <div className="row">
        <input type="text" value={text} onChange={(e) => setText(e.target.value)} autoComplete="off" style={{ maxWidth: 160 }} />
        <button className="btn danger" disabled={text !== "DELETE"}>Delete everything</button>
      </div>
      {error ? <div className="err">{error}</div> : null}
    </form>
  );
}
