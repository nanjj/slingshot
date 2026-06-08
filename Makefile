# Slingshot Makefile
#
# Targets:
#   build         go build
#   test          go test
#   install       go install
#   release       cross-compile for linux/macos/windows amd64 & arm64
#   clean         remove build artifacts
#   fmt           go fmt

BINARY   := slingshot
MODULE   := github.com/nanjj/slingshot
CMD_PATH := ./cmd/slingshot

# Go version check (requires 1.24+)
GO_VERSION := $(shell go env GOVERSION 2>/dev/null | sed 's/^go//')

ifneq (, $(filter $(shell go env GOHOSTOS), linux darwin))
  HAVE_UPX := $(shell command -v upx 2>/dev/null)
endif

# --- Build ---

.PHONY: build
build:
	go build -v $(CMD_PATH)

.PHONY: test
test:
	go test -v -count=1 ./...

.PHONY: install
install:
	go install -v $(CMD_PATH)

# --- Release: cross-compile ---

RELEASE_DIR  := _release

# Build a single release binary
define build-release
	GOOS=$(word 1,$(subst /, ,$(1))) \
	GOARCH=$(word 2,$(subst /, ,$(1))) \
	CGO_ENABLED=0 \
	go build -trimpath -ldflags="-s -w" -o $(RELEASE_DIR)/$(2) $(CMD_PATH)
	$(if $(HAVE_UPX),upx --best --lzma $(RELEASE_DIR)/$(2),true)
endef

.PHONY: release
release:
	@mkdir -p $(RELEASE_DIR)
	$(call build-release,linux/amd64,$(BINARY)-linux-amd64)
	$(call build-release,linux/arm64,$(BINARY)-linux-arm64)
	$(call build-release,darwin/amd64,$(BINARY)-darwin-amd64)
	$(call build-release,darwin/arm64,$(BINARY)-darwin-arm64)
	$(call build-release,windows/amd64,$(BINARY)-windows-amd64.exe)
	$(call build-release,windows/arm64,$(BINARY)-windows-arm64.exe)
	@echo "---"
	@echo "Release binaries in $(RELEASE_DIR)/:"
	@ls -lh $(RELEASE_DIR)/

# --- Clean ---

.PHONY: clean
clean:
	rm -rf $(RELEASE_DIR)
	rm -f $(BINARY)
	rm -f $(BINARY).exe

# --- Fmt ---

.PHONY: fmt
fmt:
	go fmt ./...
