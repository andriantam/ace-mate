# Security policy

Please report vulnerabilities privately through GitHub's **Report a vulnerability** button (Security tab), not as public issues.

Areas we treat as high priority:

- Any way for one user to read or modify another user's data
- Authentication or session bypass
- SSRF through the link fetcher (job links and profile links)
- Leaking resume contents or personal data

Run `go test ./...` to check the row-level-security, SSRF and routing guarantees locally.
