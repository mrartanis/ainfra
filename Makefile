GO ?= $(shell test ! -x .tools/go/bin/go || echo .tools/go/bin/go)
ifeq ($(GO),)
GO = go
endif
GOFMT = $(shell $(GO) env GOROOT)/bin/gofmt
.PHONY: build test fmt fmt-check lint test-integration test-e2e test-security test-race release release-check
build:
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/ ./cmd/...
test:
	$(GO) test ./...
fmt:
	$(GO) fmt ./...
fmt-check:
	@test -z "$$( $(GOFMT) -l $$(find cmd internal -name '*.go'))"
lint:
	$(GO) vet ./...
test-integration:
	$(GO) test -count=1 ./internal/adapters/... ./internal/apiclient/...
test-e2e:
	$(GO) test -count=1 ./internal/e2e/...
test-security:
	$(GO) test -count=1 ./internal/app/... ./internal/adapters/httpapi/... ./internal/adapters/routeros/... ./internal/ui/...
test-race:
	CGO_ENABLED=1 $(GO) test -race ./...
release:
	GO=$(GO) sh scripts/release.sh
release-check:
	sh scripts/release-check.sh

.PHONY: test-fuzz
test-fuzz:
	$(GO) test ./internal/domain/version -run '^$$' -fuzz '^FuzzCompare$$' -fuzztime=3s -parallel=2
	$(GO) test ./internal/adapters/linuxcollect -run '^$$' -fuzz '^FuzzDpkg$$' -fuzztime=3s -parallel=2
	$(GO) test ./internal/adapters/linuxcollect -run '^$$' -fuzz '^FuzzAPTParsers$$' -fuzztime=3s -parallel=2
	$(GO) test ./internal/adapters/feeds -run '^$$' -fuzz '^FuzzFeedNormalization$$' -fuzztime=3s -parallel=2
	$(GO) test ./internal/adapters/httpapi -run '^$$' -fuzz '^FuzzReportDecoding$$' -fuzztime=3s -parallel=2
	$(GO) test ./internal/adapters/routeros -run '^$$' -fuzz '^FuzzRouterOSValues$$' -fuzztime=3s -parallel=2
