.PHONY: ci verify-format verify-modules vet test ci-build build clean

DIST_DIR ?= dist
VERSION ?= dev
COMMIT ?=
BUILD_DATE ?=

ci: verify-format verify-modules vet test ci-build

verify-format:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "The following files are not gofmt'ed:"; \
		echo "$$files"; \
		exit 1; \
	fi

verify-modules:
	go mod verify

vet:
	go vet ./...

test:
	go test -race ./...

ci-build:
	go build ./...

build: clean
	@set -eu; \
	version='$(TAG_NAME)'; \
	if [ -z "$$version" ]; then \
		version='$(VERSION)'; \
	fi; \
	commit='$(COMMIT)'; \
	if [ -z "$$commit" ]; then \
		commit='$(GITHUB_SHA)'; \
	fi; \
	build_date='$(BUILD_DATE)'; \
	if [ -z "$$commit" ]; then \
		commit="$$(git rev-parse HEAD)"; \
	fi; \
	if [ -z "$$build_date" ]; then \
		build_date="$$(date -u +%Y-%m-%dT%H:%M:%SZ)"; \
	fi; \
	mkdir -p "$(DIST_DIR)"; \
	for target in amd64 arm64; do \
		GOOS=linux GOARCH="$$target" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X stevedore-agent/internal/buildinfo.Version=$$version -X stevedore-agent/internal/buildinfo.Commit=$$commit -X stevedore-agent/internal/buildinfo.BuildDate=$$build_date" -o "$(DIST_DIR)/stevedore-agent-linux-$$target" .; \
	done; \
	if command -v sha256sum >/dev/null 2>&1; then \
		checksum_cmd='sha256sum'; \
	else \
		checksum_cmd='shasum -a 256'; \
	fi; \
	(cd "$(DIST_DIR)" && $$checksum_cmd stevedore-agent-linux-* > checksums.txt)

clean:
	rm -rf "$(DIST_DIR)"
