import { Link, Navigate, useSearchParams } from "react-router-dom";
import { useSession } from "../session";

export function SignIn() {
  const { session } = useSession();
  const [params] = useSearchParams();
  if (session?.user) return <Navigate to="/dashboard" replace />;
  const error = params.get("error");
  const providers = session?.providers ?? [];
  const nothing = providers.length === 0;
  return (
    <div className="narrow" style={{ maxWidth: 420, marginTop: 60 }}>
      <div className="card">
        <h1 style={{ marginTop: 0, fontSize: 24 }}>Sign in</h1>
        <p className="muted" style={{ fontSize: 14 }}>
          We only use your sign-in to keep your data private to you. We never store your provider tokens.
        </p>
        {error ? <p className="err">{error}</p> : null}
        {nothing ? (
          <p className="err">No sign-in provider is configured. Set the Google or Okta variables (see the README).</p>
        ) : null}
        {providers.map((p) => (
          // A plain link: sign-in is a full-page redirect to the identity provider.
          <a key={p.id} className="btn" href={`/api/auth/login/${p.id}`} style={{ display: "block", textAlign: "center", marginTop: 10 }}>
            Continue with {p.name}
          </a>
        ))}
        <p className="muted" style={{ fontSize: 12, marginTop: 14 }}>
          We receive your name, email, profile picture and language from your provider
          {session?.legal.birthday ? ", and your birthday if you allow it" : ""}. After signing in you&apos;ll be asked to accept our{" "}
          <Link to="/terms">Terms</Link> and <Link to="/privacy">Privacy Policy</Link>.
        </p>
      </div>
    </div>
  );
}
