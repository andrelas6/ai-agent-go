# Contributing

## Set up

- Go 1.26 or newer (the exact version is in `go.mod`).
- staticcheck, installed as a tool and not as a module dependency:

  ```bash
  go install honnef.co/go/tools/cmd/staticcheck@2026.1
  ```

## Before you push

```bash
make check   # gofmt, go vet, staticcheck, go build, go test -race: the same as CI
make cover   # per-function coverage report
```

The CI `check` job must pass before a pull request can merge.

## Rules

- Standard library only, plus the OpenRouter SDK and `github.com/google/uuid`. Do not add other modules (see `AGENTS.md`).
- Every Go test has a one-line comment right above it that says which scenario it checks.
- Go code is built test first.

## Pull requests

- Branch from `main`. `main` takes changes only through pull requests, squash merged.
- Prefix the commit subject with `feat:`, `refactor:`, `docs:` or `bugfix:`, and keep it to 60 characters or fewer.
- Fill in the pull request template.
