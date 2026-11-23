# Changelog

All notable changes to AceMate are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[calendar versioning](https://calver.org) (`YEAR.MAJOR.MINOR`: see the README).

## [Unreleased]

### Added

- Shared prep across job applications. When a journey is written, the AI compares its tasks and HR questions with your other journeys. A task that asks for the same deliverable in the same role track (frontend, backend and so on) is linked, so ticking it in one journey ticks it in all of them. A JavaScript project for a frontend role and one for a backend role stay separate. Company-specific tasks and questions are never shared.
- Interview FAQ on your profile (Profile → Interview FAQ): the generic HR questions your jobs ask ("What is your greatest weakness?", "Tell me about a conflict"), each with your own answer written once, the tip from every job that asks it, and search. A second tab lists the shared tasks by role track. Answers are kept even if you delete the jobs that asked them, and they are included in your data export.
- In a journey, shared tasks show a "Shared · N other jobs" pill, and HR questions in your FAQ show your answer with a link to edit it.

### Changed

- Upgrading keeps all existing data: the new tables are added alongside the old ones and nothing is rewritten. Journeys written before this version are linked by a background pass on the first start, and ticked tasks carry over to the other journeys they are linked to (ticks are only ever merged, never removed). "Find overlaps again" on the FAQ runs the comparison again.
- The API flow test uses `TEST_DATABASE_URL` when it is set, like the database tests.

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

[Unreleased]: https://github.com/andriantam/ace-mate/compare/v27.0.1...HEAD
[27.0.1]: https://github.com/andriantam/ace-mate/compare/v27.0.0...v27.0.1
[27.0.0]: https://github.com/andriantam/ace-mate/releases/tag/v27.0.0
