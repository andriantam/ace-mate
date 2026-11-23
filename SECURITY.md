# Security policy

Please report vulnerabilities privately through GitHub's **Report a vulnerability** button (Security tab), not as public issues.

Areas we treat as high priority:

- Any way for one user to read or modify another user's data
- Authentication or session bypass
- SSRF through the link fetcher (job links and profile links)
- Leaking resume contents or personal data
- Anything that exposes encryption keys, or lets one user's key or sealed data be read as another user's

Run `go test ./...` to check the row-level-security, encryption, SSRF and routing guarantees locally. How encryption works and what it covers is in the README under Encryption.
