BINARY  := claude-review
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BUILD_DIR := dist

.PHONY: build clean test lint install release-darwin-amd64 release-darwin-arm64 release-linux-amd64 release-linux-arm64 release-windows-amd64 release-all

build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/claude-review

install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	@echo "Installed $(BINARY) to /usr/local/bin"

test:
	go test ./...

lint:
	go vet ./...
	@which staticcheck > /dev/null && staticcheck ./... || echo "staticcheck not installed, skipping"

clean:
	rm -rf $(BUILD_DIR)

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
