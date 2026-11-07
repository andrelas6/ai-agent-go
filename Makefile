# Local versions of the checks that CI runs (.github/workflows/ci.yml).

.PHONY: check fmt vet staticcheck build test

# Run everything CI's `check` job runs.
check: fmt vet staticcheck build test

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

# Needs staticcheck: go install honnef.co/go/tools/cmd/staticcheck@2026.1
# Config (and the SA4006 exclusion) lives in staticcheck.conf.
staticcheck:
	staticcheck ./...

build:
	go build ./...

test:
	go test -race ./...
