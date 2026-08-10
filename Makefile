.DEFAULT_GOAL := build

GO ?= go
ACTIONLINT_VERSION := v1.7.12
GOVULNCHECK_VERSION := v1.6.0

.PHONY: actionlint build check corpus dogfood editors-check fmt fmt-check policy-test scan-example test tidy-check vet vuln

build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/mori ./cmd/mori

dogfood: build
	./bin/mori scan --config configs/self-review.mori.json .

corpus:
	$(GO) run ./internal/cmd/corpuseval >/dev/null

fmt:
	gofmt -w cmd internal examples corpus/code

fmt-check:
	@files="$$(gofmt -l cmd internal examples corpus/code)"; \
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

editors-check:
	node --check editors/vscode/extension.js

policy-test:
	bash scripts/test-policies.sh

scan-example:
	$(GO) run ./cmd/mori scan --threshold 0.70 --cross-language-only examples/email-validation

check: fmt-check tidy-check vet test corpus build editors-check policy-test actionlint vuln
