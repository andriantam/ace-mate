import { Link } from "react-router-dom";
import { useSession } from "../../session";
import { sameModel, type Engine } from "../../types";
import { Contact, LegalDoc } from "./LegalDoc";
import { privacyLaw, regulator } from "./law";

// AceMate's Privacy Policy. It describes exactly what this app collects and why.
// Operators: read the note at the top of LegalDoc.tsx before running a public copy.

function ModelName({ e }: { e: Engine }) {
  return (
    <>
      <b>{e.displayName || e.model}</b>, reached at <code>{e.host}</code>, which this site labels <b>{e.privacy === "secure" ? "Secure" : "Not secure"}</b>
    </>
  );
}

function Models({ m }: { m: { documents: Engine; journeys: Engine } }) {
  if (!m.documents.enabled && !m.journeys.enabled) return <>No AI model is configured on this site at the moment, so nothing is sent to one.</>;
  if (sameModel(m.documents, m.journeys)) {
    return (
      <>
        On this site both tasks use <ModelName e={m.documents} />.
      </>
    );
  }
  return (
    <>
      On this site documents are read by {m.documents.enabled ? <ModelName e={m.documents} /> : "no model (disabled)"}, and journeys are written by{" "}
      {m.journeys.enabled ? <ModelName e={m.journeys} /> : "no model (disabled)"}.
    </>
  );
}

