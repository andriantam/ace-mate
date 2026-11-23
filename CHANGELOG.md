# Changelog

All notable changes to AceMate are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[calendar versioning](https://calver.org) (`YEAR.MAJOR.MINOR`: see the README).

## [Unreleased]

## [27.1.0] - 2026-11-23

### Action required when upgrading

- **Set `ENCRYPTION_KEYS` before you start this version in production.** The server refuses to start without it. Generate a key with `openssl rand -base64 32` and set `1:<key>`. Docker Compose needs it in `.env` too: its container can't keep a generated key, so without one it stops with a message saying what to set. In development, without `ENCRYPTION_KEYS`, a key is created once in `DATA_DIR/encryption.key`.
- **Keep a copy of that key somewhere safe, apart from the database and its backups.** If it is lost, encrypted data can't be recovered.
- **Back up the database before the first start.** At the first start, existing data is encrypted and the readable copy is cleared.
- **Every user is asked to accept the Terms and Privacy Policy again** at their next sign-in: the Privacy Policy is now version 3.

### Added

- Shared prep across job applications. When a journey is written, the AI compares its tasks and HR questions with your other journeys, using the model picked for that job. A task that asks for the same deliverable in the same role track (frontend, backend and so on) is linked, so ticking it in one journey ticks it in all of them. A JavaScript project for a frontend role and one for a backend role stay separate. Company-specific tasks and questions are never shared. If the model gets an entry wrong, only that item stays unshared.
- Interview FAQ on your profile (Profile → Interview FAQ): the generic HR questions your jobs ask ("What is your greatest weakness?", "Tell me about a conflict"), each with your own answer written once, the tip from every job that asks it, and search. A second tab lists the shared tasks by role track. Answers are kept even if you delete the jobs that asked them, and they are included in your data export.
- In a journey, shared tasks show a "Shared · N other jobs" pill, and HR questions in your FAQ show your answer with a link to edit it.
- Choose the AI model for each document upload and each job application. Set `ANTHROPIC_API_KEY` and `OPENROUTER_API_KEY` + `OPENROUTER_MODEL` and a picker appears, with each model's Secure / Not secure label. The pick is stored with the task and the job, so retries use the same model. Migration `0003` adds the columns.
- The server logs which model reads each document.

### Changed

- The README has a new Encryption section: how keys work, what is and isn't encrypted, what it protects against, and how to set, back up and rotate `ENCRYPTION_KEYS`. The Privacy Policy now describes encryption at rest and its limits, and the landing page no longer says data is "visible only to you".
- The Privacy Policy security section now states the facts: the measures used (HTTPS, per-user encryption at rest, row-level security, short retention), what the encryption means in practice (a copied database, a backup or a direct database connection shows encrypted values that need the master key to read), and what it doesn't (it is not end-to-end, the people who hold the master key could technically read the data, and it doesn't apply to what is sent to the AI provider). It lists what stays readable. The policy version is now 3.
- The sign-up screen no longer says "Only you can see it".

### Security

- Every user now has their own encryption key. It is created with the account, and the database stores it only encrypted by a master key held outside the database (the new `ENCRYPTION_KEYS` setting, required in production).
- Each user's personal content is now stored encrypted with their own key: the profile, every document's name, link, extracted details and cross-check, each job's link, notes, posting text, journey and person cards, and uploaded files waiting to be read. Each value is tied to its row, so a value copied elsewhere can't be decrypted. The job title and company stay readable, so the jobs list can show and sort them.
- The personal details your sign-in provider shares (email, name, photo link and birthday) are stored encrypted too.
- Shared-prep labels and your FAQ answers are encrypted with your own key, like the rest of your content. Migration `0006` adds the tables; it only adds, never rewrites. Journeys saved before this version (encrypted or not) are linked by a background pass at the first start, and ticked tasks carry over to the journeys they are linked to (ticks are only ever merged, never removed). "Find overlaps again" on the FAQ runs the comparison again.
- Data saved by an earlier version is encrypted automatically at the first start, and the readable copy is cleared. Nobody has to sign up again.
- Deleting an account deletes the user's key first.
- Master keys can be rotated: add a new version to `ENCRYPTION_KEYS` and restart. Users' keys are re-encrypted with the newest master key at startup, and existing users without a key get one.

### Fixed

- A profile row with no data (from an earlier pre-release database) no longer fails every request that reads the profile; it reads as "no profile". Migration `0002` backfills such rows with an empty profile and enforces `NOT NULL`, without deleting anything.
- The progress text for uploads and journeys now names the model the user picked, not the site default.

## [27.0.1] - 2026-11-22

### Fixed

- The API flow test gave up after about 24 seconds, which was too short for GitHub's slower runners to render PDF pages under the race detector. It now waits up to three minutes, so continuous integration no longer fails for that reason. No change to the app itself.

## [27.0.0] - 2026-11-21

First public release.

### Added

- Profile map: a graph of your roles, skills, industries, highlights and sources, with a career timeline. It drifts while idle, reacts to the pointer with focus, push and parallax, and stays still for people who prefer reduced motion. Phones get an expandable list instead.
- Documents: PDF (read from the text and from page images, then cross-checked), Word `.docx`, text and Markdown files, pasted text, GitHub profiles and repositories, and any public page.
- Journeys for each job application, with four lanes (tech stack, industry, people, HR questions), readiness scores, quizzes, scenarios and people notes. Finished parts can be used while the rest is still being written.
- Background work in a Postgres task queue: leases, recovery after a crash, a clean hand-back on shutdown, and automatic retries when the AI service is busy or unreachable.
- Any Claude model, or any OpenAI-compatible API, with a separate model per tool (`DOCS_LLM_*`, `JOURNEY_LLM_*`), and the model and its Secure / Not secure label shown before anything is sent.
- Sign-in with Google or Okta (OpenID Connect with PKCE). Row-level security in Postgres keeps each user's data separate.
- Terms of Service and Privacy Policy pages filled from `LEGAL_*` settings, consent tracking, a separate marketing opt-in, data export and account deletion.
- Health checks (`/healthz`, `/readyz`), a container image with a built-in health check, and `docker-compose.yml` with Postgres.
- ESLint and Vitest for the UI, and a CI workflow that runs the Go tests, UI lint, typecheck and tests, and builds the image.

[Unreleased]: https://github.com/andriantam/ace-mate/compare/v27.1.0...HEAD
[27.1.0]: https://github.com/andriantam/ace-mate/compare/v27.0.1...v27.1.0
[27.0.1]: https://github.com/andriantam/ace-mate/compare/v27.0.0...v27.0.1
[27.0.0]: https://github.com/andriantam/ace-mate/releases/tag/v27.0.0
