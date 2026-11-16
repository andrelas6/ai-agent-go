# Security policy

## Reporting a vulnerability

Do not open a public issue for a security problem.

Use GitHub's private reporting instead: on the repository page, open
**Security**, then **Report a vulnerability**.

Include what you found, the steps to reproduce it, and the commit you tested.

## What is checked automatically

- `govulncheck` scans the code and its modules for known vulnerabilities on
  every pull request, on pushes to `main` and weekly.
- CodeQL analyses the Go code on the same triggers.
- Dependency review blocks pull requests that add dependencies with known
  advisories of moderate severity or higher.
- Dependabot opens weekly updates for Go modules and GitHub Actions.

Run the vulnerability scan locally with `govulncheck ./...` (see
`CONTRIBUTING.md`).
