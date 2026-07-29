.DEFAULT_GOAL := build

GO ?= go
ACTIONLINT_VERSION := v1.7.12
GOVULNCHECK_VERSION := v1.6.0

.PHONY: actionlint build check fmt fmt-check policy-test scan-example test tidy-check vet vuln

build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/mori ./cmd/mori

fmt:
	gofmt -w cmd internal examples

fmt-check:
	@files="$$(gofmt -l cmd internal examples)"; \
	if [ -n "$$files" ]; then \
		echo "$$files"; \
		exit 1; \
	fi

tidy-check:
	$(GO) mod tidy -diff

vet:
	$(GO) vet ./...

test:
	$(GO) test -race -coverprofile=coverage.out ./...

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

actionlint:
	$(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

policy-test:
	bash scripts/test-policies.sh

scan-example:
	$(GO) run ./cmd/mori scan --threshold 0.70 --cross-language-only examples/email-validation

check: fmt-check tidy-check vet test build policy-test actionlint vuln
