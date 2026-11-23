# Changelog

All notable changes to AceMate are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[calendar versioning](https://calver.org) (`YEAR.MAJOR.MINOR`: see the README).

## [Unreleased]

### Added

- Choose the AI model for each document upload and each job application. Set `ANTHROPIC_API_KEY` and `OPENROUTER_API_KEY` + `OPENROUTER_MODEL` and a picker appears, with each model's Secure / Not secure label. The pick is stored with the task and the job, so retries use the same model. Migration `0003` adds the columns.
- The server logs which model reads each document.

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

[Unreleased]: https://github.com/andriantam/ace-mate/compare/v27.0.1...HEAD
[27.0.1]: https://github.com/andriantam/ace-mate/compare/v27.0.0...v27.0.1
[27.0.0]: https://github.com/andriantam/ace-mate/releases/tag/v27.0.0
