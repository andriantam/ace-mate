import { Link } from "react-router-dom";
import { useSession } from "../../session";
import { Contact, LegalDoc } from "./LegalDoc";

// AceMate's Terms of Service. Operators: read the note at the top of LegalDoc.tsx before
// running a public copy.

export function Terms() {
  const { session } = useSession();
  if (!session) return null;
  const l = session.legal;
  return (
    <LegalDoc title="AceMate Terms of Service" legal={l}>
      <h2>1. Agreement</h2>
      <p>
        These terms are an agreement between you and {l.operator || "the operator of this site"}, who runs this AceMate site. By using it you agree to them and to our{" "}
        <Link to="/privacy">Privacy Policy</Link>. If you don&apos;t agree, don&apos;t use the site.
      </p>

      <h2>2. The service</h2>
      <p>
        AceMate uses AI to turn your resume, other documents and the jobs you add into interview-preparation journeys. We may change, suspend or stop any part of
        it at any time.
      </p>

      <h2>3. Your account</h2>
      <p>
        You must be at least {l.minAge} and sign in with your own Google or Okta account. You are responsible for what happens under your account, so keep your
        sign-in account secure.
      </p>

      <h2>4. Your content</h2>
      <p>
        You keep ownership of everything you upload or write. You give us permission to store and process it only to provide AceMate to you, including sending
        it to the AI model shown before you send anything. Only upload your own documents, or content you have the right to share, and don&apos;t upload other
        people&apos;s personal data without their permission.
      </p>

      <h2>5. AI output</h2>
      <p>
        AI output can be wrong, out of date or incomplete, and facts marked &quot;verify&quot; need checking. It is not career, legal, financial or immigration
        advice, and it doesn&apos;t guarantee an interview, an offer or any other outcome. Check anything important yourself.
      </p>
      <p>
        When you prepare for the people you may meet, use only public information, follow the law and each platform&apos;s rules, and never harass or pressure
        anyone.
      </p>

      <h2>6. Other services</h2>
      <p>
        Sign-in providers, AI providers and the websites you link to have their own terms, and we are not responsible for them. The &quot;Secure&quot; and
        &quot;Not secure&quot; labels are our summary of a provider&apos;s terms at the time, not a guarantee.
      </p>

      <h2>7. Acceptable use</h2>
      <p>
        Don&apos;t use AceMate for anything unlawful, try to reach other people&apos;s data or break its security, overload it or scrape it with automated tools,
        or use it to fetch pages you aren&apos;t allowed to access.
      </p>

      <h2>8. Ending your use</h2>
      <p>
        You can delete your account at any time on the <Link to="/account">Your data</Link> page. We may suspend or close accounts that break these terms or put
        the service or other users at risk.
      </p>

      <h2>9. No warranty</h2>
      <p>AceMate is provided &quot;as is&quot; and &quot;as available&quot;, without warranties of any kind, to the extent the law allows.</p>

      <h2>10. Liability</h2>
      <p>
        To the extent the law allows, we are not liable for indirect or consequential loss, or for decisions you make based on AceMate. Nothing in these terms
        limits liability that the law does not allow us to limit.
      </p>

      <h2>11. Law</h2>
      <p>These terms are governed by the laws of {l.jurisdiction}.</p>

      <h2>12. Changes and contact</h2>
      <p>
        If we change these terms in a meaningful way, we will ask you to accept the new version. Questions: <Contact legal={l} />.
      </p>
    </LegalDoc>
  );
}
