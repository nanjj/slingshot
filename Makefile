# Slingshot Makefile
#
# Targets:
#   build         go build
#   test          go test
#   install       go install
#   release       cross-compile for linux/macos/windows amd64 & arm64 + gzip + sha256
#   release-gh    release + create/upload GitHub release (requires gh CLI)
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

# sha256sum command (portable across Linux & macOS)
SHA256_CMD := $(if $(filter darwin,$(shell go env GOHOSTOS)),shasum -a 256,sha256sum)

# Build a single release binary, then gzip + sha256
define build-release
	GOOS=$(word 1,$(subst /, ,$(1))) \
	GOARCH=$(word 2,$(subst /, ,$(1))) \
	CGO_ENABLED=0 \
	go build -trimpath -ldflags="-s -w" -o $(RELEASE_DIR)/$(2) $(CMD_PATH)
	$(if $(HAVE_UPX),upx --best --lzma $(RELEASE_DIR)/$(2),true)
	gzip -fk $(RELEASE_DIR)/$(2)
	cd $(RELEASE_DIR) && $(SHA256_CMD) $(2).gz > $(2).gz.sha256
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
	@echo "Release artifacts in $(RELEASE_DIR)/:"
	@ls -lh $(RELEASE_DIR)/

# --- GitHub release helper ---
#
# Creates a GitHub release with the given tag and uploads all _release/*.gz files.
# Usage: make release-gh TAG=v0.1.0
.PHONY: release-gh
release-gh:
	@test -n "$(TAG)" || { echo "Usage: make release-gh TAG=vX.Y.Z"; exit 1; }
	@test -d "$(RELEASE_DIR)" || { echo "Run 'make release' first"; exit 1; }
	gh release create "$(TAG)" $(RELEASE_DIR)/*.gz $(RELEASE_DIR)/*.sha256 \
		--repo $(MODULE) \
		--title "$(TAG)" \
		--generate-notes
	@echo "---"
	@echo "Release $(TAG) created at https://github.com/$(MODULE)/releases/tag/$(TAG)"
# --- Site release ---
#
# Update the slingshot.dscli.io site with the latest version and rebuild.
# The site repo is expected at SITE_DIR.
# Usage: make release-site TAG=v0.2.0
SITE_DIR := $(HOME)/.local/src/gitlab.com/dscli/slingshot.dscli.io

.PHONY: release-site
release-site:
	@test -n "$(TAG)" || { echo "Usage: make release-site TAG=vX.Y.Z"; exit 1; }
	@test -d "$(SITE_DIR)" || { echo "Site repo not found at $(SITE_DIR)"; exit 1; }
	@cd "$(SITE_DIR)" && grep -q "^## $(TAG) (" content/en-US/docs/releases.smd || { echo "ERROR: release notes for $(TAG) missing — add a '## $(TAG)' entry to content/en-US/docs/releases.smd and content/zh-CN/docs/releases.smd first"; exit 1; }
	cd "$(SITE_DIR)" && \
		git pull --ff-only && \
		./scripts/update-version.sh "$(TAG)" && \
		rm -rf public && \
		zine release && \
		git add assets/version.ziggy && \
		{ git diff --cached --quiet || git commit -m "chore: update version to $(TAG)"; } && \
		git push
	slingshot site rsync slingshot.dscli.io
	@echo "---"
	@echo "Site updated, built and deployed to https://slingshot.dscli.io"
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
