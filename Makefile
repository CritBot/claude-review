BINARY        := claude-review
VERSION       := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS       := -ldflags "-X main.version=$(VERSION)"
BUILD_DIR     := dist
COVER_DIR     := coverage
COVER_PROFILE := $(COVER_DIR)/coverage.out
COVER_HTML    := $(COVER_DIR)/coverage.html
# Packages that have tests (excludes cmd, config, hooks, memory — mostly I/O bound)
TESTED_PKGS   := ./internal/agents/... ./internal/diff/... ./internal/output/...
COVER_MIN     := 40

.PHONY: build clean test coverage coverage-html coverage-check lint install \
        release-darwin-amd64 release-darwin-arm64 release-linux-amd64 \
        release-linux-arm64 release-windows-amd64 release-all

build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/claude-review

install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	@echo "Installed $(BINARY) to /usr/local/bin"

test:
	go test -race ./...

# Generate coverage profile
coverage:
	@mkdir -p $(COVER_DIR)
	go test -coverprofile=$(COVER_PROFILE) -covermode=atomic $(TESTED_PKGS)
	go tool cover -func=$(COVER_PROFILE)

# Open HTML coverage report in browser
coverage-html: coverage
	go tool cover -html=$(COVER_PROFILE) -o $(COVER_HTML)
	@echo "Coverage report: $(COVER_HTML)"
	@open $(COVER_HTML) 2>/dev/null || xdg-open $(COVER_HTML) 2>/dev/null || true

# Fail if total coverage is below COVER_MIN percent
coverage-check: coverage
	@TOTAL=$$(go tool cover -func=$(COVER_PROFILE) | awk '/^total:/ {gsub(/%/, ""); print int($$3)}'); \
	echo "Coverage: $${TOTAL}% (minimum: $(COVER_MIN)%)"; \
	if [ "$$TOTAL" -lt "$(COVER_MIN)" ]; then \
		echo "FAIL: coverage $${TOTAL}% is below minimum $(COVER_MIN)%"; \
		exit 1; \
	fi

lint:
	go vet ./...
	@which staticcheck > /dev/null && staticcheck ./... || echo "staticcheck not installed, skipping"

clean:
	rm -rf $(BUILD_DIR) $(COVER_DIR)

# Cross-compilation targets for GitHub releases
release-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./cmd/claude-review

release-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/claude-review

release-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/claude-review

release-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/claude-review

release-windows-amd64:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe ./cmd/claude-review

release-all: release-darwin-amd64 release-darwin-arm64 release-linux-amd64 release-linux-arm64 release-windows-amd64
	@echo "All release binaries built in $(BUILD_DIR)/"

# Create compressed archives for GitHub release
package: release-all
	cd $(BUILD_DIR) && \
	tar -czf $(BINARY)-darwin-amd64.tar.gz $(BINARY)-darwin-amd64 && \
	tar -czf $(BINARY)-darwin-arm64.tar.gz $(BINARY)-darwin-arm64 && \
	tar -czf $(BINARY)-linux-amd64.tar.gz $(BINARY)-linux-amd64 && \
	tar -czf $(BINARY)-linux-arm64.tar.gz $(BINARY)-linux-arm64 && \
	zip $(BINARY)-windows-amd64.zip $(BINARY)-windows-amd64.exe
	@echo "Release packages created in $(BUILD_DIR)/"
