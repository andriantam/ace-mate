# Contributing

Thanks for helping. Bug reports, fixes and new model providers are all welcome. Everyone taking part is expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting started

Follow *Install and set up* in the [README](README.md). For local work without OAuth, set `DEV_LOGIN=true` in `.env.local` and open `http://localhost:3000/api/auth/dev`.

Before you open a pull request, run:

```bash
make fmt
make test
```

`make test` runs `go vet`, every Go test (including the row-level-security and task-queue tests on an embedded Postgres), then ESLint, the TypeScript check and the Vitest tests for the UI. CI runs the same checks and builds the container image.

## Guidelines

- **Keep each pull request focused** on one change, and explain why it's needed.
- **Never commit secrets or real personal data.** That includes `.env.local`, API keys, and real resumes in tests. Test fixtures use placeholder text (see `internal/model/modeltest` and `web/src/test/fixtures.ts`).
- **Privacy and security come first.** Every query on user data must go through `db.WithUser` so row-level security applies. Anything that fetches a user-supplied URL must go through `internal/safefetch`.
- **Adding a model provider:** implement `llm.Provider` in `internal/llm` and add it to `llm.New`.
- **Background work** goes through the task queue (`internal/worker`, `db.Enqueue`), never a bare goroutine, so it survives restarts and runs on any instance. Return an error that says whether retrying can help (`Retryable()`), or wrap it with `worker.Permanent`.
- **Database changes** go in a new numbered file in `migrations/` (`0002_….sql`). Never edit one that has shipped.
- **Changing the Terms or Privacy Policy** (`web/src/pages/legal`): bump `Version` and `Effective` in `internal/legal` so every user is asked to accept again.
- **Releases:** add your change under *Unreleased* in [CHANGELOG.md](CHANGELOG.md). Versions use calendar versioning, `YEAR.MAJOR.MINOR` (see the README). Raise `MAJOR` when an upgrade needs action from the person running AceMate (a renamed or removed setting, a manual database step, a changed API) and `MINOR` for everything else; the first release of a new year is `YEAR.0.0`. To cut a release, move *Unreleased* under the new version in the changelog, set `Version` in `internal/version/version.go` and the default `VERSION` in `docker-compose.yml`, then tag `vYEAR.MAJOR.MINOR`.

## Reporting a security issue

Please don't open a public issue. See [SECURITY.md](SECURITY.md).
