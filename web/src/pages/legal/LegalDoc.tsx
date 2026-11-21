import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import type { Legal } from "../../types";

// The frame shared by the Terms of Service and the Privacy Policy.
//
// FOR ANYONE RUNNING A PUBLIC COPY OF ACEMATE: these documents describe AceMate as it
// ships, filled with your LEGAL_* settings. They are a starting point, not legal advice.
// Before you open your site to the public, have them reviewed for your jurisdiction, and
// change them whenever you change what the app collects, where it runs, or who processes
// data (another AI provider, analytics, email). Then bump Version in internal/legal so
// every user is asked to accept the new version. See docs/legal-checklist.md.

export function LegalDoc({ title, legal, children }: { title: string; legal: Legal; children: ReactNode }) {
  const date = new Date(`${legal.effective}T00:00:00`);
  const effective = Number.isNaN(date.getTime()) ? legal.effective : date.toLocaleDateString(undefined, { day: "numeric", month: "long", year: "numeric" });
  return (
    <article className="narrow card legal">
      <h1>{title}</h1>
      <p className="muted">
        Effective {effective} · version {legal.version}
      </p>
      {!legal.contact ? (
        <p className="warnline">
          This site hasn&apos;t set its legal details yet (LEGAL_OPERATOR, LEGAL_CONTACT_EMAIL, LEGAL_HOSTING), so parts of this document are incomplete. If you
          run this site, see docs/legal-checklist.md before opening it to the public.
        </p>
      ) : null}
      {children}
      <p className="muted" style={{ marginTop: 28 }}>
        <Link to="/terms">Terms of Service</Link> · <Link to="/privacy">Privacy Policy</Link>
      </p>
    </article>
  );
}

export function Contact({ legal }: { legal: Legal }) {
  return legal.contact ? <a href={`mailto:${legal.contact}`}>{legal.contact}</a> : <>the contact address on this site</>;
}
