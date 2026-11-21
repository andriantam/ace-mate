# Changelog

All notable changes to AceMate are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[calendar versioning](https://calver.org) (`YEAR.MAJOR.MINOR`: see the README).

## [Unreleased]

## [27.1.0] - unreleased

### Changed

- **Building from source now needs Go 1.27 or newer and Node 22.13 or newer** (Node 24 and later also work). The container image now builds with Go 1.27 and Node 26.
- Updated the UI's development tools: ESLint 10, Vitest 5, jsdom 31 and TypeScript 7. TypeScript 7 does the type-checking. The linter keeps using the TypeScript 6 API through Microsoft's compatibility package (`@typescript/typescript6`), because typescript-eslint doesn't support TypeScript 7 yet.
- Updated the GitHub Actions in CI: checkout 7, setup-node 7 and setup-go 7.
- The UI test settings moved into `vite.config.ts`, and `vitest.config.ts` is gone.

### Security

- Vitest 5 replaces Vitest 3.2.7. Two of that version's test-runner dependencies (tinypool and @vitest/mocker) had published advisories, two of them rated critical. They affected development tooling only, not the app or its production dependencies. `npm audit` now reports no vulnerabilities.

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
