## What and why

<!-- What does this change, and why is it needed? Link an issue if there is one. -->

## Checklist

- [ ] `make fmt` and `make test` pass
- [ ] I added or updated tests for the change
- [ ] I added a line under *Unreleased* in `CHANGELOG.md`
- [ ] No secrets, API keys or real personal data (resumes, emails) in code, tests or screenshots
- [ ] Queries on user data go through `db.WithUser`, and background work goes through the task queue
- [ ] Database changes are a new numbered file in `migrations/` (no edits to shipped ones)
- [ ] If I changed the Terms or Privacy Policy, I bumped `Version` and `Effective` in `internal/legal`
