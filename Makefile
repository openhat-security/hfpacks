.PHONY: help build test tidy check build-release release-snapshot release

help:
	@echo "build              Go binary → bin/hfpacks"
	@echo "test               go test ./..."
	@echo "tidy               go mod tidy"
	@echo "build-release      Alias for release-snapshot"
	@echo "release-snapshot   GoReleaser snapshot (or local cross-build fallback)"
	@echo "release BUMP=patch Cut release: bump patch|minor|major, tag, push"

build:
	go build -o bin/hfpacks ./cmd/hfpacks

test:
	go test ./...

tidy:
	go mod tidy

check: test
	go vet ./...
	$(MAKE) build

build-release release-snapshot:
	bash scripts/build-release.sh

release:
	@test -n "$(BUMP)" || (echo "usage: make release BUMP=patch|minor|major [ARGS='--dry-run']" >&2; exit 1)
	bash scripts/release.sh "$(BUMP)" $(ARGS)
