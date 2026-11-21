# Legal checklist before launch

The Terms of Service and Privacy Policy in `web/src/pages/legal` describe AceMate as it ships. They are a starting point, not legal advice. If you run a public copy, you are the operator: have a lawyer in your jurisdiction review them, change them to match how you run the app, and check each item below. Whenever you change them materially, bump `Version` and `Effective` in `internal/legal` so every user is asked to accept the new version.

## Must do

- **Fill in your details.** Set `LEGAL_OPERATOR`, `LEGAL_CONTACT_EMAIL`, `LEGAL_HOSTING` and `LEGAL_JURISDICTION`. Production refuses to start without a contact email.
- **Name a data protection officer (Singapore PDPA).** Organisations must designate one and publish a business contact for them. The policy points to `LEGAL_CONTACT_EMAIL`.
- **List your sub-processors accurately.** The policy names your host and the configured AI model. If you add a separate database host, an email provider or analytics, update the policy and bump `Version`.
- **Check the regulator line.** The policy names the data-protection law and regulator for Singapore, the UK, Australia, New Zealand and Canada, and uses generic wording elsewhere (`web/src/pages/legal/law.ts`). For the EU, name your lead supervisory authority and add the GDPR's legal bases and rights.
- **Uploaded files wait in the database** until they've been read (normally under a minute, a few minutes if retried), and are then deleted. The policy says so. If you change the queue, keep that promise or change the policy.
- **Breach notification.** Under the PDPA a notifiable breach must be reported to the PDPC within 3 calendar days of assessing it, and to affected people. Have a plan.

## Marketing

- "Used only for our own marketing" can't be the whole story: the data is also needed to run the service, and the policy says so.
- **Marketing consent is a separate, optional, unticked box.** Bundling it into acceptance of the Terms is not valid consent under the GDPR, and the PDPA's consent and Do Not Call rules apply to marketing messages.
- Record when consent was given or withdrawn (stored as `marketing_opt_in` and `marketing_updated_at`), and include an unsubscribe link in every marketing email.

## Resumes and AI providers

- **Resumes are personal data**, often including contact details, work history and sometimes nationality or age. They are sent to a third-party AI provider: the provider is your sub-processor.
- **"Not secure" models** (free tiers, unverified hosts) may log or train on inputs. Don't send real resumes to them in production, or make that very clear to users. The upload box shows the model and the pill every time.
- **"Secure" is a claim you make.** Only label a provider secure after reading its current data terms (retention period, training, region). Wording that overstates protection can be misleading under consumer-protection law.
- **Cross-border transfers.** Most AI providers process data in the US. The PDPA's transfer limitation obligation requires comparable protection (for example contractual clauses in the provider's terms).

## Profile data from Google

- **Data minimisation and purpose limitation** (PDPA and GDPR): collect only what the service needs. Name, email and picture are needed to show the account. **Birthday is not used by the app**, which is why `AUTH_GOOGLE_BIRTHDAY` is off by default.
- **Age and hiring.** Storing birthdays in a job-preparation product invites claims of age-based treatment. If you do collect it, never feed it to the AI or use it in any assessment.
- **Google's policies.** The birthday scope is sensitive: production apps need Google's OAuth verification, a privacy policy that discloses it, and a justification. Google's API Services User Data Policy (Limited Use) restricts using that data for advertising.
- Gender, phone numbers and addresses are also sensitive scopes, and this app deliberately does not request them.

## Other risks

- **Minimum age.** The default is 18 (`LEGAL_MIN_AGE`). Collecting data from children brings extra obligations in most countries.
- **AI output accuracy.** The Terms disclaim it, but avoid marketing that promises interview success.
- **Researching interviewers.** The app suggests researching people from public sources. The Terms forbid harassment, and you should keep "people" notes private to each user (they are).
- **Fetching job postings.** The server fetches links users add. Some job sites forbid automated access in their terms. Fetching one page on a user's request is low risk, but don't build bulk scraping on it.
- **Backups and deletion.** Account deletion is immediate in the live database. Check your host's backup retention and state it in the policy.
- **Logs.** Request logs include IP addresses (personal data in many regimes). Set a retention period at your host.
- **Cookies.** Only a strictly necessary session cookie is used, so no cookie banner is needed. Adding analytics changes that.