export function Privacy() {
  const { session } = useSession();
  if (!session) return null;
  const l = session.legal;
  const law = privacyLaw(l.jurisdiction);
  const reg = regulator(l.jurisdiction);
  return (
    <LegalDoc title="AceMate Privacy Policy" legal={l}>
      <p>
        AceMate helps you prepare for job interviews. You add your resume and other documents, AceMate builds a profile of your experience, and for each job you
        apply for it writes a preparation journey. This policy explains what that involves for your personal data.
      </p>

      <h2>1. Who we are</h2>
      <p>
        {l.operator ? <>{l.operator} (&quot;we&quot;, &quot;us&quot;) runs this site and is</> : <>The operator of this site (&quot;we&quot;, &quot;us&quot;) is</>} responsible
        for your personal data under the law of {l.jurisdiction}
        {law ? ` (${law})` : ""}. For any privacy question or request, including to reach our data protection officer, contact <Contact legal={l} />.
      </p>

      <h2>2. What we collect</h2>
      <ul>
        <li>
          <b>From your sign-in provider (Google or Okta):</b> your account identifier there, your name, first and last name, email address and whether it is
          verified, profile picture link and language setting{l.birthday ? ", and your birthday if you grant Google's separate birthday permission" : ""}. We never
          see your password, and we don&apos;t keep the provider&apos;s access tokens.
        </li>
        <li>
          <b>Documents you add to your profile:</b> files (PDF, Word and text), text you paste, and links. For a link we fetch the page; for a GitHub profile or
          repository we read its public information through GitHub&apos;s API. We keep the AI&apos;s summary of each document (your roles with their dates and
          companies, skills, industries, headline, location, work rights and achievements), a report comparing the ways a PDF was read, and the document&apos;s
          name or link.
        </li>
        <li>
          <b>The original files:</b> a file you upload waits in our database only until it has been read, normally under a minute and at most a few minutes if
          the AI service is busy and we retry. It is then deleted. We don&apos;t keep the file or its full text.
        </li>
        <li>
          <b>Job applications:</b> the job links you add, the job description (fetched from the link or pasted by you), your notes, the journey written for you,
          which tasks and tests you complete, and any notes you write about the people you may meet.
        </li>
        <li>
          <b>Your choices:</b> which version of our Terms and this policy you accepted and when, and whether you opted in to marketing and when you last changed it.
        </li>
        <li>
          <b>Technical data:</b> one strictly necessary session cookie; records of background tasks and their progress, deleted an hour after each task finishes;
          and server logs (request path, status, timing and IP address) used for security and rate limiting. We use no advertising or analytics cookies.
        </li>
      </ul>

      <h2>3. How we use it</h2>
      <ul>
        <li>To provide AceMate: building your profile and journeys, scoring your readiness and saving your progress.</li>
        <li>To keep it secure and fair: sign-in, preventing abuse and enforcing usage limits.</li>
        <li>
          <b>For our own marketing, only if you opt in:</b> emails about our own products and services. You can change this at any time on the Your data page. We
          never sell your data or share it with anyone for their marketing.
        </li>
      </ul>
      <p>We make no decisions about you that have legal or similarly significant effects. AceMate&apos;s output is guidance for you alone.</p>

      <h2>4. AI processing</h2>
      <p>
        To read a document, its content (and, for PDFs, images of its pages) is sent to an AI model. To write a journey, your profile summary, the job
        description and your notes are sent to an AI model. Your sign-in details (email, picture{l.birthday ? ", birthday" : ""}) are not sent, but the
        documents you add may contain your contact details and are sent as they are. Our servers decrypt your stored data to do this (see section 7).{" "}
        <Models m={session.models} /> The model and its label are shown before you send anything.
      </p>
      <p>
        The AI provider processes your data under its own terms, and <b>how your data is handled there depends on the model</b>. Providers may keep inputs for a
        period. A model labelled &quot;Not secure&quot; may log your data or use it to train its models. A model labelled &quot;Secure&quot; runs on our own
        servers or, to our knowledge, is covered by terms that exclude training on your data. The label is our summary of the provider&apos;s terms, not a
        guarantee.
      </p>

      <h2>5. Who else receives data</h2>
      <ul>
        <li>Our hosting provider, {l.hosting}, which runs our servers and database.</li>
        <li>The AI provider described above.</li>
        <li>Your sign-in provider, when you sign in.</li>
        <li>The websites you link to, and GitHub for GitHub links: our server requests those pages, so they see our server&apos;s address, not yours.</li>
      </ul>

      <h2>6. Where your data is stored</h2>
      <p>
        Your account and everything we keep about you are stored in our database, hosted by {l.hosting}. Sign-in and AI providers may process data in other
        countries, including the United States. When personal data leaves {l.jurisdiction}, we take the steps the law requires to keep it protected to a
        comparable standard.
      </p>

      <h2>7. Security</h2>
      <p>These are the security measures AceMate uses.</p>
      <ul>
        <li>
          <b>In transit:</b> traffic between your browser and our servers uses HTTPS.
        </li>
        <li>
          <b>At rest:</b> the personal content we store is encrypted in our database (AES-256) with a key made for you alone. This covers your sign-in details
          (email, name, picture link{l.birthday ? ", birthday" : ""}), your profile, the names, links and summaries of your documents, your job applications
          (links, descriptions, notes, journeys and notes about people), and any file waiting to be read.
        </li>
        <li>
          <b>Keys:</b> your key is stored in our database in encrypted form. It is locked with a master key that is kept in the server&apos;s configuration, not
          in the database.
        </li>
        <li>
          <b>Separation of users:</b> the database is set up so that queries made for one account only reach that account&apos;s records (row-level security).
        </li>
        <li>
          <b>Retention:</b> an uploaded file is deleted once it has been read, and progress messages of background tasks an hour after a task finishes. We don&apos;t
          keep your password or your sign-in provider&apos;s tokens, and session tokens are stored only in hashed form.
        </li>
        <li>
          <b>Advertising and sales:</b> we use no advertising or analytics cookies, and we never sell your data.
        </li>
      </ul>
      <p>
        <b>What the encryption means in practice.</b>
      </p>
      <ul>
        <li>Someone who copies our database or a backup, or who connects to the database directly, sees encrypted values for the content listed above. Reading them takes the master key.</li>
        <li>Each person has their own key, and each value is tied to the record it belongs to, so a value copied to another account or record can&apos;t be decrypted there.</li>
        <li>Deleting your account deletes your key from our live database. Backups are covered in section 8.</li>
      </ul>
      <p>
        <b>What it doesn&apos;t mean.</b> This is not end-to-end encryption. Our servers decrypt your data to work for you, for example to show it to you or to send
        it to the AI model (section 4), so the people who run AceMate and hold the master key could technically read it. The encryption applies to data stored in
        our database. It does not apply to what is sent to the AI provider, which handles it under its own terms.
      </p>
      <p>
        <b>What is not encrypted.</b> Job titles and company names, the type and status of items, dates, error messages, your language setting, whether your email
        is verified, your consent choices, progress messages of background tasks, and server logs are stored readable.
      </p>
      <p>
        No system is perfectly secure. If a breach is likely to cause you significant harm, we will tell you and the regulator as the law requires.
      </p>

      <h2>8. How long we keep it</h2>
      <p>
        We keep your data while your account exists. Removing a document removes what it added to your profile. Deleting a job deletes its journey, progress and
        notes. Deleting your account removes everything above, and your encryption key, from our live database at once. Backups kept by our hosting provider may
        hold copies (and the keys that go with them) for a limited time until they expire, and server logs are kept for a limited period for security.
      </p>

      <h2>9. Your rights</h2>
      <p>
        You can do the following yourself on the <Link to="/account">Your data</Link> page (in the menu under your name):
      </p>
      <ul>
        <li><b>Access:</b> download everything we hold about you as a JSON file.</li>
        <li><b>Deletion:</b> delete your account and all of its data, at any time.</li>
        <li><b>Marketing:</b> opt in or out, at any time.</li>
      </ul>
      <ul>
        <li><b>Correction:</b> remove a document and add a corrected one, or edit your notes. Contact us for anything else.</li>
        <li><b>Withdrawing consent:</b> stop using AceMate and delete your account.</li>
        <li>
          <b>Complaints:</b> contact us first. You may also complain to {reg ?? "your data-protection regulator"}.
        </li>
      </ul>

      <h2>10. Age</h2>
      <p>AceMate is not for anyone under {l.minAge}.</p>

      <h2>11. Changes</h2>
      <p>If we change this policy in a meaningful way, we will ask you to review and accept the new version before you continue.</p>
    </LegalDoc>
  );
}
