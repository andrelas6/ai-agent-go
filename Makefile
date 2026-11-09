# Local versions of the checks that CI runs (.github/workflows/ci.yml).

.PHONY: check fmt vet build test cover

# Run everything CI's `check` job runs.
check: fmt vet build test

# Fail if any file needs gofmt.
fmt:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

build:
	go build ./...

test:
	go test -race ./...

# Print per-function coverage. CI shows the same report in the job summary.
cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
